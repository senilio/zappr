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
	ModeHelp
)

type Model struct {
	app           *app.App
	repo          string
	prs           []model.PullRequest
	selected      int
	sortMode      app.SortMode
	focus         Focus
	mode          Mode
	loading       bool
	error         string
	message       string
	diff          string
	rawDiff       string
	diffFiles     []diffFile // raw per-file bodies (for file switching + V pager)
	renderedFiles []diffFile // rendered (delta/fallback) per-file bodies
	fileIndex     int
	sideBySide    bool
	comment       string
	lastFetch     time.Time
	pollEvery     time.Duration
	width         int
	height        int
	theme         uitheme.Theme
	aiEnabled     bool
	prevMode      Mode
	inboxVP       viewport.Model
	summaryVP     viewport.Model
	diffVP        viewport.Model

	statusActive  bool
	statusText    string
	statusErr     bool
	statusSpinner spinner.Model
	statusToken   int
	diffLoadKey   string
	diffEnabled   bool
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
	ivp := viewport.New(0, 0)
	dvp := viewport.New(0, 0)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{app: zappr, repo: repo, loading: true, pollEvery: pollEvery, sortMode: app.SortUpdated, theme: uitheme.Default(), aiEnabled: zappr.AIEnabled(), inboxVP: ivp, summaryVP: vp, diffVP: dvp, statusSpinner: sp}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.app, m.repo), tickCmd(m.pollEvery), uiTickCmd())
}

type refreshMsg struct {
	prs []model.PullRequest
	err error
}

type diffMsg struct {
	repo          string
	number        int
	raw           string
	rendered      string
	files         []diffFile
	renderedFiles []diffFile
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

func refreshCmd(app *app.App, repo string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		prs, err := app.Refresh(ctx, repo)
		return refreshMsg{prs: prs, err: err}
	}
}

func analyzeCmd(app *app.App, pr model.PullRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if _, err := app.Analyze(ctx, pr.Repo, pr.Number); err != nil {
			return refreshMsg{err: err}
		}
		prs, err := app.Refresh(ctx, pr.Repo)
		return refreshMsg{prs: prs, err: err}
	}
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func diffCmd(app *app.App, pr model.PullRequest, width int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		diff, err := app.GitHub.GetDiff(ctx, pr.Repo, pr.Number)
		if err != nil {
			return diffMsg{err: err}
		}
		// Split on the raw diff so that the "diff --git" markers are intact.
		// Rendering (delta) rewrites those headers so splitting rendered output
		// would collapse all files into a single chunk.
		files := splitDiffFiles(diff)
		rendered, err := renderDiff(ctx, diff, width, false)
		renderedFiles := renderDiffFiles(ctx, files, width, false)
		return diffMsg{repo: pr.Repo, number: pr.Number, raw: diff, rendered: rendered, files: files, renderedFiles: renderedFiles, err: err}
	}
}

func renderDiffCmd(diff string, repo string, number int, width int, sideBySide bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rendered, err := renderDiff(ctx, diff, width, sideBySide)
		files := splitDiffFiles(diff)
		renderedFiles := renderDiffFiles(ctx, files, width, sideBySide)
		return diffMsg{repo: repo, number: number, raw: diff, rendered: rendered, files: files, renderedFiles: renderedFiles, err: err}
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
		args := []string{"--paging=never", "--line-numbers", "--file-decoration-style=none", "--hunk-header-decoration-style=none"}
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
