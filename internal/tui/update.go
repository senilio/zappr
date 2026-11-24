package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/model"
	"github.com/senilio/zappr/internal/plan"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Wheel input only changes viewport offsets. It cannot invalidate delta's
	// layout, so do not inspect or lay out unrelated diff content here.
	if _, ok := msg.(tea.MouseWheelMsg); ok {
		return m.update(msg)
	}
	oldRaw := m.sideLayoutRaw()
	next, cmd := m.update(msg)
	n := next.(Model)
	n.leaveDescriptionOnNavigation(m, msg)
	// Keep stored content current for scrolling and the pre-Update mouse filter.
	if n.focus != m.focus || n.mode != m.mode {
		n.resizeViewports()
	} else {
		n.syncInboxVP()
		n.syncSummaryVP()
		n.syncDiffVP()
	}
	anchor := 0
	if n.sideLayoutNeeded(n.sideLayoutKey()) {
		anchor = m.diffAnchor()
		if raw := n.sideLayoutRaw(); oldRaw != "" && raw != "" && raw != oldRaw {
			anchor = n.diffAnchor()
		}
	}
	if layout := n.sideLayoutCmd(anchor); layout != nil {
		return n, tea.Batch(cmd, layout)
	}
	return n, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.theme = m.theme.WithBackground(msg.IsDark())
		m.resizeViewports()
		return m, nil
	case tea.ColorProfileMsg:
		m.theme = uitheme.New(m.theme.Palette, msg.Profile, msg.Profile <= colorprofile.ASCII, m.theme.IsDark)
		m.resizeViewports()
		return m, nil
	case tea.PasteMsg:
		// Pasted text is input, never a shortcut or confirmation.
		switch m.mode {
		case ModeComment:
			m.comment += msg.Content
		case ModeCommand:
			m.commandInput += strings.ReplaceAll(strings.ReplaceAll(msg.Content, "\r", ""), "\n", " ")
			m.commandCursor = 0
		case ModeHelp:
			m.helpInput += strings.ReplaceAll(strings.ReplaceAll(msg.Content, "\r", ""), "\n", " ")
			m.syncHelpVP()
		}
		return m, nil
	case sideLayoutMsg:
		if msg.key == m.sideDiffRequested {
			m.sideDiffRequested = ""
		}
		if msg.key != m.sideLayoutKey() {
			return m, nil
		}
		if msg.err != nil {
			// Settle failed requests without automatically retrying on every event.
			m.sideDiffFailed = msg.key
			return m, nil
		}
		anchor := msg.anchor
		if m.diffVP.YOffset() != msg.offset {
			// Navigation while formatting takes precedence over the request anchor.
			anchor = m.diffAnchor()
		}
		m.sideDiffKey, m.sideDiffContent = msg.key, msg.content
		m.sideFileStarts = msg.fileStarts
		m.syncDiffVP()
		m.restoreDiffAnchor(anchor)
		return m, nil
	case refreshMsg:
		current := msg.id == m.inboxRequestID
		latest := current && !errors.Is(msg.err, app.ErrRefreshSuperseded)
		var selection tea.Cmd
		changed := false
		for _, observation := range msg.updates {
			if observation.Closed || (latest && msg.err == nil) {
				changed = m.applyPRUpdate(observation) || changed
			}
		}
		if changed {
			selection = m.reconcilePRUpdates()
		}
		if current {
			m.inboxRefreshing = false
		}
		if latest {
			m.inboxRefreshErr = msg.err
			if msg.err == nil {
				m.lastInboxRefresh = timeNow()
			}
		}
		return m, selection

	case refreshPRMsg:
		if (msg.update.Closed || msg.err == nil) && m.applyPRUpdate(msg.update) {
			return m, m.reconcilePRUpdates()
		}
		if msg.err != nil {
			m.failPlanOpen(msg.requestKey, msg.requestHead, 0, msg.err)
		}
		return m, nil

	case prActionResult:
		cmd := m.handlePRActionResult(msg)
		return m, cmd

	case commitDiffMsg:
		if msg.request != m.commitDiffRequest || msg.sha != m.commitDiffKey {
			return m, nil
		}
		m.commitDiffRequest = 0
		if msg.err != nil {
			m.commitDiffErr = "commit diff error: " + msg.err.Error()
			return m, nil
		}
		m.commitDiffErr = ""
		m.renderedCommitDiff = msg.rendered
		m.rawCommitDiff = msg.raw
		m.commitFileStarts = msg.fileStarts
		m.diffVP.GotoTop()
		m.syncDiffVP()
		return m, nil

	case fileContentMsg:
		if msg.request != m.fileRequest {
			return m, m.completeStatus(msg.owner, "", false)
		}
		m.fileRequest = 0
		if msg.key != m.selectedFileContentKey() || !m.diffEnabled || m.diffMode != DiffModeFile {
			return m, m.completeStatus(msg.owner, "", false)
		}
		if msg.err != nil {
			m.fileLoadErr, m.fileLoadErrKey = "file error: "+msg.err.Error(), msg.key
			return m, m.completeStatus(msg.owner, m.fileLoadErr, true)
		}
		m.fileLoadErr = ""
		m.fileContentKey = msg.key
		m.fileContent = msg.content
		m.diffVP.GotoTop()
		m.syncDiffVP()
		return m, m.completeStatus(msg.owner, "", false)

	case diffMsg:
		// Ignore stale diff results that no longer match the selected PR.
		if msg.request != m.diffRequest {
			return m, m.completeStatus(msg.owner, "", false)
		}
		m.diffRequest = 0
		pr, ok := m.current()
		if !ok || msg.repo != pr.Repo || msg.number != pr.Number || msg.key != prRefreshKey(pr) {
			return m, m.completeStatus(msg.owner, "", false)
		}
		if msg.err != nil {
			m.diffLoadErr = "diff error: " + msg.err.Error()
			return m, m.completeStatus(msg.owner, m.diffLoadErr, true)
		}
		m.diffLoadErr = ""
		m.rawDiff = msg.raw
		m.diff = trimDiff(msg.rendered)
		m.diffFiles = msg.files
		m.renderedFiles = msg.renderedFiles
		m.diffFileStarts = msg.fileStarts
		m.diffXOffset = 0
		m.diffVP.GotoTop()
		m.syncDiffVP()
		return m, m.completeStatus(msg.owner, "", false)

	case tea.WindowSizeMsg:
		diffAnchor, summaryAnchor := m.diffAnchor(), m.summaryAnchor()
		inboxAnchor := m.inboxAnchor()
		if m.width != msg.Width || m.height != msg.Height {
			m.prConfirmationPage = 0 // Reflow can change which lines were shown.
		}
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewports()
		m.restoreDiffAnchor(diffAnchor)
		m.restoreSummaryAnchor(summaryAnchor)
		m.restoreInboxAnchor(inboxAnchor)
		return m, nil

	case clipboardMsg:
		if m.statusPending {
			return m, nil
		}
		if msg.err != nil {
			m.error = "copy failed: " + msg.err.Error()
			return m, m.finishStatus(m.error, true)
		}
		m.error = ""
		return m, m.finishStatus("copied URL", false)
	case closePRMsg:
		changed := m.applyPRUpdate(msg.update)
		var selection, refresh tea.Cmd
		if changed {
			selection = m.reconcilePRUpdates()
		}
		if msg.update.Closed {
			refresh = m.startInboxRefresh()
		}
		if msg.err != nil {
			return m, tea.Batch(selection, refresh, m.completeStatus(msg.owner, "error: "+msg.err.Error(), true))
		}
		return m, tea.Batch(selection, refresh, m.completeStatus(msg.owner, fmt.Sprintf("closed %s#%d", msg.key.Repo, msg.key.Number), false))
	case actionMsg:
		if msg.err != nil {
			if m.ownsStatus(msg.owner) {
				m.error = msg.err.Error()
			}
			return m, m.completeStatus(msg.owner, "error: "+msg.err.Error(), true)
		}
		if m.ownsStatus(msg.owner) {
			m.error, m.message = "", msg.message
		}
		refresh := m.startInboxRefresh()
		if refresh != nil && msg.refreshPR != nil {
			// Refresh details last so an older list snapshot cannot overwrite
			// the reviewers returned after the review action.
			refresh = tea.Sequence(refresh, refreshPRCmd(m.app, *msg.refreshPR))
		}
		return m, tea.Batch(m.completeStatus(msg.owner, msg.message, false), refresh)
	case configReloadedMsg:
		cmd := m.acceptConfigReload(msg)
		return m, cmd
	case configWatchTickMsg:
		return m, m.checkConfig()
	case configWatchCheckedMsg:
		var cmd tea.Cmd
		if msg.result != nil {
			cmd = m.acceptConfigReload(*msg.result)
		}
		return m, tea.Batch(cmd, configWatchTickCmd())
	case settingsSavedMsg:
		if m.settingsPending > 0 {
			m.settingsPending--
		}
		if msg.err != nil {
			m.quitPending = false
			if m.statusPending {
				return m, nil
			}
			m.error = "failed to save settings: " + msg.err.Error()
			return m, m.finishStatus(m.error, true)
		}
		if m.quitPending && m.settingsPending == 0 {
			return m, tea.Quit
		}
		return m, nil

	case pagerMsg:
		if m.statusPending {
			return m, nil
		}
		if msg.err != nil {
			m.error = msg.err.Error()
			return m, nil
		}
		m.error = ""
		m.message = msg.message
		return m, nil

	case statusDismissMsg:
		if msg.token == m.statusToken {
			m.clearStatus()
		}
		return m, nil

	case tickMsg:
		if msg.generation != m.pollGeneration {
			return m, nil
		}
		if m.inboxRefreshing {
			return m, tickCmd(m.pollEvery, m.pollGeneration)
		}
		refresh := m.startInboxRefresh()
		if pr, ok := m.current(); ok && refresh != nil {
			return m, tea.Sequence(refresh, refreshPRCmd(m.app, pr), tickCmd(m.pollEvery, m.pollGeneration))
		}
		return m, tea.Sequence(refresh, tickCmd(m.pollEvery, m.pollGeneration))

	case uiTickMsg:
		m.expirePRActionNotices(timeNow())
		// Re-render every second so the Inbox refresh age stays live.
		return m, uiTickCmd()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		if m.mode == ModeConfirmAtlantis {
			// Clicks must not dismiss the guard dialog; it is answered by key only.
			return m, nil
		}
		if msg.Button == tea.MouseLeft {
			return m.handleMouseClick(msg)
		}
	}

	var cmd tea.Cmd
	switch m.focus {
	case FocusInbox:
		if mouse, ok := msg.(tea.MouseWheelMsg); ok && (mouse.Button == tea.MouseWheelDown || mouse.Button == tea.MouseWheelUp) {
			m.syncInboxVP()
			m.inboxManualScroll = true
		}
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
	w := max(1, region.w-paneHFrame)
	line := m.inboxOffset(w, max(1, region.h-paneFrame)) + contentY
	_, spans := m.inboxContent(w)
	for i, span := range spans {
		if line >= span.start && line < span.end {
			return i, true
		}
	}
	return 0, false
}

func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.planState.pendingOpen != nil {
		m.cancelPlanOpen()
	}
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
			return m, m.selectionCmd()
		}
	case FocusSummary:
		if _, ok := m.current(); ok {
			m.mode = ModeDetail
		}
	case FocusDiff:
		if m.largePaneActive() || m.mode == ModeDiff {
			m.mode = ModeDiff
		}
	}
	m.resizeViewports()
	return m, nil
}

// isFullscreenDiff reports whether the diff occupies the whole screen rather
// than being one pane of a multi-pane layout. Only narrow layouts render the
// diff full-screen; at wider widths View falls through to layoutView and all
// three panes stay visible, so normal-context keys must keep working there.
// The width test matches the one toggle-diff uses to enter ModeDiff.
func (m Model) isFullscreenDiff() bool {
	return m.mode == ModeDiff && m.width > 0 && m.width < minMediumWidth
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "space" {
		key = " "
	}
	if m.consumeInvalidatedPRInput(key) {
		return m, nil
	}

	switch m.mode {
	case ModeComment:
		return m.handleCommentKey(key)
	case ModeConfirmApprove:
		return m.handleApproveKey(key)
	case ModeCommand:
		return m.handleCommandKey(key)
	case ModeConfirmCommand:
		return m.handleConfirmCommandKey(key)
	case ModeConfirmAtlantis:
		return m.handleConfirmAtlantisKey(key)
	case ModeColorscheme:
		return m.handleColorschemeKey(key)
	case ModeHelp:
		return m.handleHelpKey(key)
	case ModeSettings:
		return m.handleSettingsKey(key)
	}

	context := actionContextNormal
	if m.isFullscreenDiff() {
		context = actionContextDiff
	}
	action, ok := m.actions.actionForKey(context, key)
	if !ok {
		return m, nil
	}
	return m.triggerAction(action)
}

func (m Model) triggerAction(action tuiAction) (tea.Model, tea.Cmd) {
	action = m.selectedPRAction(action)
	pr, hasPR := m.actionPR()
	if action.RequiresPR {
		if !hasPR {
			if m.mode == ModeCommand && m.modalTarget.key.Number != 0 {
				m.invalidatePRModal()
				return m, m.finishStatus("action cancelled: original PR is no longer available", true)
			}
			return m, m.finishStatus("no PR selected", true)
		}
	}
	if action.ID == "confirm-action" {
		cmd := m.openPRActionConfirmation()
		return m, cmd
	}
	if action.Operation == "comment" && action.Comment != "" {
		if command := classifyAtlantisComment(action.Comment); command != atlantisCommandNone {
			if m.mode == ModeCommand {
				m.cancelPRModal()
			}
			cmd := m.enqueueAtlantisComment(pr, command, action.Comment, action.Confirmation != confirmationNone)
			return m, cmd
		}
	}
	if action.Confirmation != confirmationNone {
		m.cancelPlanOpen()
		if m.mode != ModeCommand {
			m.prevMode = m.mode
		}
		m.modalTarget = m.intentForPR(pr)
		m.pendingAction = action
		if action.Operation == "approve" {
			m.mode = ModeConfirmApprove
			return m, nil
		}
		if action.ID == "toggle-automerge" {
			m.pendingPR = pr
		}
		m.mode = ModeConfirmCommand
		return m, nil
	}
	return m.executeAction(action)
}

func (m Model) executeAction(action tuiAction) (tea.Model, tea.Cmd) {
	switch action.ID {
	case "focus-next", "focus-prev", "move-down", "move-up", "next-item", "prev-item", "go-top", "go-bottom", "page-down", "page-up", "inspect-pr", "help", "settings", "command-palette", "toggle-diff":
		if m.planState.pendingOpen != nil {
			m.cancelPlanOpen()
		}
	}
	if action.ID == "summary-plan" {
		cmd := m.openPlan()
		return m, cmd
	}
	if action.ID == "quit" {
		if m.settingsPending > 0 {
			m.quitPending = true
			return m, nil
		}
		return m, tea.Quit
	}
	if action.ID == "help" {
		m.prevMode = m.mode
		m.mode = ModeHelp
		m.helpInput = ""
		m.syncHelpVP()
		return m, nil
	}
	if action.ID == "diff-close" {
		m.diffEnabled = false
		m.mode = ModeDetail
		m.focus = FocusSummary
		return m, nil
	}
	fromModal := m.mode == ModeCommand || m.mode == ModeConfirmCommand || m.mode == ModeConfirmApprove
	pr, ok := m.actionPR()
	if fromModal && action.RequiresPR && !ok {
		m.invalidatePRModal()
		return m, m.finishStatus("action cancelled: original PR is no longer available", true)
	}
	if m.mode == ModeConfirmCommand && action.ID == "toggle-automerge" {
		pr = m.pendingPR
	}
	closedRevision := m.intentForPR(pr).closedRevision
	if fromModal {
		closedRevision = m.modalTarget.closedRevision
	}
	if fromModal {
		m.cancelPRModal()
	}
	if action.ID == "colorscheme" {
		palettes := uitheme.AllPalettes(m.theme.IsDark)
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
	if action.ID == "reload" {
		cmd := m.reloadConfig()
		return m, cmd
	}
	switch action.Operation {
	case "approve":
		owner := m.startStatus("approving…")
		return m, approveCmd(m.app, pr, owner, closedRevision)
	case "comment":
		if action.Comment == "" {
			m.cancelPlanOpen()
			m.comment = ""
			m.prevMode = m.mode
			m.modalTarget = m.intentForPR(pr)
			m.mode = ModeComment
			return m, nil
		}
		owner := m.startStatus("commenting…")
		return m, commentCmd(m.app, pr, action.Comment, owner, closedRevision)
	}
	switch action.ID {
	case "toggle-draft":
		status := "marking ready…"
		if !pr.Draft {
			status = "marking draft…"
		}
		owner := m.startStatus(status)
		return m, toggleDraftCmd(m.app, pr, owner, closedRevision)
	case "toggle-automerge":
		status := "enabling auto-merge…"
		if pr.AutoMerge {
			status = "disabling auto-merge…"
		}
		owner := m.startStatus(status)
		return m, toggleAutoMergeCmd(m.app, pr, owner, closedRevision)
	case "close-pr":
		owner := m.startStatus("closing…")
		return m, closePRCmd(m.app, pr, "", owner, closedRevision)
	case "update-branch":
		owner := m.startStatus("updating branch…")
		return m, updateBranchCmd(m.app, pr, owner, closedRevision)
	case "request-changes":
		owner := m.startStatus("requesting changes…")
		return m, requestChangesCmd(m.app, pr, "", owner, closedRevision)
	case "merge-pr":
		owner := m.startStatus("merging…")
		return m, mergePRCmd(m.app, pr, owner, closedRevision)
	}
	if m.setSummaryMode(action.ID) {
		m.summaryVP.GotoTop()
		m.commitIndex = 0
		m.reconcileComments()
		m.summaryXOffset = 0
		m.commitDiffKey = ""
		m.commitDiffRequest = 0
		m.commitDiffErr = ""
		m.renderedCommitDiff = ""
		m.rawCommitDiff = ""
		m.commitFileStarts = nil
		m.diffVP.GotoTop()
		m.diffXOffset = 0
		m.syncSummaryVP()
		m.syncDiffVP()
		if m.summaryMode == SummaryModeCommits {
			return m, m.autoCommitDiffCmd()
		}
		return m, nil
	}

	switch action.ID {
	case "toggle-wrap":
		switch m.focus {
		case FocusSummary:
			anchor := m.summaryAnchor()
			m.cfg.Wrap.Summary = !m.cfg.Wrap.Summary
			m.summaryXOffset = 0
			m.syncSummaryVP()
			m.restoreSummaryAnchor(anchor)
		case FocusDiff:
			anchor := m.diffAnchor()
			m.cfg.Wrap.Diff = !m.cfg.Wrap.Diff
			m.diffXOffset = 0
			m.syncDiffVP()
			m.restoreDiffAnchor(anchor)
		default:
			anchor := m.inboxAnchor()
			m.cfg.Wrap.Inbox = !m.cfg.Wrap.Inbox
			m.syncInboxVP()
			m.restoreInboxAnchor(anchor)
		}
		cmd := m.saveSettings()
		return m, cmd
	case "focus-next":
		m.nextFocus()
	case "focus-prev":
		m.prevFocus()
	case "go-top":
		switch m.focus {
		case FocusSummary:
			if m.summaryMode == SummaryModePlan {
				m.movePlan(-m.planNodeIndex())
			} else {
				m.summaryVP.GotoTop()
			}
		case FocusDiff:
			m.diffVP.GotoTop()
		default: // FocusInbox
			if m.selected != 0 {
				m.selected = 0
				m.resetPRViewState()
				return m, m.selectionCmd()
			}
		}
	case "go-bottom":
		switch m.focus {
		case FocusSummary:
			if m.summaryMode == SummaryModePlan {
				m.movePlan(len(m.planNodes()) - 1 - m.planNodeIndex())
			} else {
				m.summaryVP.GotoBottom()
			}
		case FocusDiff:
			m.diffVP.GotoBottom()
		default: // FocusInbox
			last := len(m.prs) - 1
			if last >= 0 && m.selected != last {
				m.selected = last
				m.resetPRViewState()
				return m, m.selectionCmd()
			}
		}
	case "open-pr":
		if pr, ok := m.current(); ok {
			return m, openPRURLCmd(prURL(pr))
		}
	case "copy-pr-url":
		return m, copyPRURLCmd(m.contextualCopyURL())
	case "settings":
		m.prevMode = m.mode
		m.mode = ModeSettings
		m.settingsCursor = 0
	case "move-down":
		switch m.focus {
		case FocusSummary:
			if m.summaryMode == SummaryModePlan {
				m.movePlan(1)
			} else if m.summaryMode == SummaryModeCommits {
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
				m.moveComment(1)
			}
		case FocusDiff:
			m.diffVP.ScrollDown(1)
		default: // FocusInbox
			if m.selected < len(m.prs)-1 {
				m.selected++
				m.resetPRViewState()
				return m, m.selectionCmd()
			}
		}
	case "move-up":
		switch m.focus {
		case FocusSummary:
			if m.summaryMode == SummaryModePlan {
				m.movePlan(-1)
			} else if m.summaryMode == SummaryModeCommits {
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
				m.moveComment(-1)
			}
		case FocusDiff:
			m.diffVP.ScrollUp(1)
		default: // FocusInbox
			if m.selected > 0 {
				m.selected--
				m.resetPRViewState()
				return m, m.selectionCmd()
			}
		}
	case "pan-left":
		if m.focus == FocusDiff && !m.cfg.Wrap.Diff {
			m.diffXOffset = max(0, m.diffXOffset-max(1, m.diffVP.Width()/2))
		} else if m.focus == FocusSummary && m.summaryMode == SummaryModeCommits && !m.cfg.Wrap.Summary {
			m.summaryXOffset = max(0, m.summaryXOffset-max(1, m.summaryVP.Width()/2))
		}
	case "pan-right":
		if m.focus == FocusDiff && !m.cfg.Wrap.Diff {
			m.diffXOffset = min(m.maxDiffXOffset(), m.diffXOffset+max(1, m.diffVP.Width()/2))
		} else if m.focus == FocusSummary && m.summaryMode == SummaryModeCommits && !m.cfg.Wrap.Summary {
			m.summaryXOffset = m.summaryXOffset + max(1, m.summaryVP.Width()/2)
		}
	case "page-down":
		switch m.focus {
		case FocusSummary:
			if m.summaryMode == SummaryModePlan {
				m.pagePlan(1)
			} else {
				m.summaryVP.PageDown()
			}
		case FocusDiff:
			m.diffVP.PageDown()
		case FocusInbox:
			m.syncInboxVP()
			m.inboxManualScroll = true
			m.inboxVP.PageDown()
		}
	case "page-up":
		switch m.focus {
		case FocusSummary:
			if m.summaryMode == SummaryModePlan {
				m.pagePlan(-1)
			} else {
				m.summaryVP.PageUp()
			}
		case FocusDiff:
			m.diffVP.PageUp()
		case FocusInbox:
			m.syncInboxVP()
			m.inboxManualScroll = true
			m.inboxVP.PageUp()
		}
	case "next-item":
		if m.summaryMode == SummaryModePlan {
			m.movePlan(1)
		} else if m.summaryMode == SummaryModeComments {
			m.moveComment(1)
		} else if m.summaryMode == SummaryModeCommits {
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
	case "prev-item":
		if m.summaryMode == SummaryModePlan {
			m.movePlan(-1)
		} else if m.summaryMode == SummaryModeComments {
			m.moveComment(-1)
		} else if m.summaryMode == SummaryModeCommits {
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
	case "inspect-pr":
		if m.summaryMode == SummaryModePlan {
			if node, ok := m.selectedPlanNode(); ok && node.Kind == plan.NodeReport {
				m.adoptPlanReport(node.ReportID)
			} else {
				m.focus = FocusDiff
				m.mode = ModeDetail
				if m.width > 0 && m.width < minMediumWidth {
					m.mode = ModeDiff
				}
			}
			m.resizeViewports()
			return m, nil
		}
		if _, ok := m.current(); ok {
			m.mode = ModeDetail
			m.focus = FocusSummary
			m.resizeViewports()
			return m, m.selectionCmd()
		}
	case "back-to-inbox":
		m.cancelPlanOpen()
		m.mode = ModeInbox
		m.focus = FocusInbox
		m.resizeViewports()
	case "refresh-pr":
		if ok {
			cmd := m.enqueuePRAction(pr, queuedPRAction{})
			return m, cmd
		}
		refresh := m.startInboxRefresh()
		return m, refresh
	case "refresh-inbox":
		refresh := m.startInboxRefresh()
		return m, refresh
	case "toggle-description":
		m.toggleDescription()
		return m, nil
	case "toggle-diff":
		if m.width > 0 && m.width < minMediumWidth {
			if m.diffEnabled && m.mode == ModeDiff {
				// Narrow: close the full-screen diff.
				m.diffEnabled = false
				m.mode = ModeDetail
				m.focus = FocusSummary
				return m, nil
			}
			// Narrow: open the full-screen diff. Diff starts enabled, so
			// "enabled" alone no longer means the full-screen view is open.
			m.diffEnabled = true
			m.syncDiffVP()
			if m.diff != "" || m.summaryMode == SummaryModeComments || m.summaryMode == SummaryModePlan {
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
			owner := m.startStatus("fetching diff…")
			cmd := m.startDiffLoad(pr, owner)
			return m, cmd
		}
		// Wide/medium: toggle diff pane content.
		m.diffEnabled = !m.diffEnabled
		if m.diffEnabled {
			if m.mode == ModeInbox {
				m.mode = ModeDetail
			}
			m.syncDiffVP()
			return m, m.selectionCmd()
		}
		// Turned off: unfocus diff pane if it was focused.
		if m.focus == FocusDiff {
			m.focus = FocusSummary
		}
		return m, nil
	case "open-diff-pager":
		if m.readerContentActive() {
			m.message = "the diff pager is only available for source-code diffs"
			return m, nil
		}
		if pr, ok := m.current(); ok {
			return m, openDiffPagerCmd(pr.Repo, pr.Number)
		}
	case "toggle-side-by-side":
		if m.readerContentActive() {
			return m, nil
		}
		if m.rawDiff != "" && (m.mode == ModeDiff || m.focus == FocusDiff) {
			m.sideBySide = !m.sideBySide
			m.diffXOffset = 0
			m.syncDiffVP()
		}
	case "toggle-file-content":
		if m.readerContentActive() {
			return m, nil
		}
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
		m.fileLoadErr = ""
		owner := m.startStatus("fetching file…")
		m.loadToken++
		m.fileRequest = m.loadToken
		return m, fileContentCmd(m.app.GitHub, pr.Repo, path, pr.HeadRefOID, key, owner, m.fileRequest)
	case "command-palette":
		m.commandInput = ""
		m.commandCursor = 0
		m.prevMode = m.mode
		m.modalTarget = m.intentForPR(pr)
		m.mode = ModeCommand
	}
	return m, nil
}

func (m Model) handleApproveKey(key string) (tea.Model, tea.Cmd) {
	action, ok := m.actions.actionForKey(actionContextApprove, key)
	if !ok {
		return m, nil
	}
	if action.ID == "approve-cancel" {
		m.cancelPRModal()
		m.mode = ModeDetail
		return m, nil
	}
	if action.ID == "approve-confirm" {
		return m.executeAction(m.pendingAction)
	}
	return m, nil
}

func (m *Model) clearLoadedDiffState() {
	m.diffRequest, m.fileRequest, m.commitDiffRequest = 0, 0, 0
	m.diff = ""
	m.rawDiff = ""
	m.diffFiles = nil
	m.renderedFiles = nil
	m.diffFileStarts = nil
	m.diffMode = DiffModeHunks
	m.fileContent = ""
	m.fileContentKey = ""
	m.fileLoadErr, m.fileLoadErrKey = "", ""
	m.fileIndex = 0
	m.commitIndex = 0
	m.commentIndex = 0
	m.diffLoadKey = ""
	m.diffLoadErr = ""
	m.commitDiffKey = ""
	m.commitDiffErr = ""
	m.renderedCommitDiff = ""
	m.rawCommitDiff = ""
	m.commitFileStarts, m.sideFileStarts = nil, nil
	m.sideDiffContent, m.sideDiffKey, m.sideDiffRequested, m.sideDiffFailed = "", "", "", ""
	m.diffXOffset = 0
	m.summaryXOffset = 0
	m.summaryVP.GotoTop()
	m.diffVP.GotoTop()
	m.diffVP.SetContent("")
	m.diffVPLayout = nil
}

func prRefreshKey(pr model.PullRequest) string {
	key := pr.Repo + "#" + strconv.Itoa(pr.Number)
	if pr.HeadRefOID != "" {
		key += "@" + pr.HeadRefOID
	}
	return key
}

// selectionCmd loads whatever the currently selected PR still needs.
func (m *Model) selectionCmd() tea.Cmd {
	m.reconcilePlan()
	return tea.Batch(m.autoDiffCmd(), m.autoDetailsCmd(), m.selectedCommitDiffCmd())
}

func (m *Model) selectedCommitDiffCmd() tea.Cmd {
	if m.summaryMode == SummaryModeCommits {
		return m.autoCommitDiffCmd()
	}
	return nil
}

// autoDetailsCmd fetches commits, comments, and reviewers for the selected PR
// when the inbox list refresh did not include them. It fetches each PR
// version once.
func (m *Model) autoDetailsCmd() tea.Cmd {
	pr, ok := m.current()
	if !ok || (pr.DetailsLoaded && pr.ReviewCommentsLoaded && pr.ReviewersLoaded && pr.ConversationCommentsLoaded && pr.ReviewActivityLoaded) {
		return nil
	}
	if q := m.prActions[keyForPR(pr)]; q != nil && len(q.jobs) > 0 && q.jobs[0].phase != prActionConfirming {
		return nil // the queue already owns a read or write for the selected PR
	}
	key := fmt.Sprintf("%s@%s", prRefreshKey(pr), pr.UpdatedAt.Format(time.RFC3339Nano))
	if m.detailsLoadKey == key {
		return nil
	}
	m.detailsLoadKey = key
	return refreshPRCmd(m.app, pr)
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
	if m.diffLoadKey == prRefreshKey(pr) {
		return nil
	}
	return m.startDiffLoad(pr, 0)
}

func (m *Model) startDiffLoad(pr model.PullRequest, owner int) tea.Cmd {
	m.diffLoadKey, m.diffLoadErr = prRefreshKey(pr), ""
	m.loadToken++
	m.diffRequest = m.loadToken
	diffInnerW := m.paneInnerWidth(m.diffPaneOuterWidth(), FocusDiff)
	return diffCmd(m.app, pr, diffRenderWidth(diffInnerW), owner, m.diffRequest)
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
	sha := newestCommits(pr.Commits)[m.commitIndex].OID
	if m.commitDiffKey == sha {
		return nil
	}
	m.commitDiffKey = sha
	m.commitDiffErr = ""
	m.renderedCommitDiff = ""
	m.rawCommitDiff = ""
	m.commitFileStarts = nil
	m.diffVP.GotoTop()
	m.loadToken++
	m.commitDiffRequest = m.loadToken
	diffInnerW := m.paneInnerWidth(m.diffPaneOuterWidth(), FocusDiff)
	return commitDiffCmd(m.app.GitHub, pr.Repo, sha, diffRenderWidth(diffInnerW), m.commitDiffRequest)
}

func (m *Model) resizeViewports() {
	// Set the stored viewport heights so that vpCanScroll and ScrollDown/ScrollUp
	// work correctly between renders. Only the header occupies a chrome line.
	// Pane border adds 2 lines vertically. These constants mirror the layout
	// functions (wideLayout / mediumLayout) and the pane frame size (2).
	const headerLines = 1
	const footerLines = 0
	bodyH := max(4, m.height-headerLines-footerLines)
	if m.width >= minWideWidth {
		inboxH, summaryH := m.leftColumnHeights(bodyH)
		m.inboxVP.SetHeight(max(1, inboxH-paneFrame))
		m.summaryVP.SetHeight(max(1, summaryH-paneFrame))
		m.diffVP.SetHeight(max(1, bodyH-paneFrame))
	} else if m.width >= minMediumWidth {
		topH := max(8, bodyH/2)
		bottomH := max(6, bodyH-topH)
		vpTopH := max(1, topH-paneFrame)
		vpBotH := max(1, bottomH-paneFrame)
		m.inboxVP.SetHeight(vpTopH)
		m.summaryVP.SetHeight(vpTopH)
		m.diffVP.SetHeight(vpBotH)
	} else {
		vpH := max(1, bodyH-paneFrame)
		m.inboxVP.SetHeight(vpH)
		m.summaryVP.SetHeight(vpH)
		m.diffVP.SetHeight(vpH)
	}
	// Update diff viewport width to the real visible diff content width.
	diffInnerW := m.paneInnerWidth(m.diffPaneOuterWidth(), FocusDiff)
	m.diffVP.SetWidth(m.diffContentWidth(diffInnerW))
	m.diffVP.SetHorizontalStep(max(1, m.diffVP.Width()/2))
	m.syncDiffVP()
	m.syncInboxVP()
	m.syncSummaryVP()
	m.diffVP.SetYOffset(m.diffVP.YOffset())
	m.summaryVP.SetYOffset(m.summaryVP.YOffset())
	m.syncHelpVP()
}

func (m *Model) nextFocus() {
	switch m.focus {
	case FocusInbox:
		m.focus = FocusSummary
		if _, ok := m.current(); ok {
			m.mode = ModeDetail
		}
	case FocusSummary:
		if m.largePaneActive() {
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
		if m.largePaneActive() {
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
		return clipboardMsg{err: cmd.Run()}
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
	// Feedback such as "no supported plan report found" belongs to the PR it
	// described; a different selection must not inherit it.
	m.message = ""
	m.cancelPlanOpen()
	m.clearLoadedDiffState()
	m.reconcilePlan()
	m.inboxManualScroll = false
	m.syncInboxVP()
	m.syncSummaryVP()
	m.syncDiffVP()
}

// syncSummaryVPScroll adjusts the summary viewport Y offset to keep the
// selected item visible with surrounding context in commits or files mode.
func (m *Model) syncSummaryVPScroll() {
	vpH := m.summaryVP.Height()
	if vpH <= 0 {
		return
	}
	pr, ok := m.current()
	if !ok {
		return
	}
	_, spans := m.summaryLayout(pr, max(1, m.summaryVP.Width()))
	index := m.fileIndex
	switch m.summaryMode {
	case SummaryModeCommits:
		index = m.commitIndex
	case SummaryModeComments:
		index = m.commentIndex
	case SummaryModePlan:
		index = m.planNodeIndex()
	}
	if index >= 0 && index < len(spans) {
		m.summaryVP.SetYOffset(keepSelectionVisible(m.summaryVP.YOffset(), vpH, spans, index))
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
	_, summaryInnerW, _ := m.paneInnerWidths()
	layout := m.summaryPane(pr, summaryInnerW)
	m.summaryVP.SetWidth(summaryInnerW)
	if m.summaryVPLayout != layout {
		m.summaryVP.SetContent(layout.content)
		m.summaryVPLayout = layout
	}
}

// syncDiffVP updates the stored diffVP's content so that TotalLineCount is
// accurate for vpCanScroll checks and j/k scrolling works correctly.
// Lines are stored at full width — the viewport's ansi.Cut windows horizontally.
// Width is set to the real diff-pane inner width so scroll % and cut are correct.
func (m *Model) syncDiffVP() {
	if !m.largePaneActive() || (!m.readerContentActive() && m.diff == "") {
		return
	}
	// Use the real visible diff content width so horizontal cut matches render.
	mv := Model{width: m.width, height: m.height, theme: m.theme, focus: m.focus, prs: m.prs, selected: m.selected}
	diffInnerW := mv.paneInnerWidth(mv.diffPaneOuterWidth(), FocusDiff)
	vpW := mv.diffContentWidth(diffInnerW)
	layout := m.diffPane(vpW)
	m.diffVP.SetWidth(vpW)
	m.diffVP.SetHorizontalStep(max(1, vpW/2))
	if m.diffVPLayout != layout {
		m.diffVP.SetContent(layout.content)
		m.diffVPLayout = layout
	}
	m.diffXOffset = min(m.diffXOffset, m.maxDiffXOffset())
}

func (m *Model) currentDiffContent() string {
	if m.descriptionShown() {
		if pr, ok := m.current(); ok {
			return m.descriptionDetail(pr)
		}
		return ""
	}
	if m.summaryMode == SummaryModePlan {
		return m.planDetail()
	}
	if m.summaryMode == SummaryModeComments {
		if entry, ok := m.selectedActivity(); ok {
			return m.activityDetail(entry)
		}
		return "no activity"
	}
	if m.diffMode == DiffModeFile {
		return m.currentFileContent()
	}
	if m.activeSideDiff() {
		return m.sideDiffContent
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
	if file, ok := m.displayedPRDiffFile(); ok {
		return file.Body
	}
	return m.diff
}

func (m *Model) activeSideDiff() bool {
	return m.sideBySide && m.sideDiffKey != "" && m.sideDiffKey == m.sideLayoutKey()
}

func (m Model) displayedPRDiffFile() (diffFile, bool) {
	if len(m.renderedFiles) == 0 {
		return diffFile{}, false
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
	return m.renderedFiles[idx], true
}

func (m Model) selectedFileContentKey() string {
	pr, ok := m.current()
	if !ok || m.fileIndex < 0 || m.fileIndex >= len(pr.Files) {
		return ""
	}
	return fmt.Sprintf("%s#%d:%s@%s", pr.Repo, pr.Number, pr.Files[m.fileIndex].Path, pr.HeadRefOID)
}

func (m *Model) currentFileContent() string {
	if m.fileContentKey != m.selectedFileContentKey() || m.fileContent == "" {
		return ""
	}
	raw, theme := m.selectedRawDiffFile(), m.layoutTheme()
	if cached := m.fileLayoutCache; cached != nil && cached.content == m.fileContent && cached.raw == raw && cached.theme == theme {
		return cached.rendered
	}
	rendered := m.renderFullFileContent(m.fileContent, diffFileMarks(raw))
	m.fileLayoutCache = &fileLayoutCache{m.fileContent, raw, theme, rendered}
	return rendered
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

func (m *Model) maxDiffXOffset() int {
	m.diffPane(max(1, m.diffVP.Width()))
	return max(0, m.diffLayoutCache.maxWidth-m.diffVP.Width())
}

// largePaneActive reports whether the large pane has content to focus. Comments
// are already loaded, so the Comment pane does not depend on the diff toggle.
func (m Model) largePaneActive() bool {
	return m.diffEnabled || m.readerContentActive()
}

// moveComment changes the selected comment and shows it from the top.
func (m *Model) moveComment(delta int) {
	pr, ok := m.current()
	if !ok {
		return
	}
	next := m.commentIndex + delta
	comments := m.activityEntries(pr)
	if next < 0 || next >= len(comments) {
		return
	}
	m.commentIndex = next
	m.commentState.selectedKey = comments[next].Key
	m.diffVP.GotoTop()
	m.diffXOffset = 0
	m.syncDiffVP()
	m.syncSummaryVP()
	m.syncSummaryVPScroll()
}

func (m *Model) setSummaryMode(actionID string) bool {
	if strings.HasPrefix(actionID, "summary-") {
		m.cancelPlanOpen()
	}
	switch actionID {
	case "summary-files":
		m.summaryMode = SummaryModeFiles
	case "summary-commits":
		m.summaryMode = SummaryModeCommits
	case "summary-comments":
		m.summaryMode = SummaryModeComments
	default:
		return false
	}
	return true
}

func diffRenderWidth(visibleWidth int) int {
	return max(visibleWidth*100, 10000)
}

func (m *Model) cycleSortMode() tea.Cmd {
	current := 0
	if pr, ok := m.current(); ok {
		current = pr.Number
	}
	m.sortMode = m.sortMode.Next()
	m.cfg.Sort = m.sortMode.String()
	m.applyPRSettings()
	m.message = "sort: " + m.sortMode.String()
	if current != 0 {
		for i, pr := range m.prs {
			if pr.Number == current {
				m.selected = i
				break
			}
		}
	}
	return m.saveSettings()
}

func (m *Model) applyPRSettings() {
	defer m.reconcilePRModal()
	current := app.PRKey{}
	if pr, ok := m.current(); ok {
		current = keyForPR(pr)
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
	if current.Number != 0 {
		for i, pr := range m.prs {
			if keyForPR(pr) == current {
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
	if pr, ok := m.current(); ok && keyForPR(pr) != current {
		m.resetPRViewState()
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
	action, ok := m.actions.actionForKey(actionContextSettings, key)
	if !ok {
		return m, nil
	}
	switch action.ID {
	case "settings-close":
		m.mode = m.prevMode
		return m, nil
	case "settings-next":
		if m.settingsCursor < 1 {
			m.settingsCursor++
		}
	case "settings-prev":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case "settings-toggle":
		if m.settingsCursor == 0 {
			m.hideDrafts = !m.hideDrafts
			m.cfg.HideDrafts = m.hideDrafts
			m.applyPRSettings()
			m.message = draftFilterMessage(m.hideDrafts)
			save := m.saveSettings()
			if !m.hideDrafts {
				refresh := m.startInboxRefresh()
				return m, tea.Batch(save, refresh)
			}
			return m, save
		} else {
			cmd := m.cycleSortMode()
			return m, cmd
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
	palettes := uitheme.AllPalettes(m.theme.IsDark)
	action, ok := m.actions.actionForKey(actionContextColorscheme, key)
	if !ok {
		return m, nil
	}
	switch action.ID {
	case "colorscheme-cancel":
		// Revert to original theme (re-resolve from saved config).
		m.theme = uitheme.New(uitheme.PaletteByName(m.cfg.Theme, m.theme.IsDark), m.theme.Profile, m.theme.NoColor, m.theme.IsDark)
		m.mode = m.prevMode
		return m, nil
	case "colorscheme-next":
		if m.colorschemeIndex < len(palettes)-1 {
			m.colorschemeIndex++
			m.theme = uitheme.New(palettes[m.colorschemeIndex], m.theme.Profile, m.theme.NoColor, m.theme.IsDark)
		}
	case "colorscheme-prev":
		if m.colorschemeIndex > 0 {
			m.colorschemeIndex--
			m.theme = uitheme.New(palettes[m.colorschemeIndex], m.theme.Profile, m.theme.NoColor, m.theme.IsDark)
		}
	case "colorscheme-confirm":
		// Confirm: save to config.
		chosen := palettes[m.colorschemeIndex]
		m.cfg.Theme = chosen.Name
		m.theme = uitheme.New(chosen, m.theme.Profile, m.theme.NoColor, m.theme.IsDark)
		m.mode = m.prevMode
		cmd := tea.Batch(m.finishStatus("theme: "+chosen.Name, false), m.saveSettings())
		return m, cmd
	}
	return m, nil
}

func (m Model) handleCommentKey(key string) (tea.Model, tea.Cmd) {
	if action, ok := m.actions.actionForKey(actionContextComment, key); ok {
		switch action.ID {
		case "comment-cancel":
			m.cancelPRModal()
			m.mode = ModeDetail
			return m, nil
		case "comment-submit":
			pr, ok := m.actionPR()
			if !ok {
				m.invalidatePRModal()
				return m, m.finishStatus("comment cancelled: original PR is no longer available", true)
			}
			if m.comment == "" {
				return m, nil
			}
			body, closedRevision := m.comment, m.modalTarget.closedRevision
			m.cancelPRModal()
			m.mode = ModeDetail
			if command := classifyAtlantisComment(body); command != atlantisCommandNone {
				cmd := m.enqueueAtlantisComment(pr, command, body, false)
				return m, cmd
			}
			owner := m.startStatus("commenting…")
			return m, commentCmd(m.app, pr, body, owner, closedRevision)
		case "comment-backspace":
			if len(m.comment) > 0 {
				m.comment = m.comment[:len(m.comment)-1]
			}
			return m, nil
		}
	}
	if len(key) == 1 {
		m.comment += key
	}
	return m, nil
}

func (m Model) handleCommandKey(key string) (tea.Model, tea.Cmd) {
	action, control := m.actions.actionForKey(actionContextCommand, key)
	if control && action.ID == "command-cancel" {
		m.cancelPRModal()
		m.commandCursor = 0
		return m, nil
	}
	if control && action.ID == "command-submit" {
		command, ok := m.actions.exactCommand(m.commandInput)
		if !ok {
			// No exact match; try cursor-selected command from matching list
			matches := m.actions.matchingCommands(m.commandInput)
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
			command = matches[idx].action
			// Normalize the input to the alias that actually matched so the
			// confirmation prompt names the command the user selected.
			m.commandInput = matches[idx].alias
			ok = true
		}
		if !ok {
			return m, nil
		}
		return m.triggerAction(command)
	}
	if control && action.ID == "command-next" {
		matches := m.actions.matchingCommands(m.commandInput)
		if len(matches) > 0 {
			m.commandCursor++
			if m.commandCursor >= len(matches) {
				m.commandCursor = len(matches) - 1
			}
		}
		return m, nil
	}
	if control && action.ID == "command-prev" {
		m.commandCursor--
		if m.commandCursor < 0 {
			m.commandCursor = 0
		}
		return m, nil
	}
	if control && action.ID == "command-backspace" {
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
	action, ok := m.actions.actionForKey(actionContextCommandConfirmation, key)
	if !ok {
		return m, nil
	}
	if action.ID == "command-cancel-confirmation" {
		m.cancelPRModal()
		return m, nil
	}
	if action.ID == "command-confirm" {
		return m.executeAction(m.pendingAction)
	}
	return m, nil
}

func (m Model) handleHelpKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		m.mode = m.prevMode
		return m, nil
	}
	switch key {
	case "up":
		m.helpVP.ScrollUp(1)
	case "down":
		m.helpVP.ScrollDown(1)
	case "pgup":
		m.helpVP.PageUp()
	case "pgdown", " ":
		m.helpVP.PageDown()
	case "g":
		m.helpVP.GotoTop()
	case "G":
		m.helpVP.GotoBottom()
	case "backspace", "ctrl+h":
		if len(m.helpInput) > 0 {
			m.helpInput = m.helpInput[:len(m.helpInput)-1]
			m.syncHelpVP()
		}
	default:
		if len(key) == 1 {
			m.helpInput += key
			m.syncHelpVP()
		}
	}
	return m, nil
}

func (m *Model) syncHelpVP() {
	width := min(54, max(1, m.width-12))
	if m.width <= 0 {
		width = 54
	}
	m.helpVP.SetWidth(max(1, width-1))
	m.helpVP.SetHeight(max(1, m.helpModalHeight()-6))
	m.helpVP.SetContent(m.helpPanel())
}
