package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/senilio/zappr/internal/model"
)

func TestVisiblePRsExcludesDrafts(t *testing.T) {
	prs := []model.PullRequest{
		{Number: 1, Draft: true},
		{Number: 2},
		{Number: 3, Draft: true},
	}

	got := visiblePRs(prs)

	if len(got) != 1 || got[0].Number != 2 {
		t.Fatalf("visiblePRs = %#v, want only PR #2", got)
	}
}

// minimalModel returns a Model with enough state for key-handling tests,
// without requiring a real *app.App.
func minimalModel() Model {
	m := Model{}
	m.prs = []model.PullRequest{
		{Number: 1, Repo: "org/repo", Title: "first"},
	}
	m.selected = 0
	m.mode = ModeInbox
	m.focus = FocusInbox
	return m
}

func pressKey(m Model, key string) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return next.(Model)
}

func pressSpecial(m Model, key tea.KeyType) Model {
	next, _ := m.Update(tea.KeyMsg{Type: key})
	return next.(Model)
}

// TestDiffDisabledByDefault verifies diff is off and autoDiffCmd returns nil.
func TestDiffDisabledByDefault(t *testing.T) {
	m := minimalModel()
	if m.diffEnabled {
		t.Fatal("diffEnabled should be false by default")
	}
	if cmd := m.autoDiffCmd(); cmd != nil {
		t.Fatal("autoDiffCmd should return nil when diffEnabled=false")
	}
}

// TestVKeyTogglesDiffOn verifies pressing v enables diff and stays in Summary focus.
func TestVKeyTogglesDiffOn(t *testing.T) {
	m := minimalModel()
	m.width = 200 // wide, so no narrow-path branch
	m = pressKey(m, "v")
	if !m.diffEnabled {
		t.Fatal("diffEnabled should be true after pressing v")
	}
	if m.focus == FocusDiff {
		t.Fatalf("focus should not be FocusDiff after v (diff scrolls from Summary), got %v", m.focus)
	}
}

// TestVKeyTogglesDiffOff verifies a second v press disables diff.
func TestVKeyTogglesDiffOff(t *testing.T) {
	m := minimalModel()
	m.width = 200
	m = pressKey(m, "v") // on
	m = pressKey(m, "v") // off
	if m.diffEnabled {
		t.Fatal("diffEnabled should be false after pressing v twice")
	}
	if m.focus == FocusDiff {
		t.Fatal("focus should not be FocusDiff after diff is turned off")
	}
}

// TestAutoDiffCmdRespectsEnabled verifies autoDiffCmd returns a command when enabled.
func TestAutoDiffCmdRespectsEnabled(t *testing.T) {
	m := minimalModel()
	m.diffEnabled = true
	if cmd := m.autoDiffCmd(); cmd == nil {
		t.Fatal("autoDiffCmd should return a command when diffEnabled=true and diff not yet loaded")
	}
}

// TestAutoDiffCmdDeduplicates verifies autoDiffCmd returns nil when diff already loaded for same PR.
func TestAutoDiffCmdDeduplicates(t *testing.T) {
	m := minimalModel()
	m.diffEnabled = true
	m.diffLoadKey = "org/repo#1"
	if cmd := m.autoDiffCmd(); cmd != nil {
		t.Fatal("autoDiffCmd should return nil when diffLoadKey matches current PR")
	}
}

// TestSplitDiffFilesRaw verifies that splitDiffFiles correctly splits a raw
// unified diff (with "diff --git" markers) into per-file chunks.
func TestSplitDiffFilesRaw(t *testing.T) {
	raw := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1 +1 @@
-old
+new
diff --git a/bar.go b/bar.go
--- a/bar.go
+++ b/bar.go
@@ -1 +1 @@
-x
+y
`
	files := splitDiffFiles(raw)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Path != "foo.go" {
		t.Errorf("files[0].Path = %q, want %q", files[0].Path, "foo.go")
	}
	if files[1].Path != "bar.go" {
		t.Errorf("files[1].Path = %q, want %q", files[1].Path, "bar.go")
	}
	// Each body must not contain the other file's content.
	if contains(files[0].Body, "bar.go") {
		t.Error("files[0].Body contains bar.go content")
	}
	if contains(files[1].Body, "foo.go") {
		t.Error("files[1].Body contains foo.go content")
	}
}

// TestSplitDiffFilesRenderedDeltaOutput verifies that splitting a
// delta-rendered diff (which rewrites "diff --git" header lines) returns only
// a single chunk — demonstrating WHY we split on the raw diff instead.
// renderDiffFiles must be used for correct per-file rendering.
func TestSplitDiffFilesRenderedDeltaOutput(t *testing.T) {
	// Simulate what delta does: it replaces the "diff --git" line with styled
	// output that no longer starts with "diff --git ".
	deltaRendered := `━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
foo.go
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 1  │-old
 1  │+new
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
bar.go
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 1  │-x
 1  │+y
`
	files := splitDiffFiles(deltaRendered)
	// Delta output has no "diff --git" lines, so everything is one chunk (or zero).
	if len(files) > 1 {
		t.Fatalf("expected 0 or 1 chunks from delta output (no git markers), got %d", len(files))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
