package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/senilio/zappr/internal/ai"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/config"
	"github.com/senilio/zappr/internal/github"
	"github.com/senilio/zappr/internal/tui"
)

func Run(args []string) error {
	if len(args) == 0 {
		return usage()
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "watch":
		zappr, err := newApp(cfg)
		if err != nil {
			return err
		}
		return runWatch(zappr, cfg, args[1:])
	case "inbox":
		zappr, err := newApp(cfg)
		if err != nil {
			return err
		}
		return runInbox(zappr, args[1:])
	case "review":
		zappr, err := newApp(cfg)
		if err != nil {
			return err
		}
		return runReview(zappr, args[1:])
	case "approve":
		zappr, err := newApp(cfg)
		if err != nil {
			return err
		}
		return runApprove(zappr, args[1:])
	case "config":
		return runConfig(args[1:])
	case "cache":
		zappr, err := newApp(cfg)
		if err != nil {
			return err
		}
		return runCache(zappr, args[1:])
	case "doctor":
		return runDoctor(cfg)
	case "help", "--help", "-h":
		return usage()
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usageText())
	}
}

func newApp(cfg config.Config) (*app.App, error) {
	gh := github.NewGHProvider()
	aiProvider := ai.Provider(ai.NoopProvider{})
	aiCommand := cfg.AI.Command
	if cmd := os.Getenv("ZAPPR_AI_COMMAND"); strings.TrimSpace(cmd) != "" {
		aiCommand = cmd
	}
	if strings.TrimSpace(aiCommand) != "" {
		aiProvider = ai.ShellProvider{Command: aiCommand, Timeout: cfg.AI.Timeout.Duration()}
	}
	store, err := cache.OpenSQLite(cfg.Cache.Path)
	if err != nil {
		return nil, err
	}
	return app.New(gh, aiProvider, store), nil
}

func runWatch(zappr *app.App, cfg config.Config, args []string) error {
	repo, err := requireRepo(args)
	if err != nil {
		return err
	}
	program := tea.NewProgram(tui.New(zappr, repo, cfg.Refresh.Interval.Duration()))
	_, err = program.Run()
	return err
}

func runInbox(zappr *app.App, args []string) error {
	repo, err := requireRepo(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	prs, err := zappr.Refresh(ctx, repo)
	if err != nil {
		return err
	}
	for _, pr := range prs {
		fmt.Printf("%-12s #%d %-6s %-8s %s\n", pr.Repo, pr.Number, strings.ToUpper(string(pr.Triage.Risk)), pr.CIStatus, pr.Title)
	}
	return nil
}

func runReview(zappr *app.App, args []string) error {
	repo, number, err := parsePRRef(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pr, err := zappr.Analyze(ctx, repo, number)
	if err != nil {
		return err
	}
	fmt.Printf("%s #%d %s\n%s\n", pr.Repo, pr.Number, strings.ToUpper(string(pr.Triage.Risk)), pr.Title)
	fmt.Printf("Summary: %s\n", pr.Triage.Summary)
	printList("Reasons", pr.Triage.Reasons)
	printList("Concerns", pr.Triage.Concerns)
	printList("High-risk files", pr.Triage.HighRiskFiles)
	printList("Review targets", pr.Triage.ReviewTargets)
	return nil
}

func runApprove(zappr *app.App, args []string) error {
	repo, number, err := parsePRRef(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pr, err := zappr.Analyze(ctx, repo, number)
	if err != nil {
		return err
	}
	if pr.Triage.Risk == "block" {
		return fmt.Errorf("refusing quick approval for BLOCK PR; inspect manually")
	}
	return zappr.Approve(ctx, repo, number, "Reviewed via zappr.")
}

func runConfig(args []string) error {
	path, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		fmt.Println(path)
		return nil
	}
	switch args[0] {
	case "path":
		fmt.Println(path)
		return nil
	case "init":
		created, err := config.Init(false)
		if err != nil {
			return err
		}
		fmt.Printf("created %s\n", created)
		return nil
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func runCache(zappr *app.App, args []string) error {
	if len(args) == 0 || args[0] == "status" {
		prs := zappr.Inbox()
		fmt.Printf("path: %s\n", zappr.Cache.Path())
		fmt.Printf("pull_requests: %d\n", len(prs))
		return nil
	}
	if args[0] == "clear" {
		if err := zappr.Cache.Clear(); err != nil {
			return err
		}
		fmt.Println("cache cleared")
		return nil
	}
	return fmt.Errorf("unknown cache command %q", args[0])
}

func runDoctor(cfg config.Config) error {
	checks := []struct {
		name string
		err  error
	}{
		{name: "gh installed", err: commandExists("gh")},
		{name: "gh authenticated", err: runQuiet("gh", "auth", "status")},
		{name: "cache writable", err: ensureWritable(cfg.Cache.Path)},
	}
	if strings.TrimSpace(cfg.AI.Command) != "" {
		checks = append(checks, struct {
			name string
			err  error
		}{name: "ai command configured", err: nil})
	}
	failed := false
	for _, check := range checks {
		if check.err != nil {
			failed = true
			fmt.Printf("FAIL %s: %v\n", check.name, check.err)
			continue
		}
		fmt.Printf("OK   %s\n", check.name)
	}
	if failed {
		return fmt.Errorf("doctor found failures")
	}
	return nil
}

func commandExists(name string) error {
	_, err := exec.LookPath(name)
	return err
}

func runQuiet(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

func ensureWritable(path string) error {
	if err := os.MkdirAll(filepathDir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

func filepathDir(path string) string {
	idx := strings.LastIndex(path, string(os.PathSeparator))
	if idx < 0 {
		return "."
	}
	return path[:idx]
}

func requireRepo(args []string) (string, error) {
	if len(args) != 1 || !strings.Contains(args[0], "/") {
		return "", errors.New("repo required: zappr watch owner/repo")
	}
	return args[0], nil
}

func parsePRRef(args []string) (string, int, error) {
	if len(args) != 1 {
		return "", 0, errors.New("PR required: owner/repo#123")
	}
	parts := strings.Split(args[0], "#")
	if len(parts) != 2 || !strings.Contains(parts[0], "/") {
		return "", 0, errors.New("PR must be owner/repo#123")
	}
	number, err := strconv.Atoi(parts[1])
	if err != nil || number <= 0 {
		return "", 0, errors.New("PR number must be positive")
	}
	return parts[0], number, nil
}

func printList(title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Printf("%s:\n", title)
	for _, value := range values {
		fmt.Printf("- %s\n", value)
	}
}

func usage() error {
	fmt.Print(usageText())
	return nil
}

func usageText() string {
	return `zappr - terminal-first PR inbox

Commands:
  zappr watch owner/repo        open keyboard inbox
  zappr inbox owner/repo        print current inbox
  zappr review owner/repo#123   analyze one PR
  zappr approve owner/repo#123  approve non-block PR via gh pr review
  zappr config [path|init]      inspect or create config
  zappr cache [status|clear]    inspect or clear local cache
  zappr doctor                  check gh, auth, cache

Keys:
  j/k navigate  enter inspect  a approve  c comment  s skip  r refresh  v diff  q quit
`
}
