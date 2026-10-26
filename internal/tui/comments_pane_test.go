package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/model"
)

func commentPaneModel(t *testing.T) Model {
	t.Helper()
	m := scrollModel(t, 1)
	now := time.Now()
	m.prs[0].Comments = []model.PRComment{
		{Author: "older-author", Body: "older body", CreatedAt: now.Add(-2 * time.Hour)},
		{Author: "newest-author", Body: "newest body", Kind: "inline", Path: "main.go", Line: 7, CreatedAt: now.Add(-time.Minute)},
	}
	m.allPRs = m.prs
	m.diffEnabled = false
	m.mode, m.focus = ModeDetail, FocusSummary
	m = pressKey(m, "3")
	return m
}

func TestNewModelEnablesDiffByDefault(t *testing.T) {
	if m := scrollModel(t, 1); !m.diffEnabled {
		t.Fatal("diff should be enabled by default")
	}
}

func TestCommentsListShowsAuthorAndTimestampOnly(t *testing.T) {
	m := commentPaneModel(t)
	view := stripANSI(m.summaryPanel(60, 12))
	if !strings.Contains(view, "newest-author") || !strings.Contains(view, "Today") {
		t.Fatalf("comment list missing author or timestamp: %q", view)
	}
	if strings.Index(view, "newest-author") > strings.Index(view, "older-author") {
		t.Fatalf("comment list not newest first: %q", view)
	}
	if lines := strings.Split(view, "\n"); !strings.HasPrefix(lines[0], "Today ") || !strings.Contains(lines[0], " · newest-author") || !strings.Contains(lines[1], " · older-author") {
		t.Fatalf("comment rows not formatted as timestamp · author: %q", lines[:2])
	}
	if strings.Contains(view, "body") || strings.Contains(view, "main.go") {
		t.Fatalf("comment list shows comment content: %q", view)
	}
}

func TestCommentPaneShowsSelectedComment(t *testing.T) {
	m := commentPaneModel(t)
	if got := m.diffTitle(); got != "Comment" {
		t.Fatalf("large pane title = %q, want Comment", got)
	}
	view := stripANSI(m.diffPanel(80, 20))
	if !strings.Contains(view, "newest body") || !strings.Contains(view, "main.go:7") || strings.Contains(view, "older body") {
		t.Fatalf("Comment pane should show newest comment while diff is off: %q", view)
	}
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeyDown, Mod: tea.ModShift}} {
		m.commentIndex = 0
		next, _ := m.Update(key)
		m = next.(Model)
		view = stripANSI(m.diffPanel(80, 20))
		if m.commentIndex != 1 || !strings.Contains(view, "older body") {
			t.Fatalf("%v did not select the next comment: index=%d view=%q", key, m.commentIndex, view)
		}
	}
	m = pressSpecial(m, tea.KeyUp)
	if m.commentIndex != 0 {
		t.Fatalf("up did not select the previous comment: %d", m.commentIndex)
	}
}

func TestCommentPaneFocusableWithoutDiff(t *testing.T) {
	m := commentPaneModel(t)
	m.nextFocus()
	if m.focus != FocusDiff {
		t.Fatalf("focus after Summary = %v, want Comment pane", m.focus)
	}
}

func TestCommentSelectionResetsOnModeSwitch(t *testing.T) {
	m := commentPaneModel(t)
	m = pressSpecial(m, tea.KeyDown)
	m = pressKey(m, "1")
	m = pressKey(m, "3")
	if m.commentIndex != 0 {
		t.Fatalf("comment selection = %d after switching back, want 0", m.commentIndex)
	}
}
