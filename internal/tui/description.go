package tui

type paneOffset struct {
	key  string
	x, y int
}

// applyDescriptionFocus updates descriptionActive after a focus change from
// prev. When the large pane switches content, it parks the outgoing scroll
// position and returns the incoming one if it belongs to the same PR.
func (m *Model) applyDescriptionFocus(prev Model) *paneOffset {
	// next-item/prev-item clear the flag and select new content themselves.
	selected := prev.descriptionActive && !m.descriptionActive
	switch {
	case m.focus == FocusInbox && prev.focus != FocusInbox:
		m.descriptionActive = true
	case m.focus == FocusSummary:
		m.descriptionActive = false
	}
	if m.descriptionShown() == prev.descriptionShown() {
		return nil
	}
	key := ""
	if pr, ok := m.current(); ok {
		key = prRefreshKey(pr)
	}
	incoming := m.parkedDiffOffset
	m.parkedDiffOffset = paneOffset{key, m.diffXOffset, m.diffVP.YOffset()}
	m.diffVP.GotoTop()
	m.diffXOffset = 0
	if !selected && key != "" && incoming.key == key {
		return &incoming
	}
	return nil
}
