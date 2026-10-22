package tui

import tea "github.com/charmbracelet/bubbletea"

// FilterMouseWheel drops ineffective pane scrolling before Bubble Tea invokes
// Update and View. Non-wheel input is always preserved.
func FilterMouseWheel(current tea.Model, msg tea.Msg) tea.Msg {
	mouse, ok := msg.(tea.MouseMsg)
	if !ok || !tea.MouseEvent(mouse).IsWheel() {
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
		if !m.inboxManualScroll && (mouse.Button == tea.MouseButtonWheelUp || mouse.Button == tea.MouseButtonWheelDown) {
			// The first event also changes selection-following into manual
			// scrolling, even if the visible position is already at a boundary.
			return msg
		}
	case FocusSummary:
		vp = m.summaryVP
	case FocusDiff:
		vp = m.diffVP
		if m.cfg.Wrap.Diff && (mouse.Shift || mouse.Button == tea.MouseButtonWheelLeft || mouse.Button == tea.MouseButtonWheelRight) {
			return nil
		}
	default:
		return msg
	}
	// Probe a value copy using Bubbles' own wheel semantics rather than
	// duplicating its Shift, horizontal-step, and disabled-wheel behavior.
	next, cmd := vp.Update(msg)
	if cmd == nil && next.YOffset == vp.YOffset && next.HorizontalScrollPercent() == vp.HorizontalScrollPercent() {
		return nil
	}
	return msg
}
