package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		m.loading = false
		m.aiAnalyzing = false
		if msg.err != nil {
			m.error = msg.err.Error()
			m.allPRs = m.app.InboxRepo(m.repo, m.sortMode)
			m.applyPRSettings()
			return m, m.finishStatus("error: "+msg.err.Error(), true)
		}
		m.error = ""
		m.message = "refreshed"
		// Always treat a successful refresh as the authoritative PR list. Use a
		// non-nil empty slice when there are zero PRs so applyPRSettings does
		// not fall back to the previous (now stale) list and keep showing a PR
		// that was merged/closed externally.
		if msg.prs == nil {
			m.allPRs = []model.PullRequest{}
		} else {
			m.allPRs = msg.prs
		}
		m.applyPRSettings()
		m.lastFetch = timeNow()
		if m.selected >= len(m.prs) {
			m.selected = len(m.prs) - 1
		}
		if m.selected < 0 {
			m.selected = 0
		}
		// If the queue is now empty (e.g. the last PR was merged/closed
		// externally), drop back to the Inbox view and focus so the UI does
		// not stay stuck in a detail/diff mode for a PR that no longer exists.
		if len(m.prs) == 0 {
			m.mode = ModeInbox
			m.focus = FocusInbox
			m.resizeViewports()
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

	case commitDiffMsg:
		if msg.err != nil {
			return m, m.finishStatus("commit diff error: "+msg.err.Error(), true)
		}
		// Only apply if this SHA is still the active commit.
		if msg.sha == m.commitDiffKey {
			m.renderedCommitDiff = msg.rendered
			m.diffVP.GotoTop()
			m.syncDiffVP()
		}
		return m, nil

	case fileContentMsg:
		if msg.key != m.selectedFileContentKey() {
			return m, nil
		}
		if msg.err != nil {
			return m, m.finishStatus("file error: "+msg.err.Error(), true)
		}
		m.fileContentKey = msg.key
		m.fileContent = msg.content
		m.diffVP.GotoTop()
		m.syncDiffVP()
		return m, nil

	case diffMsg:
		m.loading = false
		// Ignore stale diff results that no longer match the selected PR.
		if pr, ok := m.current(); ok {
			if msg.repo != pr.Repo || msg.number != pr.Number {
				return m, nil
			}
		} else {
			return m, nil
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
		m.diffXOffset = 0
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
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			return m.handleMouseClick(msg)
		}
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

type paneRegion struct {
	focus      Focus
	x, y, w, h int
}

func (r paneRegion) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

func (m Model) paneRegions() []paneRegion {
	if m.width <= 0 || m.height <= 0 {
		return nil
	}
	bodyY := 1
	bodyH := max(4, m.height-chromeLines)

	if m.width >= minWideWidth {
		leftW, diffW, gap := m.widePaneOuterWidths()
		inboxH, summaryH := m.leftColumnHeights(bodyH)
		return []paneRegion{
			{focus: FocusInbox, x: 0, y: bodyY, w: leftW, h: inboxH},
			{focus: FocusSummary, x: 0, y: bodyY + inboxH, w: leftW, h: summaryH},
			{focus: FocusDiff, x: leftW + gap, y: bodyY, w: diffW, h: bodyH},
		}
	}

	if m.width >= minMediumWidth {
		topH := max(8, bodyH/2)
		bottomH := max(6, bodyH-topH)
		inboxW := m.width/2 - 1
		summaryW := m.width / 2
		return []paneRegion{
			{focus: FocusInbox, x: 0, y: bodyY, w: inboxW, h: topH},
			{focus: FocusSummary, x: inboxW + 1, y: bodyY, w: summaryW, h: topH},
			{focus: FocusDiff, x: 0, y: bodyY + topH, w: m.width, h: bottomH},
		}
	}

	switch m.mode {
	case ModeDiff:
		return []paneRegion{{focus: FocusDiff, x: 0, y: bodyY, w: m.width, h: bodyH}}
	case ModeDetail:
		return []paneRegion{{focus: FocusSummary, x: 0, y: bodyY, w: m.width, h: bodyH}}
	default:
		return []paneRegion{{focus: FocusInbox, x: 0, y: bodyY, w: m.width, h: bodyH}}
	}
}

func (m Model) paneRegionAt(x, y int) (paneRegion, bool) {
	for _, region := range m.paneRegions() {
		if region.contains(x, y) {
			return region, true
		}
	}
	return paneRegion{}, false
}

func (m Model) inboxIndexAt(region paneRegion, _ int, y int) (int, bool) {
	contentY := y - region.y - 1
	if contentY < 0 || contentY >= max(1, region.h-paneFrame) {
		return 0, false
	}
	line := m.inboxVP.YOffset + contentY
	// Each inbox item: 2 content lines + 1 blank separator = 3 lines per item.
	idx := line / 3
	if idx < 0 || idx >= len(m.prs) {
		return 0, false
	}
	// Separator line (line%3 == 2) clicks only focus the pane.
	if line%3 == 2 {
		return 0, false
	}
	return idx, true
}

func (m Model) handleMouseClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	region, ok := m.paneRegionAt(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	m.focus = region.focus
	switch region.focus {
	case FocusInbox:
		m.mode = ModeInbox
		if idx, ok := m.inboxIndexAt(region, msg.X, msg.Y); ok {
			m.selected = idx
			m.resizeViewports()
			m.resetPRViewState()
			return m, m.autoDiffCmd()
		}
	case FocusSummary:
		if _, ok := m.current(); ok {
			m.mode = ModeDetail
		}
	case FocusDiff:
		if m.diffEnabled || m.mode == ModeDiff {
			m.mode = ModeDiff
		}
	}
	m.resizeViewports()
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.mode {
	case ModeComment:
		return m.handleCommentKey(key)
	case ModeConfirmApprove:
		return m.handleApproveKey(key)
	case ModeCommand:
		return m.handleCommandKey(key)
	case ModeConfirmCommand:
		return m.handleConfirmCommandKey(key)
	case ModeColorscheme:
		return m.handleColorschemeKey(key)
	case ModeHelp:
		if key == "esc" || key == "?" || key == "enter" {
			m.mode = m.prevMode
		}
		return m, nil
	case ModeSettings:
		return m.handleSettingsKey(key)
	case ModeDiff:
		if key == "esc" || key == "enter" || key == "v" {
			m.diffEnabled = false
			m.mode = ModeDetail
			m.focus = FocusSummary
			return m, nil
		}
	}

	if key == "q" || key == "ctrl+c" {
		return m, tea.Quit
	}

	if key == "?" {
		m.prevMode = m.mode
		m.mode = ModeHelp
		return m, nil
	}
	if m.setSummaryMode(key) {
		m.summaryVP.GotoTop()
		m.commitIndex = 0
		m.summaryXOffset = 0
		m.commitDiffKey = ""
		m.renderedCommitDiff = ""
		m.syncSummaryVP()
		m.syncDiffVP()
		if m.summaryMode == SummaryModeCommits {
			return m, m.autoCommitDiffCmd()
		}
		return m, nil
	}

	switch key {
	case "tab":
		m.nextFocus()
	case "shift+tab":
		m.prevFocus()
	case "g":
		switch m.focus {
		case FocusSummary:
			m.summaryVP.GotoTop()
		case FocusDiff:
			m.diffVP.GotoTop()
		default: // FocusInbox
			if m.selected != 0 {
				m.selected = 0
				m.resetPRViewState()
				return m, m.autoDiffCmd()
			}
		}
	case "G":
		switch m.focus {
		case FocusSummary:
			m.summaryVP.GotoBottom()
		case FocusDiff:
			m.diffVP.GotoBottom()
		default: // FocusInbox
			last := len(m.prs) - 1
			if last >= 0 && m.selected != last {
				m.selected = last
				m.resetPRViewState()
				return m, m.autoDiffCmd()
			}
		}
	case "o":
		if pr, ok := m.current(); ok {
			return m, openPRURLCmd(prURL(pr))
		}
	case "C":
		if m.focus == FocusInbox {
			if pr, ok := m.current(); ok {
				return m, tea.Batch(m.finishStatus("copied URL", false), copyPRURLCmd(prURL(pr)))
			}
		}
	case "f":
		m.prevMode = m.mode
		m.mode = ModeSettings
		m.settingsCursor = 0
	case "j", "down":
		switch m.focus {
		case FocusSummary:
			if m.summaryMode == SummaryModeCommits {
				if pr, ok := m.current(); ok && m.commitIndex < len(pr.Commits)-1 {
					m.commitIndex++
					m.syncSummaryVP()
					m.syncSummaryVPScroll()
					return m, m.autoCommitDiffCmd()
				}
			} else if m.summaryMode == SummaryModeFiles {
				if pr, ok := m.current(); ok && m.fileIndex < len(pr.Files)-1 {
					m.fileIndex++
					m.diffVP.GotoTop()
					m.syncDiffVP()
					m.syncSummaryVP()
					m.syncSummaryVPScroll()
				}
			} else {
				m.summaryVP.LineDown(1)
			}
		case FocusDiff:
			m.diffVP.LineDown(1)
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
			if m.summaryMode == SummaryModeCommits {
				if m.commitIndex > 0 {
					m.commitIndex--
					m.syncSummaryVP()
					m.syncSummaryVPScroll()
					return m, m.autoCommitDiffCmd()
				}
			} else if m.summaryMode == SummaryModeFiles {
				if m.fileIndex > 0 {
					m.fileIndex--
					m.diffVP.GotoTop()
					m.syncDiffVP()
					m.syncSummaryVP()
					m.syncSummaryVPScroll()
				}
			} else {
				m.summaryVP.LineUp(1)
			}
		case FocusDiff:
			m.diffVP.LineUp(1)
		default: // FocusInbox
			if m.selected > 0 {
				m.selected--
				m.resetPRViewState()
				return m, m.autoDiffCmd()
			}
		}
	case "left":
		if m.focus == FocusDiff {
			m.diffXOffset = max(0, m.diffXOffset-max(1, m.diffVP.Width/2))
		} else if m.focus == FocusSummary && m.summaryMode == SummaryModeCommits {
			m.summaryXOffset = max(0, m.summaryXOffset-max(1, m.summaryVP.Width/2))
		}
	case "right":
		if m.focus == FocusDiff {
			m.diffXOffset = min(m.maxDiffXOffset(), m.diffXOffset+max(1, m.diffVP.Width/2))
		} else if m.focus == FocusSummary && m.summaryMode == SummaryModeCommits {
			m.summaryXOffset = m.summaryXOffset + max(1, m.summaryVP.Width/2)
		}
	case "pgdown", " ":
		switch m.focus {
		case FocusSummary:
			m.summaryVP.ViewDown()
		case FocusDiff:
			m.diffVP.ViewDown()
		case FocusInbox:
			m.inboxVP.ViewDown()
		}
	case "pgup":
		switch m.focus {
		case FocusSummary:
			m.summaryVP.ViewUp()
		case FocusDiff:
			m.diffVP.ViewUp()
		case FocusInbox:
			m.inboxVP.ViewUp()
		}
	case "shift+down":
		if m.summaryMode == SummaryModeCommits {
			if pr, ok := m.current(); ok && m.commitIndex < len(pr.Commits)-1 {
				m.commitIndex++
				m.syncSummaryVP()
				m.syncSummaryVPScroll()
				return m, m.autoCommitDiffCmd()
			}
		} else if len(m.diffFiles) > 0 && m.fileIndex < len(m.diffFiles)-1 {
			m.fileIndex++
			m.diffVP.GotoTop()
			m.syncDiffVP()
			m.syncSummaryVP()
			m.syncSummaryVPScroll()
		} else if pr, ok := m.current(); ok && m.fileIndex < len(pr.Files)-1 {
			m.fileIndex++
			m.diffVP.GotoTop()
			m.syncDiffVP()
			m.syncSummaryVP()
			m.syncSummaryVPScroll()
		}
	case "shift+up":
		if m.summaryMode == SummaryModeCommits {
			if m.commitIndex > 0 {
				m.commitIndex--
				m.syncSummaryVP()
				m.syncSummaryVPScroll()
				return m, m.autoCommitDiffCmd()
			}
		} else if m.fileIndex > 0 {
			m.fileIndex--
			m.diffVP.GotoTop()
			m.syncDiffVP()
			m.syncSummaryVP()
			m.syncSummaryVPScroll()
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
			if !m.aiEnabled {
				return m, m.finishStatus("AI review not configured", true)
			}
			if pr, ok := m.current(); ok {
				m.loading = true
				m.message = "re-running analysis"
				m.aiAnalyzing = true
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
			m.syncDiffVP()
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
			diffInnerW := mv.paneInnerWidth(mv.diffPaneOuterWidth(), FocusDiff)
			return m, tea.Batch(m.startStatus("fetching diff…"), diffCmd(m.app, pr, diffRenderWidth(diffInnerW)))
		}
		// Wide/medium: toggle diff pane content.
		m.diffEnabled = !m.diffEnabled
		if m.diffEnabled {
			if m.mode == ModeInbox {
				m.mode = ModeDetail
			}
			m.syncDiffVP()
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
			m.syncSummaryVPScroll()
		} else if pr, ok := m.current(); ok && m.fileIndex < len(pr.Files)-1 {
			m.fileIndex++
			m.diffVP.GotoTop()
			m.syncDiffVP()
			m.syncSummaryVP()
			m.syncSummaryVPScroll()
		}
	case "p", "[":
		if m.fileIndex > 0 {
			m.fileIndex--
			m.diffVP.GotoTop()
			m.syncDiffVP()
			m.syncSummaryVP()
			m.syncSummaryVPScroll()
		}
	case "s":
		if m.rawDiff != "" && (m.mode == ModeDiff || m.focus == FocusDiff) {
			m.sideBySide = !m.sideBySide
			pr, _ := m.current()
			mv := Model{width: m.width, height: m.height, theme: m.theme, focus: m.focus, prs: m.prs, selected: m.selected}
			diffInnerW := mv.paneInnerWidth(mv.diffPaneOuterWidth(), FocusDiff)
			return m, renderDiffCmd(m.rawDiff, pr.Repo, pr.Number, diffRenderWidth(diffInnerW), m.sideBySide)
		}
	case "x":
		if !m.diffEnabled {
			return m, nil
		}
		if m.diffMode == DiffModeFile {
			m.diffMode = DiffModeHunks
			m.diffVP.GotoTop()
			m.syncDiffVP()
			return m, nil
		}
		pr, ok := m.current()
		if !ok || len(pr.Files) == 0 || m.fileIndex < 0 || m.fileIndex >= len(pr.Files) {
			return m, nil
		}
		if strings.EqualFold(pr.Files[m.fileIndex].Status, "removed") {
			return m, m.finishStatus("file removed in PR", true)
		}
		if strings.TrimSpace(pr.HeadRefOID) == "" {
			return m, m.finishStatus("missing PR head ref", true)
		}
		m.diffMode = DiffModeFile
		m.diffVP.GotoTop()
		m.syncDiffVP()
		key := m.selectedFileContentKey()
		if m.fileContentKey == key && m.fileContent != "" {
			return m, nil
		}
		path := pr.Files[m.fileIndex].Path
		return m, tea.Batch(m.startStatus("fetching file…"), fileContentCmd(m.app.GitHub, pr.Repo, path, pr.HeadRefOID, key))
	case "a":
		if _, ok := m.current(); ok {
			m.mode = ModeConfirmApprove
		}
	case "A":
		if pr, ok := m.current(); ok {
			m.loading = true
			return m, tea.Batch(m.startStatus("approving…"), approveCmd(m.app, pr))
		}
	case "c":
		if _, ok := m.current(); ok {
			m.comment = ""
			m.mode = ModeComment
		}
	case ":":
		m.commandInput = ""
		m.commandCursor = 0
		m.prevMode = m.mode
		m.mode = ModeCommand
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
	diffInnerW := mv.paneInnerWidth(mv.diffPaneOuterWidth(), FocusDiff)
	return diffCmd(m.app, pr, diffRenderWidth(diffInnerW))
}

// autoCommitDiffCmd fetches the diff for the currently selected commit when
// diff is enabled and the commit has not already been loaded.
func (m *Model) autoCommitDiffCmd() tea.Cmd {
	if !m.diffEnabled {
		return nil
	}
	pr, ok := m.current()
	if !ok {
		return nil
	}
	if m.commitIndex < 0 || m.commitIndex >= len(pr.Commits) {
		return nil
	}
	sha := pr.Commits[m.commitIndex].OID
	if m.commitDiffKey == sha {
		return nil
	}
	m.commitDiffKey = sha
	m.renderedCommitDiff = ""
	m.diffVP.GotoTop()
	mv := Model{width: m.width, height: m.height, theme: m.theme, focus: m.focus, prs: m.prs, selected: m.selected}
	diffInnerW := mv.paneInnerWidth(mv.diffPaneOuterWidth(), FocusDiff)
	return commitDiffCmd(m.app.GitHub, pr.Repo, sha, diffRenderWidth(diffInnerW))
}

func (m *Model) resizeViewports() {
	// Set the stored viewport heights so that vpCanScroll and LineDown/LineUp
	// work correctly between renders. Header and footer each occupy 1 line.
	// Pane border adds 2 lines vertically. These constants mirror the layout
	// functions (wideLayout / mediumLayout) and the pane frame size (2).
	const headerLines = 1
	const footerLines = 0
	bodyH := max(4, m.height-headerLines-footerLines)
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
	// Update diff viewport width to the real visible diff content width.
	diffInnerW := mv.paneInnerWidth(mv.diffPaneOuterWidth(), FocusDiff)
	m.diffVP.Width = mv.diffContentWidth(diffInnerW)
	m.diffVP.SetHorizontalStep(max(1, m.diffVP.Width/2))
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

func openPRURLCmd(url string) tea.Cmd {
	if strings.TrimSpace(url) == "" {
		return nil
	}
	name := "xdg-open"
	args := []string{url}
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	}
	return tea.ExecProcess(exec.Command(name, args...), func(err error) tea.Msg {
		if err != nil {
			return pagerMsg{err: err}
		}
		return pagerMsg{message: "opened PR in browser"}
	})
}

func copyPRURLCmd(url string) tea.Cmd {
	if url == "" {
		return nil
	}
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("pbcopy")
		case "windows":
			cmd = exec.Command("clip")
		default:
			// Linux: try xclip first, fall back to xsel.
			if _, err := exec.LookPath("xclip"); err == nil {
				cmd = exec.Command("xclip", "-selection", "clipboard")
			} else {
				cmd = exec.Command("xsel", "--clipboard", "--input")
			}
		}
		cmd.Stdin = strings.NewReader(url)
		if err := cmd.Run(); err != nil {
			return actionMsg{message: "copy failed: " + err.Error(), err: err}
		}
		return actionMsg{message: "copied URL"}
	}
}

func prURL(pr model.PullRequest) string {
	if strings.TrimSpace(pr.URL) != "" {
		return pr.URL
	}
	if pr.Repo == "" || pr.Number == 0 {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/pull/%d", pr.Repo, pr.Number)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func (m *Model) resetPRViewState() {
	m.diff = ""
	m.rawDiff = ""
	m.diffFiles = nil
	m.renderedFiles = nil
	m.diffMode = DiffModeHunks
	m.fileContent = ""
	m.fileContentKey = ""
	m.fileIndex = 0
	m.commitIndex = 0
	m.diffLoadKey = ""
	m.commitDiffKey = ""
	m.renderedCommitDiff = ""
	m.diffXOffset = 0
	m.summaryXOffset = 0
	m.inboxVP.GotoTop()
	m.summaryVP.GotoTop()
	m.diffVP.GotoTop()
	m.syncSummaryVP()
	m.syncDiffVP()
}

// syncSummaryVPScroll adjusts the summary viewport Y offset to keep the
// selected item visible when in commits or files mode.
func (m *Model) syncSummaryVPScroll() {
	vpH := m.summaryVP.VisibleLineCount()
	if vpH <= 0 {
		return
	}
	switch m.summaryMode {
	case SummaryModeCommits:
		// Each commit renders as 2 lines (headline + meta).
		selectedLine := m.commitIndex * 2
		if selectedLine < m.summaryVP.YOffset {
			m.summaryVP.SetYOffset(selectedLine)
		} else if selectedLine+1 >= m.summaryVP.YOffset+vpH {
			m.summaryVP.SetYOffset(selectedLine + 2 - vpH)
		}
	case SummaryModeFiles:
		// Each file renders as 1 line.
		selectedLine := m.fileIndex
		if selectedLine < m.summaryVP.YOffset {
			m.summaryVP.SetYOffset(selectedLine)
		} else if selectedLine >= m.summaryVP.YOffset+vpH {
			m.summaryVP.SetYOffset(selectedLine - vpH + 1)
		}
	}
}

// syncSummaryVP updates the stored summaryVP's content so that TotalLineCount
// is accurate for the scroll percentage indicator. Width is set to the real
// summary inner width; content is hard-clipped so line count is correct.
func (m *Model) syncSummaryVP() {
	pr, ok := m.current()
	if !ok {
		return
	}
	mv := Model{
		theme:       m.theme,
		fileIndex:   m.fileIndex,
		commitIndex: m.commitIndex,
		summaryMode: m.summaryMode,
		width:       m.width,
		height:      m.height,
		focus:       m.focus,
		prs:         m.prs,
		selected:    m.selected,
	}
	_, summaryInnerW, _ := mv.paneInnerWidths()
	content := mv.summaryContent(pr, summaryInnerW)
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
	content := m.currentDiffContent()
	// Use the real visible diff content width so horizontal cut matches render.
	mv := Model{width: m.width, height: m.height, theme: m.theme, focus: m.focus, prs: m.prs, selected: m.selected}
	diffInnerW := mv.paneInnerWidth(mv.diffPaneOuterWidth(), FocusDiff)
	vpW := mv.diffContentWidth(diffInnerW)
	m.diffVP.Width = vpW
	m.diffVP.SetHorizontalStep(max(1, vpW/2))
	m.diffVP.SetContent(content)
	m.diffXOffset = min(m.diffXOffset, m.maxDiffXOffset())
}

func (m Model) currentDiffContent() string {
	if m.diffMode == DiffModeFile {
		return m.currentFileContent()
	}
	// In commits mode show the selected commit's diff (if loaded).
	if m.summaryMode == SummaryModeCommits {
		if m.renderedCommitDiff != "" {
			return m.renderedCommitDiff
		}
		// Commit diff not yet loaded — show the full PR diff as fallback,
		// or a placeholder if that's also not loaded.
		if m.diff != "" {
			return m.diff
		}
		return ""
	}
	content := m.diff
	if len(m.renderedFiles) == 0 {
		return content
	}
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
	return m.renderedFiles[idx].Body
}

func (m Model) selectedFileContentKey() string {
	pr, ok := m.current()
	if !ok || m.fileIndex < 0 || m.fileIndex >= len(pr.Files) {
		return ""
	}
	return fmt.Sprintf("%s#%d:%s@%s", pr.Repo, pr.Number, pr.Files[m.fileIndex].Path, pr.HeadRefOID)
}

func (m Model) currentFileContent() string {
	if m.fileContentKey != m.selectedFileContentKey() || m.fileContent == "" {
		return ""
	}
	return m.renderFullFileContent(m.fileContent, diffFileMarks(m.selectedRawDiffFile()))
}

func (m Model) selectedRawDiffFile() string {
	if len(m.diffFiles) == 0 {
		return ""
	}
	if pr, ok := m.current(); ok && m.fileIndex >= 0 && m.fileIndex < len(pr.Files) {
		want := pr.Files[m.fileIndex].Path
		for _, f := range m.diffFiles {
			if f.Path == want {
				return f.Body
			}
		}
	}
	if m.fileIndex >= 0 && m.fileIndex < len(m.diffFiles) {
		return m.diffFiles[m.fileIndex].Body
	}
	return ""
}

func (m Model) maxDiffXOffset() int {
	return max(0, longestLineWidth(m.currentDiffContent())-m.diffVP.Width)
}

func (m *Model) setSummaryMode(key string) bool {
	switch key {
	case "1":
		m.summaryMode = SummaryModeFiles
	case "2":
		m.summaryMode = SummaryModeCommits
	case "3":
		m.summaryMode = SummaryModeComments
	case "4":
		m.summaryMode = SummaryModeAtlantis
	default:
		return false
	}
	return true
}

func diffRenderWidth(visibleWidth int) int {
	return max(visibleWidth*100, 10000)
}

func (m *Model) cycleSortMode() {
	current := 0
	if pr, ok := m.current(); ok {
		current = pr.Number
	}
	m.sortMode = m.sortMode.Next()
	m.applyPRSettings()
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

func (m *Model) applyPRSettings() {
	current := 0
	if pr, ok := m.current(); ok {
		current = pr.Number
	}
	source := m.allPRs
	if source == nil {
		source = m.prs
	}
	m.prs = sortPRs(filterPRs(source, m.hideDrafts), m.sortMode)
	if len(m.prs) == 0 {
		m.selected = 0
		m.resetPRViewState()
		return
	}
	if current != 0 {
		for i, pr := range m.prs {
			if pr.Number == current {
				m.selected = i
				return
			}
		}
	}
	if m.selected >= len(m.prs) {
		m.selected = len(m.prs) - 1
	}
	if m.selected < 0 {
		m.selected = 0
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

func filterPRs(prs []model.PullRequest, hideDrafts bool) []model.PullRequest {
	if !hideDrafts {
		return append([]model.PullRequest(nil), prs...)
	}
	out := make([]model.PullRequest, 0, len(prs))
	for _, pr := range prs {
		if !pr.Draft {
			out = append(out, pr)
		}
	}
	return out
}

func (m Model) handleSettingsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "f":
		m.mode = m.prevMode
		return m, nil
	case "j", "down":
		if m.settingsCursor < 1 {
			m.settingsCursor++
		}
	case "k", "up":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case " ", "enter", "right", "l":
		if m.settingsCursor == 0 {
			m.hideDrafts = !m.hideDrafts
			m.applyPRSettings()
			m.message = draftFilterMessage(m.hideDrafts)
		} else {
			m.cycleSortMode()
		}
	case "left", "h":
		if m.settingsCursor == 0 {
			m.hideDrafts = !m.hideDrafts
			m.applyPRSettings()
			m.message = draftFilterMessage(m.hideDrafts)
		} else {
			m.cycleSortMode()
		}
	}
	return m, nil
}

func draftFilterMessage(hideDrafts bool) string {
	if hideDrafts {
		return "drafts: hidden"
	}
	return "drafts: shown"
}

func (m Model) handleColorschemeKey(key string) (tea.Model, tea.Cmd) {
	palettes := uitheme.AllPalettes()
	switch key {
	case "esc", "q":
		// Revert to original theme (re-resolve from saved config).
		m.theme = uitheme.DefaultWithName(m.cfg.Theme)
		m.mode = m.prevMode
		return m, nil
	case "j", "down":
		if m.colorschemeIndex < len(palettes)-1 {
			m.colorschemeIndex++
			m.theme = uitheme.New(palettes[m.colorschemeIndex], m.theme.Profile, m.theme.NoColor)
		}
	case "k", "up":
		if m.colorschemeIndex > 0 {
			m.colorschemeIndex--
			m.theme = uitheme.New(palettes[m.colorschemeIndex], m.theme.Profile, m.theme.NoColor)
		}
	case "enter", " ":
		// Confirm: save to config.
		chosen := palettes[m.colorschemeIndex]
		m.cfg.Theme = chosen.Name
		m.theme = uitheme.New(chosen, m.theme.Profile, m.theme.NoColor)
		m.mode = m.prevMode
		return m, tea.Batch(m.finishStatus("theme: "+chosen.Name, false), saveThemeCmd(m.cfg, chosen.Name))
	}
	return m, nil
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

func (m Model) handleCommandKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		m.mode = m.prevMode
		m.commandInput = ""
		m.commandCursor = 0
		return m, nil
	}
	if key == "enter" {
		cmd, ok := exactCommand(m.commandInput)
		if !ok {
			// No exact match; try cursor-selected command from matching list
			matches := matchingCommands(m.commandInput)
			if len(matches) == 0 {
				return m, nil
			}
			idx := m.commandCursor
			if idx < 0 {
				idx = 0
			}
			if idx >= len(matches) {
				idx = len(matches) - 1
			}
			cmd = matches[idx]
			ok = true
		}
		if !ok {
			return m, nil
		}
		if cmd.RequireConfirm {
			m.pendingCmd = cmd
			m.mode = ModeConfirmCommand
			return m, nil
		}
		return m.executeCommand(cmd)
	}
	if key == "down" || key == "ctrl+j" {
		matches := matchingCommands(m.commandInput)
		if len(matches) > 0 {
			m.commandCursor++
			if m.commandCursor >= len(matches) {
				m.commandCursor = len(matches) - 1
			}
		}
		return m, nil
	}
	if key == "up" || key == "ctrl+k" {
		m.commandCursor--
		if m.commandCursor < 0 {
			m.commandCursor = 0
		}
		return m, nil
	}
	if key == "backspace" || key == "ctrl+h" {
		if len(m.commandInput) > 0 {
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
		}
		m.commandCursor = 0
		return m, nil
	}
	// Accept printable single characters (no spaces)
	if len(key) == 1 && key != " " {
		m.commandInput += key
		m.commandCursor = 0
	}
	return m, nil
}

func (m Model) handleConfirmCommandKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "n" {
		m.mode = m.prevMode
		m.commandInput = ""
		return m, nil
	}
	if key == "y" || key == "enter" {
		return m.executeCommand(m.pendingCmd)
	}
	return m, nil
}

func (m Model) executeCommand(cmd viCommand) (tea.Model, tea.Cmd) {
	// Commands that do not require a selected PR.
	if cmd.Name == "colorscheme" {
		m.commandInput = ""
		palettes := uitheme.AllPalettes()
		m.colorschemeIndex = 0
		for i, p := range palettes {
			if strings.EqualFold(p.Name, m.theme.Palette.Name) {
				m.colorschemeIndex = i
				break
			}
		}
		m.mode = ModeColorscheme
		return m, nil
	}

	pr, ok := m.current()
	if !ok {
		m.mode = m.prevMode
		m.commandInput = ""
		return m, nil
	}
	m.mode = m.prevMode
	m.commandInput = ""

	switch cmd.Name {
	case "ap":
		return m, tea.Batch(m.startStatus("commenting…"), commentCmd(m.app, pr, "atlantis plan"))
	case "aa":
		return m, tea.Batch(m.startStatus("commenting…"), commentCmd(m.app, pr, "atlantis apply"))
	case "au":
		return m, tea.Batch(m.startStatus("commenting…"), commentCmd(m.app, pr, "atlantis unlock"))
	case "draft":
		action := "marking ready…"
		if !pr.Draft {
			action = "marking draft…"
		}
		return m, tea.Batch(m.startStatus(action), toggleDraftCmd(m.app, pr))
	case "close":
		return m, tea.Batch(m.startStatus("closing…"), closePRCmd(m.app, pr, ""))
	case "update":
		return m, tea.Batch(m.startStatus("updating branch…"), updateBranchCmd(m.app, pr))
	case "block":
		return m, tea.Batch(m.startStatus("requesting changes…"), requestChangesCmd(m.app, pr, ""))
	case "merge":
		return m, tea.Batch(m.startStatus("merging…"), mergePRCmd(m.app, pr))
	}
	return m, nil
}
