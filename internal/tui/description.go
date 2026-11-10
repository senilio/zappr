package tui

import tea "charm.land/bubbletea/v2"

type paneOffset struct {
	key  string
	x, y int
}

// toggleDescription switches the large pane between its normal content and
// the selected PR's description. The diff scroll position is restored when
// the description is closed with the same toggle.
func (m *Model) toggleDescription() {
	pr, ok := m.current()
	if !ok || m.width < minMediumWidth {
		return
	}
	key := readerPRKey(pr)
	parked := m.parkedDiffOffset
	m.parkedDiffOffset = paneOffset{key, m.diffXOffset, m.diffVP.YOffset()}
	m.descriptionActive = !m.descriptionActive
	m.diffVP.GotoTop()
	m.diffXOffset = 0
	m.syncDiffVP()
	if m.descriptionActive {
		return
	}
	if parked.key == key {
		m.diffVP.SetYOffset(parked.y)
		m.diffXOffset = min(parked.x, m.maxDiffXOffset())
	}
	if m.focus == FocusDiff && !m.largePaneActive() {
		m.focus = FocusSummary
	}
}

// leaveDescriptionOnNavigation exits description mode when user input selects
// another PR or another Summary item. Asynchronous updates never exit it.
func (m *Model) leaveDescriptionOnNavigation(prev Model, msg tea.Msg) {
	if !prev.descriptionActive || !m.descriptionActive {
		return
	}
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseClickMsg:
	default:
		return
	}
	prevPR, _ := prev.current()
	pr, _ := m.current()
	navigated := prevPR.Repo != pr.Repo || prevPR.Number != pr.Number ||
		prev.summaryMode != m.summaryMode || prev.fileIndex != m.fileIndex ||
		prev.commitIndex != m.commitIndex || prev.commentIndex != m.commentIndex ||
		prev.planState.nodeID != m.planState.nodeID
	if !navigated {
		return
	}
	m.descriptionActive = false
	m.parkedDiffOffset = paneOffset{}
	m.diffVP.GotoTop()
	m.diffXOffset = 0
}
