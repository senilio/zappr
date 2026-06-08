package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

type Mode int

const (
	ModeInbox Mode = iota
	ModeDetail
	ModeDiff
	ModeConfirmApprove
	ModeComment
)

type Model struct {
	app        *app.App
	repo       string
	prs        []model.PullRequest
	selected   int
	sortMode   app.SortMode
	focus      Focus
	mode       Mode
	loading    bool
	error      string
	message    string
	diff       string
	rawDiff    string
	diffFiles  []diffFile
	fileIndex  int
	sideBySide bool
	comment    string
	lastFetch  time.Time
	pollEvery  time.Duration
	width      int
	height     int
	theme      uitheme.Theme
	summaryVP  viewport.Model
	diffVP     viewport.Model
}

type Focus int

const (
	FocusInbox Focus = iota
	FocusSummary
	FocusDiff
)

func New(zappr *app.App, repo string, pollEvery time.Duration) Model {
	if pollEvery == 0 {
		pollEvery = 2 * time.Minute
	}
	vp := viewport.New(0, 0)
	dvp := viewport.New(0, 0)
	return Model{app: zappr, repo: repo, loading: true, pollEvery: pollEvery, sortMode: app.SortUpdated, theme: uitheme.Default(), summaryVP: vp, diffVP: dvp}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.app, m.repo), tickCmd(m.pollEvery))
}

type refreshMsg struct {
	prs []model.PullRequest
	err error
}

type diffMsg struct {
	raw      string
	rendered string
	files    []diffFile
	err      error
}

type actionMsg struct {
	message string
	err     error
}

type pagerMsg struct {
	message string
	err     error
}

type tickMsg struct{}

func refreshCmd(app *app.App, repo string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		prs, err := app.Refresh(ctx, repo)
		return refreshMsg{prs: prs, err: err}
	}
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func diffCmd(app *app.App, pr model.PullRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		diff, err := app.GitHub.GetDiff(ctx, pr.Repo, pr.Number)
		if err != nil {
			return diffMsg{err: err}
		}
		files := splitDiffFiles(diff)
		rendered, err := renderDiff(ctx, diff, 0, false)
		return diffMsg{raw: diff, rendered: rendered, files: files, err: err}
	}
}

func renderDiffCmd(diff string, width int, sideBySide bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rendered, err := renderDiff(ctx, diff, width, sideBySide)
		return diffMsg{raw: diff, rendered: rendered, files: splitDiffFiles(diff), err: err}
	}
}

func approveCmd(app *app.App, pr model.PullRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		body := ""
		err := app.Approve(ctx, pr.Repo, pr.Number, body)
		return actionMsg{message: fmt.Sprintf("approved %s#%d", pr.Repo, pr.Number), err: err}
	}
}

func commentCmd(app *app.App, pr model.PullRequest, body string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := app.Comment(ctx, pr.Repo, pr.Number, body)
		return actionMsg{message: fmt.Sprintf("commented on %s#%d", pr.Repo, pr.Number), err: err}
	}
}

func (m Model) current() (model.PullRequest, bool) {
	if len(m.prs) == 0 || m.selected < 0 || m.selected >= len(m.prs) {
		return model.PullRequest{}, false
	}
	return m.prs[m.selected], true
}

func trimDiff(diff string) string {
	const max = 12000
	if len(diff) <= max {
		return diff
	}
	return diff[:max] + "\n\n... diff truncated in TUI ..."
}

type diffFile struct {
	Path string
	Body string
}

func splitDiffFiles(diff string) []diffFile {
	var files []diffFile
	var current *diffFile
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				files = append(files, *current)
			}
			current = &diffFile{Path: diffPath(line)}
		}
		if current != nil {
			current.Body += line + "\n"
		}
	}
	if current != nil {
		files = append(files, *current)
	}
	return files
}

func diffPath(line string) string {
	parts := strings.Fields(line)
	if len(parts) >= 4 {
		return strings.TrimPrefix(parts[3], "b/")
	}
	return "diff"
}

func renderDiff(ctx context.Context, diff string, width int, sideBySide bool) (string, error) {
	if _, err := exec.LookPath("delta"); err == nil {
		args := []string{"--paging=never", "--line-numbers", "--syntax-theme=GitHub"}
		if width > 0 {
			args = append(args, fmt.Sprintf("--width=%d", width))
		}
		if sideBySide {
			args = append(args, "--side-by-side")
		}
		cmd := exec.CommandContext(ctx, "delta", args...)
		cmd.Stdin = strings.NewReader(diff)
		var out bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			return out.String(), nil
		}
	}
	return fallbackDiff(diff), nil
}

func fallbackDiff(diff string) string {
	var b strings.Builder
	for _, line := range strings.Split(diff, "\n") {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func riskLabel(r model.Risk) string {
	return strings.ToUpper(string(r))
}
