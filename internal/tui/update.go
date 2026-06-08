package tui

import (
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/model"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		m.loading = false
		if msg.err != nil {
			m.error = msg.err.Error()
			m.prs = visiblePRs(m.app.Inbox(m.sortMode))
			return m, nil
		}
		m.error = ""
		m.message = "refreshed"
		m.prs = sortPRs(visiblePRs(msg.prs), m.sortMode)
		m.lastFetch = timeNow()
		if m.selected >= len(m.prs) {
			m.selected = len(m.prs) - 1
		}
		if m.selected < 0 {
			m.selected = 0
		}
		return m, nil

	case diffMsg:
		m.loading = false
		if msg.err != nil {
			m.error = msg.err.Error()
			return m, nil
		}
		m.error = ""
		m.rawDiff = msg.raw
		m.diff = trimDiff(msg.rendered)
		m.diffFiles = msg.files
		m.mode = ModeDiff
		m.focus = FocusDiff
		m.diffVP.SetContent(m.diff)
		m.diffVP.GotoTop()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewports()
		return m, nil

	case actionMsg:
		m.loading = false
		if msg.err != nil {
			m.error = msg.err.Error()
			return m, nil
		}
		m.error = ""
		m.message = msg.message
		m.mode = ModeInbox
		return m, refreshCmd(m.app, m.repo)

	case pagerMsg:
		m.loading = false
		if msg.err != nil {
			m.error = msg.err.Error()
			return m, nil
		}
		m.error = ""
		m.message = msg.message
		return m, nil

	case tickMsg:
		if m.loading {
			return m, tickCmd(m.pollEvery)
		}
		m.loading = true
		m.message = "polling"
		return m, tea.Batch(refreshCmd(m.app, m.repo), tickCmd(m.pollEvery))

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	switch m.focus {
	case FocusSummary:
		m.summaryVP, cmd = m.summaryVP.Update(msg)
	case FocusDiff:
		m.diffVP, cmd = m.diffVP.Update(msg)
	}
	if cmd != nil {
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "q" || key == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode {
	case ModeComment:
		return m.handleCommentKey(key)
	case ModeConfirmApprove:
		return m.handleApproveKey(key)
	case ModeHelp:
		if key == "esc" || key == "?" || key == "enter" {
			m.mode = m.prevMode
		}
		return m, nil
	case ModeDiff:
		if key == "esc" || key == "enter" || key == "v" {
			m.mode = ModeDetail
			m.focus = FocusSummary
			return m, nil
		}
	}

	if key == "?" {
		m.prevMode = m.mode
		m.mode = ModeHelp
		return m, nil
	}

	switch key {
	case "tab":
		m.nextFocus()
	case "shift+tab":
		m.prevFocus()
	case "o":
		m.cycleSortMode()
	case "j", "down":
		if m.focus == FocusSummary {
			m.summaryVP.LineDown(1)
		} else if m.focus == FocusDiff {
			m.diffVP.LineDown(1)
		} else if m.selected < len(m.prs)-1 {
			m.selected++
			m.resetPRViewState()
		}
	case "k", "up":
		if m.focus == FocusSummary {
			m.summaryVP.LineUp(1)
		} else if m.focus == FocusDiff {
			m.diffVP.LineUp(1)
		} else if m.selected > 0 {
			m.selected--
			m.resetPRViewState()
		}
	case "enter":
		if _, ok := m.current(); ok {
			m.mode = ModeDetail
			m.focus = FocusSummary
		}
	case "esc":
		m.mode = ModeInbox
		m.focus = FocusInbox
	case "r":
		if m.mode == ModeDetail || m.mode == ModeDiff {
			if pr, ok := m.current(); ok {
				m.loading = true
				m.message = "re-running analysis"
				return m, analyzeCmd(m.app, pr)
			}
		}
		m.loading = true
		m.message = "refreshing"
		return m, refreshCmd(m.app, m.repo)
	case "v":
		if m.diff != "" {
			m.mode = ModeDiff
			m.focus = FocusDiff
			return m, nil
		}
		pr, ok := m.current()
		if !ok {
			return m, nil
		}
		m.loading = true
		m.message = "fetching diff"
		return m, diffCmd(m.app, pr)
	case "V":
		if pr, ok := m.current(); ok {
			return m, openDiffPagerCmd(pr.Repo, pr.Number)
		}
	case "n", "]":
		if len(m.diffFiles) > 0 && m.fileIndex < len(m.diffFiles)-1 {
			m.fileIndex++
			m.diffVP.SetYOffset(m.diffFileOffset())
		} else if pr, ok := m.current(); ok && m.fileIndex < len(pr.Files)-1 {
			m.fileIndex++
		}
	case "p", "[":
		if m.fileIndex > 0 {
			m.fileIndex--
			if len(m.diffFiles) > 0 {
				m.diffVP.SetYOffset(m.diffFileOffset())
			}
		}
	case "s":
		if m.rawDiff != "" && (m.mode == ModeDiff || m.focus == FocusDiff) {
			m.sideBySide = !m.sideBySide
			m.loading = true
			return m, renderDiffCmd(m.rawDiff, m.width, m.sideBySide)
		}
		if pr, ok := m.current(); ok {
			if err := m.app.Skip(pr.Repo, pr.Number); err != nil {
				m.error = err.Error()
				return m, nil
			}
			m.prs = visiblePRs(m.app.Inbox(m.sortMode))
			m.message = "skipped"
		}
	case "a":
		if pr, ok := m.current(); ok {
			if pr.Triage.Risk == "block" {
				m.error = "blocked PR requires manual inspection before approval"
				return m, nil
			}
			m.mode = ModeConfirmApprove
		}
	case "c":
		if _, ok := m.current(); ok {
			m.comment = ""
			m.mode = ModeComment
		}
	}
	return m, nil
}

func (m Model) handleApproveKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "n" {
		m.mode = ModeDetail
		return m, nil
	}
	if key == "y" {
		pr, ok := m.current()
		if !ok {
			m.mode = ModeInbox
			return m, nil
		}
		m.loading = true
		return m, approveCmd(m.app, pr)
	}
	return m, nil
}

func (m *Model) resizeViewports() {
	m.summaryVP.Width = max(1, m.width/3)
	m.summaryVP.Height = max(1, m.height-6)
	m.diffVP.Width = max(1, m.width/3)
	m.diffVP.Height = max(1, m.height-6)
}

func (m *Model) nextFocus() {
	switch m.focus {
	case FocusInbox:
		m.focus = FocusSummary
	case FocusSummary:
		m.focus = FocusDiff
	default:
		m.focus = FocusInbox
	}
}

func (m *Model) prevFocus() {
	switch m.focus {
	case FocusDiff:
		m.focus = FocusSummary
	case FocusSummary:
		m.focus = FocusInbox
	default:
		m.focus = FocusDiff
	}
}

func (m Model) diffFileOffset() int {
	if m.fileIndex <= 0 || m.fileIndex >= len(m.diffFiles) {
		return 0
	}
	offset := 0
	for i := 0; i < m.fileIndex; i++ {
		offset += stringsCountLines(m.diffFiles[i].Body)
	}
	return offset
}

func stringsCountLines(s string) int {
	count := 0
	for _, r := range s {
		if r == '\n' {
			count++
		}
	}
	return count
}

func openDiffPagerCmd(repo string, number int) tea.Cmd {
	cmd := "if command -v delta >/dev/null 2>&1; then gh pr diff " + shellQuote(strconv.Itoa(number)) + " --repo " + shellQuote(repo) + " | delta --line-numbers --paging=always; else gh pr diff " + shellQuote(strconv.Itoa(number)) + " --repo " + shellQuote(repo) + " | less -R; fi"
	return tea.ExecProcess(exec.Command("sh", "-c", cmd), func(err error) tea.Msg {
		if err != nil {
			return pagerMsg{err: err}
		}
		return pagerMsg{message: "closed diff pager"}
	})
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func (m *Model) resetPRViewState() {
	m.diff = ""
	m.rawDiff = ""
	m.diffFiles = nil
	m.fileIndex = 0
	m.summaryVP.GotoTop()
	m.diffVP.GotoTop()
}

func (m *Model) cycleSortMode() {
	current := 0
	if pr, ok := m.current(); ok {
		current = pr.Number
	}
	m.sortMode = m.sortMode.Next()
	m.prs = sortPRs(m.prs, m.sortMode)
	m.message = "sort: " + m.sortMode.String()
	if current == 0 {
		return
	}
	for i, pr := range m.prs {
		if pr.Number == current {
			m.selected = i
			return
		}
	}
}

func sortPRs(prs []model.PullRequest, sortMode app.SortMode) []model.PullRequest {
	out := append([]model.PullRequest(nil), prs...)
	app.SortPullRequests(out, sortMode)
	return out
}

func visiblePRs(prs []model.PullRequest) []model.PullRequest {
	out := make([]model.PullRequest, 0, len(prs))
	for _, pr := range prs {
		if !pr.Draft {
			out = append(out, pr)
		}
	}
	return out
}

func (m Model) handleCommentKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		m.mode = ModeDetail
		return m, nil
	}
	if key == "enter" {
		pr, ok := m.current()
		if !ok || m.comment == "" {
			return m, nil
		}
		m.loading = true
		return m, commentCmd(m.app, pr, m.comment)
	}
	if key == "backspace" || key == "ctrl+h" {
		if len(m.comment) > 0 {
			m.comment = m.comment[:len(m.comment)-1]
		}
		return m, nil
	}
	if len(key) == 1 {
		m.comment += key
	}
	return m, nil
}
