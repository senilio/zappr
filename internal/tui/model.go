package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/config"
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
	ModeHelp
	ModeSettings
	ModeCommand
	ModeConfirmCommand
	ModeColorscheme
)

type Model struct {
	app              *app.App
	repo             string
	allPRs           []model.PullRequest
	prs              []model.PullRequest
	selected         int
	sortMode         app.SortMode
	hideDrafts       bool
	settingsCursor   int
	colorschemeIndex int // selected index in colorscheme picker
	focus            Focus
	mode             Mode
	loading          bool
	error            string
	message          string
	diff             string
	rawDiff          string
	diffFiles        []diffFile // raw per-file bodies (for file switching + V pager)
	renderedFiles    []diffFile // rendered (delta/fallback) per-file bodies
	diffMode         DiffMode
	fileContent      string
	fileContentKey   string
	fileIndex        int
	commitIndex      int
	sideBySide       bool
	comment          string
	diffXOffset      int
	summaryXOffset   int
	lastFetch        time.Time
	pollEvery        time.Duration
	width            int
	height           int
	theme            uitheme.Theme
	cfg              config.Config
	actions          actionRegistry
	prevMode         Mode
	inboxVP          viewport.Model
	summaryVP        viewport.Model
	diffVP           viewport.Model
	helpVP           viewport.Model

	statusActive       bool
	statusText         string
	statusErr          bool
	statusSpinner      spinner.Model
	statusToken        int
	diffLoadKey        string
	detailsLoadKey     string
	commitDiffKey      string
	renderedCommitDiff string
	diffEnabled        bool
	summaryMode        SummaryMode

	commandInput  string // typed input after ":"
	helpInput     string
	pendingAction tuiAction // action awaiting confirmation
	commandCursor int       // selected index in command palette list

}

type SummaryMode int

type DiffMode int

const (
	DiffModeHunks DiffMode = iota
	DiffModeFile
)

func (m DiffMode) String() string {
	switch m {
	case DiffModeFile:
		return "file"
	default:
		return "diff"
	}
}

const (
	SummaryModeFiles SummaryMode = iota
	SummaryModeCommits
	SummaryModeComments
	SummaryModeAtlantis
)

func (m SummaryMode) String() string {
	switch m {
	case SummaryModeCommits:
		return "commits"
	case SummaryModeComments:
		return "comments"
	case SummaryModeAtlantis:
		return "atlantis"
	default:
		return "files"
	}
}

type Focus int

const (
	FocusInbox Focus = iota
	FocusSummary
	FocusDiff
)

func New(zappr *app.App, repo string, pollEvery time.Duration, themeName string, cfg config.Config) (Model, error) {
	actions, err := newActionRegistry(cfg.Actions)
	if err != nil {
		return Model{}, fmt.Errorf("configure TUI actions: %w", err)
	}
	if pollEvery == 0 {
		pollEvery = 30 * time.Second
	}
	vp := viewport.New(0, 0)
	ivp := viewport.New(0, 0)
	dvp := viewport.New(0, 0)
	hvp := viewport.New(0, 0)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{app: zappr, repo: repo, loading: true, pollEvery: pollEvery, sortMode: app.ParseSortMode(cfg.Sort), hideDrafts: cfg.HideDrafts, theme: uitheme.DefaultWithName(themeName), cfg: cfg, actions: actions, inboxVP: ivp, summaryVP: vp, diffVP: dvp, helpVP: hvp, statusSpinner: sp, summaryMode: SummaryModeFiles}, nil
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.app, m.repo, !m.hideDrafts), tickCmd(m.pollEvery), uiTickCmd())
}

type refreshMsg struct {
	prs []model.PullRequest
	err error
}

type refreshPRMsg struct {
	repo   string
	number int
	pr     model.PullRequest
	err    error
}

type diffMsg struct {
	repo          string
	number        int
	raw           string
	rendered      string
	files         []diffFile
	renderedFiles []diffFile
	key           string
	err           error
}

type actionMsg struct {
	message string
	err     error
}

type pagerMsg struct {
	message string
	err     error
}

type commitDiffMsg struct {
	sha      string
	rendered string
	err      error
}

type fileContentMsg struct {
	key     string
	content string
	err     error
}

type tickMsg struct{}

type uiTickMsg struct{}

func uiTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return uiTickMsg{}
	})
}

type statusDismissMsg struct {
	token int
}

func dismissStatusCmd(token int, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return statusDismissMsg{token: token}
	})
}

// startStatus shows the status popup with an in-progress message and returns
// the spinner tick command to animate it.
func (m *Model) startStatus(text string) tea.Cmd {
	m.statusActive = true
	m.statusErr = false
	m.statusText = text
	m.statusToken++
	return m.statusSpinner.Tick
}

// finishStatus updates the popup to a final message and schedules auto-dismiss.
func (m *Model) finishStatus(text string, isErr bool) tea.Cmd {
	m.statusActive = true
	m.statusErr = isErr
	m.statusText = text
	m.statusToken++
	return dismissStatusCmd(m.statusToken, 1200*time.Millisecond)
}

func saveThemeCmd(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		if err := config.Save(cfg); err != nil {
			return actionMsg{message: "failed to save theme: " + err.Error(), err: err}
		}
		return actionMsg{message: "theme saved: " + cfg.Theme}
	}
}

func saveSettingCmd(cfg config.Config, setting string) tea.Cmd {
	return func() tea.Msg {
		if err := config.Save(cfg); err != nil {
			return actionMsg{message: "failed to save " + setting + ": " + err.Error(), err: err}
		}
		return nil
	}
}

func refreshCmd(app *app.App, repo string, includeDrafts bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		prs, err := app.Refresh(ctx, repo, includeDrafts)
		return refreshMsg{prs: prs, err: err}
	}
}

func refreshPRCmd(app *app.App, pr model.PullRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		fresh, err := app.RefreshPR(ctx, pr.Repo, pr.Number)
		return refreshPRMsg{repo: pr.Repo, number: pr.Number, pr: fresh, err: err}
	}
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func diffCmd(app *app.App, pr model.PullRequest, width int) tea.Cmd {
	return func() tea.Msg {
		key := prRefreshKey(pr)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		diff, err := app.GitHub.GetDiff(ctx, pr.Repo, pr.Number)
		if err != nil {
			return diffMsg{repo: pr.Repo, number: pr.Number, key: key, err: err}
		}
		// Split on the raw diff so that the "diff --git" markers are intact.
		// Rendering (delta) rewrites those headers so splitting rendered output
		// would collapse all files into a single chunk.
		files := splitDiffFiles(diff)
		rendered, err := renderDiff(ctx, diff, width, false)
		renderedFiles := renderDiffFiles(ctx, files, width, false)
		return diffMsg{repo: pr.Repo, number: pr.Number, key: key, raw: diff, rendered: rendered, files: files, renderedFiles: renderedFiles, err: err}
	}
}

func commitDiffCmd(ghProvider interface {
	GetCommitDiff(ctx context.Context, repo string, sha string) (string, error)
}, repo string, sha string, width int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		raw, err := ghProvider.GetCommitDiff(ctx, repo, sha)
		if err != nil {
			return commitDiffMsg{sha: sha, err: err}
		}
		rendered, _ := renderDiff(ctx, raw, width, false)
		return commitDiffMsg{sha: sha, rendered: rendered}
	}
}

func fileContentCmd(ghProvider interface {
	GetFileContent(ctx context.Context, repo string, path string, ref string) (string, error)
}, repo string, path string, ref string, key string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		content, err := ghProvider.GetFileContent(ctx, repo, path, ref)
		return fileContentMsg{key: key, content: content, err: err}
	}
}

func renderDiffCmd(diff string, repo string, number int, key string, width int, sideBySide bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rendered, err := renderDiff(ctx, diff, width, sideBySide)
		files := splitDiffFiles(diff)
		renderedFiles := renderDiffFiles(ctx, files, width, sideBySide)
		return diffMsg{repo: repo, number: number, key: key, raw: diff, rendered: rendered, files: files, renderedFiles: renderedFiles, err: err}
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

func toggleDraftCmd(app *app.App, pr model.PullRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		newDraft := !pr.Draft
		err := app.ToggleDraft(ctx, pr.Repo, pr.Number, newDraft)
		action := "marked ready"
		if newDraft {
			action = "marked draft"
		}
		return actionMsg{message: fmt.Sprintf("%s %s#%d", action, pr.Repo, pr.Number), err: err}
	}
}

func closePRCmd(app *app.App, pr model.PullRequest, comment string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := app.ClosePR(ctx, pr.Repo, pr.Number, comment)
		return actionMsg{message: fmt.Sprintf("closed %s#%d", pr.Repo, pr.Number), err: err}
	}
}

func updateBranchCmd(app *app.App, pr model.PullRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := app.UpdateBranch(ctx, pr.Repo, pr.Number)
		return actionMsg{message: fmt.Sprintf("updated branch for %s#%d", pr.Repo, pr.Number), err: err}
	}
}

func requestChangesCmd(app *app.App, pr model.PullRequest, body string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := app.RequestChanges(ctx, pr.Repo, pr.Number, body)
		return actionMsg{message: fmt.Sprintf("requested changes on %s#%d", pr.Repo, pr.Number), err: err}
	}
}

func mergePRCmd(a *app.App, pr model.PullRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := a.MergePR(ctx, pr.Repo, pr.Number)
		return actionMsg{message: fmt.Sprintf("merge requested for %s#%d", pr.Repo, pr.Number), err: err}
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
		args := []string{"--paging=never", "--line-numbers", "--file-decoration-style=none", "--hunk-header-decoration-style=none", "--hunk-header-style=omit"}
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
			// delta emits non-SGR CSI sequences (e.g. ESC[0K) that measure as
			// zero width but are executed by the terminal, corrupting layout.
			// Strip them at ingest while preserving color codes.
			// Also trim leading/trailing blank lines that delta adds around
			// each file header to avoid empty rows at the pane border.
			cleaned := stripNonSGR(out.String())
			cleaned = strings.TrimRight(cleaned, "\n")
			cleaned = strings.TrimLeft(cleaned, "\n")
			return cleaned, nil
		}
	}
	return fallbackDiff(diff), nil
}

// renderDiffFiles renders each raw diffFile individually so that per-file
// bodies can be displayed when the user navigates between files. Rendering the
// full diff as a single string and then splitting the result is unreliable
// because tools like delta rewrite the "diff --git" header lines, making the
// split markers invisible.
func renderDiffFiles(ctx context.Context, files []diffFile, width int, sideBySide bool) []diffFile {
	out := make([]diffFile, len(files))
	for i, f := range files {
		rendered, err := renderDiff(ctx, f.Body, width, sideBySide)
		if err != nil {
			rendered = fallbackDiff(f.Body)
		}
		out[i] = diffFile{Path: f.Path, Body: rendered}
	}
	return out
}

func fallbackDiff(diff string) string {
	var b strings.Builder
	for _, line := range strings.Split(diff, "\n") {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func longestLineWidth(content string) int {
	width := 0
	for _, line := range strings.Split(content, "\n") {
		if w := ansi.StringWidth(line); w > width {
			width = w
		}
	}
	return width
}
