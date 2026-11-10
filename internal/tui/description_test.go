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
	m.diff = strings.Repeat("+diff line\n", 200)
	m.resizeViewports()
	return m
}

func pressShift(m Model, key rune) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: key, Mod: tea.ModShift})
	return next.(Model)
}

func descriptionVisible(m Model) bool {
	return m.diffTitle() == "Description" && strings.Contains(stripANSI(m.diffPanel(100, 20)), "body")
}

func TestDescriptionOffByDefault(t *testing.T) {
	m := descriptionModel(t)
	if descriptionVisible(m) {
		t.Fatal("description shown by default")
	}
	if m = pressSpecial(m, tea.KeyTab); descriptionVisible(m) {
		t.Fatal("focus change enabled description")
	}
}

func TestDescriptionToggleKey(t *testing.T) {
	m := pressKey(descriptionModel(t), "d")
	if !descriptionVisible(m) || !strings.Contains(stripANSI(m.diffPanel(100, 20)), "first body") {
		t.Fatalf("d did not show description: title %q", m.diffTitle())
	}
	if view := stripANSI(m.diffPanel(100, 20)); strings.Contains(view, "diff line") {
		t.Fatalf("description pane still shows diff: %q", view)
	}
	if m = pressKey(m, "d"); descriptionVisible(m) {
		t.Fatal("second d did not return to diff")
	}
}

func TestDescriptionToggleWorksFromEveryPane(t *testing.T) {
	for _, focus := range []Focus{FocusInbox, FocusSummary, FocusDiff} {
		m := descriptionModel(t)
		m.focus, m.mode = focus, ModeDetail
		if m = pressKey(m, "d"); !descriptionVisible(m) {
			t.Fatalf("d ignored with focus %v", focus)
		}
	}
}

func TestDescriptionFocusChangesKeepMode(t *testing.T) {
	m := pressKey(descriptionModel(t), "d")
	for _, step := range []func(Model) Model{
		func(m Model) Model { return pressSpecial(m, tea.KeyTab) },
		func(m Model) Model { return pressSpecial(m, tea.KeyTab) },
		func(m Model) Model { return pressShift(m, tea.KeyTab) },
	} {
		if m = step(m); !descriptionVisible(m) {
			t.Fatalf("focus change to %v left description mode", m.focus)
		}
	}
}

func TestDescriptionResetsOnPRChange(t *testing.T) {
	m := pressSpecial(pressKey(descriptionModel(t), "d"), tea.KeyDown)
	if m.selected != 1 || m.descriptionActive || descriptionVisible(m) {
		t.Fatalf("selected %d active %v after PR change", m.selected, m.descriptionActive)
	}
}

func TestDescriptionLeftOnNavigation(t *testing.T) {
	steps := map[string]func(Model) Model{
		"shift+down": func(m Model) Model { return pressShift(m, tea.KeyDown) },
		"shift+up":   func(m Model) Model { m.fileIndex = 1; return pressShift(m, tea.KeyUp) },
		"summary 2":  func(m Model) Model { return pressKey(m, "2") },
		"summary 3":  func(m Model) Model { return pressKey(m, "3") },
		"summary 1":  func(m Model) Model { m.summaryMode = SummaryModeComments; return pressKey(m, "1") },
		"summary down": func(m Model) Model {
			m.focus, m.mode = FocusSummary, ModeDetail
			return pressSpecial(m, tea.KeyDown)
		},
	}
	for name, step := range steps {
		t.Run(name, func(t *testing.T) {
			m := pressKey(descriptionModel(t), "d")
			m.diffVP.SetYOffset(3)
			if m = step(m); m.descriptionActive || m.diffTitle() == "Description" {
				t.Fatalf("%s kept description mode", name)
			}
		})
	}
}

func TestDescriptionToggleRestoresDiffScroll(t *testing.T) {
	m := descriptionModel(t)
	m.focus, m.mode = FocusDiff, ModeDetail
	m.diffVP.SetYOffset(40)
	if m = pressKey(m, "d"); m.diffVP.YOffset() != 0 {
		t.Fatalf("description opened at offset %d", m.diffVP.YOffset())
	}
	if m = pressKey(m, "d"); m.diffVP.YOffset() != 40 {
		t.Fatalf("diff offset after toggle = %d, want 40", m.diffVP.YOffset())
	}
}

func TestDescriptionShownWithDiffDisabled(t *testing.T) {
	m := descriptionModel(t)
	m.diffEnabled = false
	if m = pressKey(m, "d"); !strings.Contains(stripANSI(m.diffPanel(100, 20)), "first body") {
		t.Fatal("description hidden with diff disabled")
	}
}

func TestEmptyDescription(t *testing.T) {
	m := descriptionModel(t)
	m.prs[0].Body = "  \n"
	m.allPRs = m.prs
	if m = pressKey(m, "d"); !strings.Contains(stripANSI(m.diffPanel(100, 20)), "no description") {
		t.Fatal("empty description not reported")
	}
}

func TestNarrowLayoutIgnoresDescriptionToggle(t *testing.T) {
	m := descriptionModel(t)
	m.width = 60
	if m = pressKey(m, "d"); m.descriptionActive {
		t.Fatal("d enabled description in narrow layout")
	}
}

func TestDescriptionDisablesSideBySideLayout(t *testing.T) {
	m := pressKey(descriptionModel(t), "d")
	m.sideBySide, m.rawDiff = true, "x"
	if raw := m.sideLayoutRaw(); raw != "" {
		t.Fatalf("sideLayoutRaw = %q, want empty while description shown", raw)
	}
}
