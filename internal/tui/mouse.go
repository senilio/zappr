package tui

import tea "charm.land/bubbletea/v2"

// FilterMouseWheel drops ineffective pane scrolling before Bubble Tea invokes
// Update and View. Non-wheel input is always preserved.
func FilterMouseWheel(current tea.Model, msg tea.Msg) tea.Msg {
	// Use concrete v2 events to keep the hot path allocation-free. Calling
	// Mouse through its interface makes the input message escape to the heap.
	var mouse tea.Mouse
	switch event := msg.(type) {
	case tea.MouseWheelMsg:
		mouse = tea.Mouse(event)
	case tea.MouseReleaseMsg:
		mouse = tea.Mouse(event)
	default:
		return msg
	}
	switch mouse.Button {
	case tea.MouseWheelUp, tea.MouseWheelDown, tea.MouseWheelLeft, tea.MouseWheelRight:
	default:
		return msg
	}
	m, ok := current.(Model)
	if !ok {
		return msg
	}
	switch m.mode {
	case ModeInbox, ModeDetail, ModeDiff:
	default:
		// Overlays retain their existing input behavior.
		return msg
	}
	vp := m.inboxVP
	switch m.focus {
	case FocusInbox:
		_, wheel := msg.(tea.MouseWheelMsg)
		if wheel && !m.inboxManualScroll && (mouse.Button == tea.MouseWheelUp || mouse.Button == tea.MouseWheelDown) {
			// The first event also changes selection-following into manual
			// scrolling, even if the visible position is already at a boundary.
			return msg
		}
	case FocusSummary:
		vp = m.summaryVP
	case FocusDiff:
		vp = m.diffVP
		if m.cfg.Wrap.Diff && (mouse.Mod.Contains(tea.ModShift) || mouse.Button == tea.MouseWheelLeft || mouse.Button == tea.MouseWheelRight) {
			return nil
		}
	default:
		return msg
	}
	// Probe a value copy using Bubbles' own wheel semantics rather than
	// duplicating its Shift, horizontal-step, and disabled-wheel behavior.
	next, cmd := vp.Update(msg)
	if cmd == nil && next.YOffset() == vp.YOffset() && next.HorizontalScrollPercent() == vp.HorizontalScrollPercent() {
		return nil
	}
	return msg
}
