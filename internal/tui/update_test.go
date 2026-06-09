package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func modelWithPRs(prs []model.PullRequest, selected int) Model {
	m := minimalModel()
	m.prs = prs
	m.selected = selected
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

func TestInboxTitleDoesNotChangePaneGeometryWithSelectedContent(t *testing.T) {
	short := model.PullRequest{Number: 1, Repo: "org/repo", Title: "short"}
	long := model.PullRequest{Number: 2, Repo: "org/repo", Title: strings.Repeat("long-title-", 12)}

	base := modelWithPRs([]model.PullRequest{short, long}, 0)
	base.width = 90
	base.height = 24
	base.mode = ModeInbox
	shortView := base.View()

	base.selected = 1
	longView := base.View()

	if got, want := lipWidthOfFirstLine(shortView), lipWidthOfFirstLine(longView); got != want {
		t.Fatalf("pane width changed with selected PR content: %d vs %d", got, want)
	}
	if got, want := len(strings.Split(shortView, "\n")), len(strings.Split(longView, "\n")); got != want {
		t.Fatalf("pane height changed with selected PR content: %d vs %d", got, want)
	}
}

func TestDiffHorizontalScrollMovesXOffset(t *testing.T) {
	m := minimalModel()
	m.width = 120
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("abcdefghijklmnopqrstuvwxyz", 8)
	m.syncDiffVP()

	initial := m.diffPanel(m.width, m.height)
	if !strings.Contains(initial, "abcd") {
		t.Fatalf("initial diff view missing expected prefix: %q", initial)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	afterRight := m.diffPanel(m.width, m.height)
	if afterRight == initial {
		t.Fatal("diff view did not change after right key")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(Model)
	afterLeft := m.diffPanel(m.width, m.height)
	if afterLeft != initial {
		t.Fatalf("diff view not restored after left key")
	}
}

func TestInboxSelectedPRUsesFullBlockHighlight(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number:   1,
		Repo:     "org/repo",
		Title:    "very long title with wide chars 測試 測試 測試",
		Author:   "author",
		CIStatus: "passing",
		Skipped:  true,
	}}, 0)
	m.width = 28
	m.height = 12
	view := m.inboxPanel(m.width, m.height)
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(stripANSI(line)) > m.width-4 {
			t.Fatalf("line over width: %q", line)
		}
	}
	if strings.Contains(view, ">") {
		t.Fatalf("selected inbox row still has cursor: %q", view)
	}
}

func TestInboxScrollKeepsSelectedPRVisible(t *testing.T) {
	prs := make([]model.PullRequest, 8)
	for i := range prs {
		prs[i] = model.PullRequest{Number: i + 1, Repo: "org/repo", Title: strings.Repeat("item-", i+1), Author: "author", CIStatus: "passing"}
	}
	m := modelWithPRs(prs, len(prs)-1)
	m.width = 48
	m.height = 12
	view := m.inboxPanel(m.width, m.height)
	if !strings.Contains(view, prs[len(prs)-1].Title) {
		t.Fatalf("inbox view missing selected PR title %q: %q", prs[len(prs)-1].Title, view)
	}
	if !strings.Contains(view, "CI pass") || !strings.Contains(view, "#8 · author") {
		t.Fatalf("inbox view missing full selected PR details: %q", view)
	}
}

func TestLayoutUsesFullHeight(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author", CIStatus: "passing"}}, 0)
	m.width = 120
	m.height = 20
	view := m.View()
	got := len(strings.Split(strings.TrimRight(view, "\n"), "\n"))
	if got != m.height {
		t.Fatalf("layout height = %d, want %d", got, m.height)
	}
}

func TestDiffVerticalScrollMovesYOffset(t *testing.T) {
	m := minimalModel()
	m.width = 120
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("line\n", 80)
	m.resizeViewports()

	initial := m.diffVP.YOffset
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.diffVP.YOffset <= initial {
		t.Fatalf("diff YOffset did not move down after down key: %d -> %d", initial, m.diffVP.YOffset)
	}

	afterDown := m.diffVP.YOffset
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = next.(Model)
	if m.diffVP.YOffset <= afterDown {
		t.Fatalf("diff YOffset did not move down after pgdown: %d -> %d", afterDown, m.diffVP.YOffset)
	}
}

func TestSummarySelectedFileFlushLeftAndHighlighted(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		Title:  "title",
		Author: "author",
		Files:  []model.ChangedFile{{Path: "a/very/long/path/name.go", Status: "M"}, {Path: "other.go", Status: "A"}},
	}}, 0)
	m.width = 40
	m.height = 14
	m.focus = FocusSummary
	m.fileIndex = 0
	view := m.summaryPanel(m.width, m.height)
	if strings.Contains(view, ">") {
		t.Fatalf("selected summary row still has cursor: %q", view)
	}
	if !strings.Contains(view, "name.go") {
		t.Fatalf("selected file path missing: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(stripANSI(line)) > m.width-4 {
			t.Fatalf("summary line over width: %q", line)
		}
	}
}

func TestTruncateANSIWidthLeavesTwoColumnMargin(t *testing.T) {
	got := lipgloss.Width(stripANSI(truncateANSIWidth("abcdef", 6)))
	if got != 4 {
		t.Fatalf("truncate width = %d, want 4", got)
	}
}

func lipWidthOfFirstLine(s string) int {
	line := strings.SplitN(s, "\n", 2)[0]
	return lipgloss.Width(stripANSI(line))
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
