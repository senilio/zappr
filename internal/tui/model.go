package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/model"
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
	app       *app.App
	repo      string
	prs       []model.PullRequest
	selected  int
	mode      Mode
	loading   bool
	error     string
	message   string
	diff      string
	comment   string
	lastFetch time.Time
	pollEvery time.Duration
}

func New(app *app.App, repo string, pollEvery time.Duration) Model {
	if pollEvery == 0 {
		pollEvery = 2 * time.Minute
	}
	return Model{app: app, repo: repo, loading: true, pollEvery: pollEvery}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.app, m.repo), tickCmd(m.pollEvery))
}

type refreshMsg struct {
	prs []model.PullRequest
	err error
}

type diffMsg struct {
	diff string
	err  error
}

type actionMsg struct {
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
		return diffMsg{diff: diff, err: err}
	}
}

func approveCmd(app *app.App, pr model.PullRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		body := "Reviewed via zappr."
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

func riskLabel(r model.Risk) string {
	return strings.ToUpper(string(r))
}
