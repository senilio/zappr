package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/model"
)

func descriptionModel(t *testing.T) Model {
	t.Helper()
	m := scrollModel(t, 2)
	m.prs[0].Body = "first body"
	m.prs[1].Body = "second body"
	m.prs[0].Files = []model.ChangedFile{{Path: "a.go"}, {Path: "b.go"}}
	m.allPRs = m.prs
	m.diff = "+diff line"
	m.resizeViewports()
	return m
}

func pressShift(m Model, key rune) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: key, Mod: tea.ModShift})
	return next.(Model)
}

func TestDescriptionShownOnStartupWithInboxFocus(t *testing.T) {
	m := descriptionModel(t)
	if got := m.diffTitle(); got != "Description" {
		t.Fatalf("title = %q, want Description", got)
	}
	if view := stripANSI(m.diffPanel(100, 20)); !strings.Contains(view, "first body") || strings.Contains(view, "diff line") {
		t.Fatalf("description pane = %q", view)
	}
}

func TestDescriptionFollowsInboxSelection(t *testing.T) {
	m := pressSpecial(descriptionModel(t), tea.KeyDown)
	view := stripANSI(m.diffPanel(100, 20))
	if !strings.Contains(view, "second body") || strings.Contains(view, "first body") || m.diffVP.YOffset() != 0 {
		t.Fatalf("description did not follow selection: %q offset %d", view, m.diffVP.YOffset())
	}
}

func TestSummaryFocusShowsDiff(t *testing.T) {
	m := pressSpecial(descriptionModel(t), tea.KeyTab)
	if m.focus != FocusSummary || m.diffTitle() == "Description" {
		t.Fatalf("focus %v title %q, want Summary focus with diff", m.focus, m.diffTitle())
	}
	if view := stripANSI(m.diffPanel(100, 20)); strings.Contains(view, "first body") {
		t.Fatalf("Summary focus still shows description: %q", view)
	}
	m = pressShift(m, tea.KeyTab)
	if m.focus != FocusInbox || m.diffTitle() != "Description" {
		t.Fatalf("focus %v title %q, want Inbox with Description", m.focus, m.diffTitle())
	}
}

func TestShiftArrowInInboxActivatesDiff(t *testing.T) {
	m := pressShift(descriptionModel(t), tea.KeyDown)
	if m.descriptionActive || m.fileIndex != 1 || m.focus != FocusInbox {
		t.Fatalf("active %v fileIndex %d focus %v", m.descriptionActive, m.fileIndex, m.focus)
	}
	m = pressSpecial(m, tea.KeyDown)
	if m.descriptionActive || m.diffTitle() == "Description" {
		t.Fatal("moving in Inbox re-activated description")
	}
}

func TestDescriptionKeptWhenFocusingRightPane(t *testing.T) {
	m := pressShift(descriptionModel(t), tea.KeyTab)
	if m.focus != FocusDiff || m.diffTitle() != "Description" {
		t.Fatalf("focus %v title %q, want Diff focus with Description", m.focus, m.diffTitle())
	}
}

func TestDescriptionShownWithDiffDisabled(t *testing.T) {
	m := descriptionModel(t)
	m.diffEnabled = false
	if view := stripANSI(m.diffPanel(100, 20)); !strings.Contains(view, "first body") {
		t.Fatalf("description hidden with diff disabled: %q", view)
	}
}

func TestEmptyDescription(t *testing.T) {
	m := descriptionModel(t)
	m.prs[0].Body = "  \n"
	m.allPRs = m.prs
	if view := stripANSI(m.diffPanel(100, 20)); !strings.Contains(view, "no description") {
		t.Fatalf("empty description pane = %q", view)
	}
}

func TestDescriptionDisablesSideBySideLayout(t *testing.T) {
	m := descriptionModel(t)
	m.sideBySide, m.rawDiff = true, "x"
	if raw := m.sideLayoutRaw(); raw != "" {
		t.Fatalf("sideLayoutRaw = %q, want empty while description shown", raw)
	}
}

func TestDiffScrollRestoredAfterInboxRoundTrip(t *testing.T) {
	m := descriptionModel(t)
	m.diff = strings.Repeat("+diff line\n", 200)
	m = pressSpecial(m, tea.KeyTab)
	m.diffVP.SetYOffset(40)
	m = pressShift(m, tea.KeyTab)
	if m.diffVP.YOffset() != 0 {
		t.Fatalf("description offset = %d, want 0", m.diffVP.YOffset())
	}
	m = pressSpecial(m, tea.KeyTab)
	if m.diffVP.YOffset() != 40 {
		t.Fatalf("diff offset after round trip = %d, want 40", m.diffVP.YOffset())
	}
	m = pressShift(m, tea.KeyTab)
	m = pressShift(m, tea.KeyDown)
	if m.diffVP.YOffset() != 0 {
		t.Fatalf("shift+down kept stale diff offset %d", m.diffVP.YOffset())
	}
}

func TestNarrowLayoutIgnoresDescription(t *testing.T) {
	m := descriptionModel(t)
	m.width = 60
	if m.descriptionShown() {
		t.Fatal("description shown in narrow layout")
	}
}
