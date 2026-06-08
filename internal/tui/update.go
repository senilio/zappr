package tui

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
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
			return m, m.finishStatus("error: "+msg.err.Error(), true)
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
		var cmds []tea.Cmd
		if m.statusActive {
			cmds = append(cmds, m.finishStatus("refreshed", false))
		}
		if cmd := m.autoDiffCmd(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Sync summary viewport content so TotalLineCount is accurate for
		// the scroll percentage indicator.
		m.syncSummaryVP()
		return m, tea.Batch(cmds...)

	case diffMsg:
		m.loading = false
		// Ignore stale diff results that no longer match the selected PR.
		if pr, ok := m.current(); ok {
			if msg.repo != pr.Repo || msg.number != pr.Number {
				return m, nil
			}
		}
		if msg.err != nil {
			m.error = msg.err.Error()
			return m, m.finishStatus("diff error: "+msg.err.Error(), true)
		}
		m.error = ""
		m.rawDiff = msg.raw
		m.diff = trimDiff(msg.rendered)
		m.diffFiles = msg.files
		m.renderedFiles = msg.renderedFiles
		m.diffVP.GotoTop()
		m.syncDiffVP()
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
			return m, m.finishStatus("error: "+msg.err.Error(), true)
		}
		m.error = ""
		m.message = msg.message
		m.mode = ModeInbox
		return m, tea.Batch(m.finishStatus(msg.message, false), refreshCmd(m.app, m.repo))

	case pagerMsg:
		m.loading = false
		if msg.err != nil {
			m.error = msg.err.Error()
			return m, nil
		}
		m.error = ""
		m.message = msg.message
		return m, nil

	case spinner.TickMsg:
		if !m.statusActive {
			return m, nil
		}
		var cmd tea.Cmd
		m.statusSpinner, cmd = m.statusSpinner.Update(msg)
		return m, cmd

	case statusDismissMsg:
		if msg.token == m.statusToken {
			m.statusActive = false
			m.statusText = ""
			m.statusErr = false
		}
		return m, nil

	case tickMsg:
		if m.loading {
			return m, tickCmd(m.pollEvery)
		}
		m.loading = true
		m.message = "polling"
		return m, tea.Batch(refreshCmd(m.app, m.repo), tickCmd(m.pollEvery))

	case uiTickMsg:
		// Re-render every second so the "refreshed Xs ago" counter stays live.
		return m, uiTickCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	switch m.focus {
	case FocusInbox:
		m.inboxVP, cmd = m.inboxVP.Update(msg)
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
			m.diffEnabled = false
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
		switch m.focus {
		case FocusSummary:
			if vpCanScroll(m.summaryVP) {
				m.summaryVP.LineDown(1)
			}
		case FocusDiff:
			if vpCanScroll(m.diffVP) {
				m.diffVP.LineDown(1)
			}
		default: // FocusInbox
			if m.selected < len(m.prs)-1 {
				m.selected++
				m.resetPRViewState()
				return m, m.autoDiffCmd()
			}
		}
	case "k", "up":
		switch m.focus {
		case FocusSummary:
			if vpCanScroll(m.summaryVP) {
				m.summaryVP.LineUp(1)
			}
		case FocusDiff:
			if vpCanScroll(m.diffVP) {
				m.diffVP.LineUp(1)
			}
		default: // FocusInbox
			if m.selected > 0 {
				m.selected--
				m.resetPRViewState()
				return m, m.autoDiffCmd()
			}
		}
	case "pgdown", " ":
		switch m.focus {
		case FocusSummary:
			if vpCanScroll(m.summaryVP) {
				m.summaryVP.ViewDown()
			}
		case FocusDiff:
			if vpCanScroll(m.diffVP) {
				m.diffVP.ViewDown()
			}
		case FocusInbox:
			if vpCanScroll(m.inboxVP) {
				m.inboxVP.ViewDown()
			}
		}
	case "pgup":
		switch m.focus {
		case FocusSummary:
			if vpCanScroll(m.summaryVP) {
				m.summaryVP.ViewUp()
			}
		case FocusDiff:
			if vpCanScroll(m.diffVP) {
				m.diffVP.ViewUp()
			}
		case FocusInbox:
			if vpCanScroll(m.inboxVP) {
				m.inboxVP.ViewUp()
			}
		}
	case "shift+down":
		if len(m.diffFiles) > 0 && m.fileIndex < len(m.diffFiles)-1 {
			m.fileIndex++
			m.diffVP.GotoTop()
			m.syncDiffVP()
			m.syncSummaryVP()
		} else if pr, ok := m.current(); ok && m.fileIndex < len(pr.Files)-1 {
			m.fileIndex++
			m.diffVP.GotoTop()
			m.syncDiffVP()
			m.syncSummaryVP()
		}
	case "shift+up":
		if m.fileIndex > 0 {
			m.fileIndex--
			m.diffVP.GotoTop()
			m.syncDiffVP()
			m.syncSummaryVP()
		}
	case "enter":
		if _, ok := m.current(); ok {
			m.mode = ModeDetail
			m.focus = FocusSummary
			m.resizeViewports()
			return m, m.autoDiffCmd()
		}
	case "esc":
		m.mode = ModeInbox
		m.focus = FocusInbox
		m.resizeViewports()
	case "r":
		if m.mode == ModeDetail || m.mode == ModeDiff {
			if pr, ok := m.current(); ok {
				m.loading = true
				m.message = "re-running analysis"
				return m, tea.Batch(m.startStatus("re-running analysis…"), analyzeCmd(m.app, pr))
			}
		}
		m.loading = true
		m.message = "refreshing"
		return m, tea.Batch(m.startStatus("refreshing…"), refreshCmd(m.app, m.repo))
	case "v":
		if m.width > 0 && m.width < minMediumWidth {
			// Narrow: toggle full-screen diff mode.
			if m.diffEnabled {
				m.diffEnabled = false
				if m.mode == ModeDiff {
					m.mode = ModeDetail
					m.focus = FocusSummary
				}
				return m, nil
			}
			m.diffEnabled = true
			if m.diff != "" {
				m.mode = ModeDiff
				m.focus = FocusDiff
				return m, nil
			}
			pr, ok := m.current()
			if !ok {
				return m, nil
			}
			m.mode = ModeDiff
			m.focus = FocusDiff
			mv := Model{width: m.width, height: m.height, theme: m.theme, focus: m.focus, prs: m.prs, selected: m.selected}
			_, _, diffInnerW := mv.paneInnerWidths()
			return m, tea.Batch(m.startStatus("fetching diff…"), diffCmd(m.app, pr, max(1, diffInnerW)))
		}
		// Wide/medium: toggle diff pane content.
		m.diffEnabled = !m.diffEnabled
		if m.diffEnabled {
			if m.mode == ModeInbox {
				m.mode = ModeDetail
			}
			return m, m.autoDiffCmd()
		}
		// Turned off: unfocus diff pane if it was focused.
		if m.focus == FocusDiff {
			m.focus = FocusSummary
		}
		return m, nil
	case "V":
		if pr, ok := m.current(); ok {
			return m, openDiffPagerCmd(pr.Repo, pr.Number)
		}
	case "n", "]":
		if len(m.diffFiles) > 0 && m.fileIndex < len(m.diffFiles)-1 {
			m.fileIndex++
			m.diffVP.GotoTop()
			m.syncDiffVP()
			m.syncSummaryVP()
		} else if pr, ok := m.current(); ok && m.fileIndex < len(pr.Files)-1 {
			m.fileIndex++
			m.diffVP.GotoTop()
			m.syncDiffVP()
			m.syncSummaryVP()
		}
	case "p", "[":
		if m.fileIndex > 0 {
			m.fileIndex--
			m.diffVP.GotoTop()
			m.syncDiffVP()
			m.syncSummaryVP()
		}
	case "s":
		if m.rawDiff != "" && (m.mode == ModeDiff || m.focus == FocusDiff) {
			m.sideBySide = !m.sideBySide
			pr, _ := m.current()
			mv := Model{width: m.width, height: m.height, theme: m.theme, focus: m.focus, prs: m.prs, selected: m.selected}
			_, _, diffInnerW := mv.paneInnerWidths()
			return m, renderDiffCmd(m.rawDiff, pr.Repo, pr.Number, max(1, diffInnerW), m.sideBySide)
		}
		if pr, ok := m.current(); ok {
			if err := m.app.SetSkip(pr.Repo, pr.Number, !pr.Skipped); err != nil {
				m.error = err.Error()
				return m, nil
			}
			m.prs = visiblePRs(m.app.Inbox(m.sortMode))
			if pr.Skipped {
				m.message = "unskipped"
			} else {
				m.message = "skipped"
			}
		}
	case "a":
		if _, ok := m.current(); ok {
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
		return m, tea.Batch(m.startStatus("approving…"), approveCmd(m.app, pr))
	}
	return m, nil
}

// autoDiffCmd fetches the diff for the currently selected PR if diff is
// enabled and the diff has not already been loaded. It guards against
// duplicate fetches for the same PR.
func (m *Model) autoDiffCmd() tea.Cmd {
	if !m.diffEnabled {
		return nil
	}
	pr, ok := m.current()
	if !ok {
		return nil
	}
	key := pr.Repo + "#" + strconv.Itoa(pr.Number)
	if m.diffLoadKey == key {
		return nil
	}
	m.diffLoadKey = key
	mv := Model{width: m.width, height: m.height, theme: m.theme, focus: m.focus, prs: m.prs, selected: m.selected}
	_, _, diffInnerW := mv.paneInnerWidths()
	return diffCmd(m.app, pr, max(1, diffInnerW))
}

func (m *Model) resizeViewports() {
	// Set the stored viewport heights so that vpCanScroll and LineDown/LineUp
	// work correctly between renders. Header and footer each occupy 1 line.
	// Pane border adds 2 lines vertically. These constants mirror the layout
	// functions (wideLayout / mediumLayout) and the pane frame size (2).
	const headerLines = 1
	const footerLines = 1
	bodyH := max(4, m.height-headerLines-footerLines-1)
	mv := Model{width: m.width, height: m.height, theme: m.theme, focus: m.focus, prs: m.prs, selected: m.selected}
	if m.width >= minWideWidth {
		inboxH, summaryH := mv.leftColumnHeights(bodyH)
		m.inboxVP.Height = max(1, inboxH-paneFrame)
		m.summaryVP.Height = max(1, summaryH-paneFrame)
		m.diffVP.Height = max(1, bodyH-paneFrame)
	} else if m.width >= minMediumWidth {
		topH := max(8, bodyH/2)
		bottomH := max(6, bodyH-topH)
		vpTopH := max(1, topH-paneFrame)
		vpBotH := max(1, bottomH-paneFrame)
		m.inboxVP.Height = vpTopH
		m.summaryVP.Height = vpTopH
		m.diffVP.Height = vpBotH
	} else {
		vpH := max(1, bodyH-paneFrame)
		m.inboxVP.Height = vpH
		m.summaryVP.Height = vpH
		m.diffVP.Height = vpH
	}
	// Update diff viewport width to the real diff pane inner width.
	_, _, diffInnerW := mv.paneInnerWidths()
	m.diffVP.Width = max(1, diffInnerW)
	m.syncDiffVP()
}

func (m *Model) nextFocus() {
	switch m.focus {
	case FocusInbox:
		m.focus = FocusSummary
		if _, ok := m.current(); ok {
			m.mode = ModeDetail
		}
	case FocusSummary:
		if m.diffEnabled {
			m.focus = FocusDiff
		} else {
			m.focus = FocusInbox
			m.mode = ModeInbox
		}
	default: // FocusDiff
		m.focus = FocusInbox
		m.mode = ModeInbox
	}
	m.resizeViewports()
}

func (m *Model) prevFocus() {
	switch m.focus {
	case FocusSummary:
		m.focus = FocusInbox
		m.mode = ModeInbox
	case FocusDiff:
		m.focus = FocusSummary
		if _, ok := m.current(); ok {
			m.mode = ModeDetail
		}
	default: // FocusInbox
		if m.diffEnabled {
			m.focus = FocusDiff
		} else {
			m.focus = FocusSummary
			if _, ok := m.current(); ok {
				m.mode = ModeDetail
			}
		}
	}
	m.resizeViewports()
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
	m.renderedFiles = nil
	m.fileIndex = 0
	m.diffLoadKey = ""
	m.inboxVP.GotoTop()
	m.summaryVP.GotoTop()
	m.diffVP.GotoTop()
	m.syncSummaryVP()
	m.syncDiffVP()
}

// syncSummaryVP updates the stored summaryVP's content so that TotalLineCount
// is accurate for the scroll percentage indicator. Width is set to the real
// summary inner width; content is word-wrapped so line count is correct.
func (m *Model) syncSummaryVP() {
	pr, ok := m.current()
	if !ok {
		return
	}
	mv := Model{
		theme:     m.theme,
		fileIndex: m.fileIndex,
		width:     m.width,
		height:    m.height,
		focus:     m.focus,
		prs:       m.prs,
		selected:  m.selected,
	}
	_, summaryInnerW, _ := mv.paneInnerWidths()
	content := wrapLines(mv.summaryContent(pr), summaryInnerW)
	m.summaryVP.Width = summaryInnerW
	m.summaryVP.SetContent(content)
}

// syncDiffVP updates the stored diffVP's content so that TotalLineCount is
// accurate for vpCanScroll checks and j/k scrolling works correctly.
// Lines are stored at full width — the viewport's ansi.Cut windows horizontally.
// Width is set to the real diff-pane inner width so scroll % and cut are correct.
func (m *Model) syncDiffVP() {
	if !m.diffEnabled || m.diff == "" {
		return
	}
	// Resolve which rendered file body to show (same logic as diffPanel).
	content := m.diff
	if len(m.renderedFiles) > 0 {
		idx := 0
		if pr, ok := m.current(); ok && m.fileIndex >= 0 && m.fileIndex < len(pr.Files) {
			want := pr.Files[m.fileIndex].Path
			for i, rf := range m.renderedFiles {
				if rf.Path == want {
					idx = i
					break
				}
			}
		} else if m.fileIndex >= 0 && m.fileIndex < len(m.renderedFiles) {
			idx = m.fileIndex
		}
		content = m.renderedFiles[idx].Body
	}
	// Use the real diff-pane inner width so horizontal cut matches render.
	mv := Model{width: m.width, height: m.height, theme: m.theme, focus: m.focus, prs: m.prs, selected: m.selected}
	_, _, diffInnerW := mv.paneInnerWidths()
	vpW := max(1, diffInnerW)
	m.diffVP.Width = vpW
	m.diffVP.SetContent(content)
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

func vpCanScroll(vp viewport.Model) bool {
	return vp.TotalLineCount() > vp.VisibleLineCount()
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
		return m, tea.Batch(m.startStatus("commenting…"), commentCmd(m.app, pr, m.comment))
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
