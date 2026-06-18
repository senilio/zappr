package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/model"
	"github.com/senilio/zappr/internal/tui/theme"
)

func TestFilterPRsShowsDraftsByDefault(t *testing.T) {
	prs := []model.PullRequest{
		{Number: 1, Draft: true},
		{Number: 2},
		{Number: 3, Draft: true},
	}

	got := filterPRs(prs, false)

	if len(got) != 3 {
		t.Fatalf("filterPRs = %#v, want all PRs", got)
	}
}

func TestFilterPRsCanHideDrafts(t *testing.T) {
	prs := []model.PullRequest{
		{Number: 1, Draft: true},
		{Number: 2},
		{Number: 3, Draft: true},
	}

	got := filterPRs(prs, true)

	if len(got) != 1 || got[0].Number != 2 {
		t.Fatalf("filterPRs = %#v, want only PR #2", got)
	}
}

func TestSettingsToggleCanHideDrafts(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{
		{Number: 1, Draft: true},
		{Number: 2},
	}, 0)
	m.mode = ModeSettings
	m.prevMode = ModeInbox

	m = pressSpecial(m, tea.KeySpace)

	if !m.hideDrafts {
		t.Fatal("hideDrafts should be enabled")
	}
	if len(m.prs) != 1 || m.prs[0].Number != 2 {
		t.Fatalf("prs = %#v, want only non-draft PR", m.prs)
	}
}

func TestSettingsCanCycleSortMode(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 2}, {Number: 1}}, 0)
	m.mode = ModeSettings
	m.settingsCursor = 1

	m = pressSpecial(m, tea.KeySpace)

	if m.sortMode != app.SortNumber {
		t.Fatalf("sortMode = %s, want number", m.sortMode)
	}
	if m.prs[0].Number != 2 {
		t.Fatalf("prs = %#v, want sorted by number descending", m.prs)
	}
}

func TestOKeyOpensPRURLInsteadOfSorting(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", URL: "https://github.com/org/repo/pull/1"}}, 0)
	m.sortMode = app.SortUpdated

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = next.(Model)

	if cmd == nil {
		t.Fatal("o key should return open-browser command")
	}
	if m.sortMode != app.SortUpdated {
		t.Fatalf("sortMode = %s, want unchanged", m.sortMode)
	}
}

func TestPRURLFallsBackToGitHubURL(t *testing.T) {
	got := prURL(model.PullRequest{Repo: "org/repo", Number: 7})
	if got != "https://github.com/org/repo/pull/7" {
		t.Fatalf("prURL = %q", got)
	}
}

func TestEmptyInboxClearsStaleDiff(t *testing.T) {
	m := minimalModel()
	m.diffEnabled = true
	m.diff = "stale diff"
	m.rawDiff = "stale raw diff"
	m.diffFiles = []diffFile{{Path: "main.go", Body: "stale"}}
	m.renderedFiles = []diffFile{{Path: "main.go", Body: "stale"}}
	m.allPRs = nil
	m.prs = nil

	m.applyPRSettings()

	if m.diff != "" || m.rawDiff != "" || len(m.diffFiles) != 0 || len(m.renderedFiles) != 0 {
		t.Fatalf("diff state not cleared: %#v", m)
	}
	if got := stripANSI(m.diffPanel(80, 10)); !strings.Contains(got, "select PR") {
		t.Fatalf("diffPanel = %q, want select PR", got)
	}
}

func TestFullFileDiffModeRendersSelectedFileContent(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number:     1,
		Repo:       "org/repo",
		HeadRefOID: "abc123",
		Files:      []model.ChangedFile{{Path: "main.go"}},
	}}, 0)
	m.diffEnabled = true
	m.diffMode = DiffModeFile
	m.diffFiles = []diffFile{{Path: "main.go", Body: "diff --git a/main.go b/main.go\n@@ -1,3 +1,3 @@\n line1\n-old line\n+line2\n line3\n"}}
	m.fileContentKey = m.selectedFileContentKey()
	m.fileContent = "line1\nline2\nline3\n"

	content := m.currentDiffContent()
	plain := stripANSI(content)

	if !strings.Contains(plain, " 1 │ line1") || !strings.Contains(plain, "-  │ old line") || !strings.Contains(plain, "+2 │ line2") || !strings.Contains(plain, " 3 │ line3") {
		t.Fatalf("content = %q, want full file with added and removed line markers", content)
	}
	if strings.Contains(plain, "-2 │") {
		t.Fatalf("deleted line should not render a second line-number column: %q", content)
	}
}

func TestInboxMarksDraftPRs(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Title: "draft", Author: "alice", Draft: true}}, 0)

	view := stripANSI(m.inboxPanel(80, 10))

	if !strings.Contains(view, "[draft]") {
		t.Fatalf("inbox missing draft marker: %q", view)
	}
}

// minimalModel returns a Model with enough state for key-handling tests,
// without requiring a real *app.App.
func minimalModel() Model {
	m := Model{}
	m.prs = []model.PullRequest{
		{Number: 1, Repo: "org/repo", Title: "first"},
	}
	m.allPRs = m.prs
	m.selected = 0
	m.mode = ModeInbox
	m.focus = FocusInbox
	m.summaryMode = SummaryModeFiles
	return m
}

func modelWithPRs(prs []model.PullRequest, selected int) Model {
	m := minimalModel()
	m.prs = prs
	m.allPRs = prs
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

func clickMouse(m Model, x, y int) Model {
	next, _ := m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
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

func TestMouseClickSelectsInboxPR(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{
		{Number: 1, Repo: "org/repo", Title: "first"},
		{Number: 2, Repo: "org/repo", Title: "second"},
		{Number: 3, Repo: "org/repo", Title: "third"},
	}, 0)
	m.width = 120
	m.height = 30
	m.mode = ModeInbox
	m.focus = FocusInbox
	m.resizeViewports()

	// Wide layout: body starts below the header, pane content starts below the
	// top border. First PR occupies content lines 0-1, separator line 2, second
	// PR starts at content line 3.
	m = clickMouse(m, 2, 6)

	if m.selected != 1 {
		t.Fatalf("selected = %d, want 1", m.selected)
	}
	if m.focus != FocusInbox {
		t.Fatalf("focus = %v, want FocusInbox", m.focus)
	}
	if m.mode != ModeInbox {
		t.Fatalf("mode = %v, want ModeInbox", m.mode)
	}
}

func TestMouseClickInboxSeparatorOnlyFocuses(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{
		{Number: 1, Repo: "org/repo", Title: "first"},
		{Number: 2, Repo: "org/repo", Title: "second"},
	}, 0)
	m.width = 120
	m.height = 30
	m.mode = ModeDetail
	m.focus = FocusSummary
	m.resizeViewports()

	// Separator after first PR is not selectable.
	// With 3-line layout (2 content + 1 sep): contentY=2 is separator for PR 0.
	// region.y=1, so y=4 → contentY=4-1-1=2, line=2, 2%3=2 → separator.
	m = clickMouse(m, 2, 4)

	if m.selected != 0 {
		t.Fatalf("selected = %d, want unchanged 0", m.selected)
	}
	if m.focus != FocusInbox {
		t.Fatalf("focus = %v, want FocusInbox", m.focus)
	}
	if m.mode != ModeInbox {
		t.Fatalf("mode = %v, want ModeInbox", m.mode)
	}
}

func TestMouseClickSummaryFocusesPane(t *testing.T) {
	m := minimalModel()
	m.width = 120
	m.height = 30
	m.mode = ModeInbox
	m.focus = FocusInbox
	m.resizeViewports()

	// In wide inbox-focused layout, summary pane starts after the tall inbox pane.
	m = clickMouse(m, 2, 24)

	if m.focus != FocusSummary {
		t.Fatalf("focus = %v, want FocusSummary", m.focus)
	}
	if m.mode != ModeDetail {
		t.Fatalf("mode = %v, want ModeDetail", m.mode)
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

func TestInboxSelectedPRStaysWithinWidth(t *testing.T) {
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

func TestInboxTitleDropsZeroPadding(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 12}, {Number: 11}}, 0)
	if got := m.inboxTitle(); got != "Inbox (1/2)" {
		t.Fatalf("inboxTitle = %q, want %q", got, "Inbox (1/2)")
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

func TestLayoutUsesFullWidth(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author", CIStatus: "passing"}}, 0)
	m.width = 120
	m.height = 20
	view := m.View()
	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if got := lipgloss.Width(stripANSI(line)); got != m.width {
			t.Fatalf("line width = %d, want %d: %q", got, m.width, line)
		}
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

func TestTruncateANSIWidthUsesRequestedWidth(t *testing.T) {
	got := lipgloss.Width(stripANSI(truncateANSIWidth("abcdef", 6)))
	if got != 6 {
		t.Fatalf("truncate width = %d, want 6", got)
	}
}

func TestLeftTruncatePath(t *testing.T) {
	cases := []struct {
		path string
		maxW int
		want string
	}{
		// Fits: no truncation.
		{"sa.tf", 20, "sa.tf"},
		// First segment stripped.
		{"terraform/accounts/sa.tf", 20, "…/accounts/sa.tf"},
		// Two segments stripped.
		{"terraform/accounts/emea/eks/sa.tf", 20, "…/emea/eks/sa.tf"},
		// Falls back to last segment only.
		{"terraform/accounts/emea/eks/sa.tf", 10, "…/sa.tf"},
		// Filename itself wider than maxW: hard right-truncate with ellipsis.
		{"verylongfilename.tf", 5, "…e.tf"},
		// Zero width.
		{"terraform/sa.tf", 0, ""},
	}
	for _, tc := range cases {
		got := leftTruncatePath(tc.path, tc.maxW)
		if got != tc.want {
			t.Errorf("leftTruncatePath(%q, %d) = %q, want %q", tc.path, tc.maxW, got, tc.want)
		}
	}
}

func TestLeftColumnHeightsShiftByFocus(t *testing.T) {
	m := minimalModel()
	inboxH, summaryH := m.leftColumnHeights(20)
	if inboxH != 16 || summaryH != 4 {
		t.Fatalf("inbox focus heights = %d/%d, want 16/4", inboxH, summaryH)
	}
	m.focus = FocusSummary
	m.mode = ModeDetail
	inboxH, summaryH = m.leftColumnHeights(20)
	if inboxH != 4 || summaryH != 16 {
		t.Fatalf("summary focus heights = %d/%d, want 4/16", inboxH, summaryH)
	}
}

func TestNumberKeysSwitchSummaryModeGlobally(t *testing.T) {
	m := minimalModel()
	m = pressKey(m, "2")
	if m.summaryMode != SummaryModeCommits {
		t.Fatalf("summaryMode after 2 = %v, want %v", m.summaryMode, SummaryModeCommits)
	}
	m = pressKey(m, "3")
	if m.summaryMode != SummaryModeComments {
		t.Fatalf("summaryMode after 3 = %v, want %v", m.summaryMode, SummaryModeComments)
	}
	m = pressKey(m, "4")
	if m.summaryMode != SummaryModeAtlantis {
		t.Fatalf("summaryMode after 4 = %v, want %v", m.summaryMode, SummaryModeAtlantis)
	}
	m = pressKey(m, "1")
	if m.summaryMode != SummaryModeFiles {
		t.Fatalf("summaryMode after 1 = %v, want %v", m.summaryMode, SummaryModeFiles)
	}
}

func TestCommandOverlayPreservesSelectedInboxColors(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number:   1,
		Repo:     "org/repo",
		Title:    "selected title",
		Author:   "author",
		CIStatus: "passing",
	}}, 0)
	m.width = 100
	m.height = 20
	m.theme = theme.Default()
	m.mode = ModeCommand
	m.focus = FocusInbox
	m.resizeViewports()

	view := m.View()
	want := m.theme.Styles.TitleStrong.Render("selected title")
	if !strings.Contains(view, want) {
		t.Fatalf("selected title colors not preserved; missing %q in %q", want, view)
	}
}

func TestAtlantisApplyWarningForUnsafePR(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number:         1,
		Repo:           "org/repo",
		Title:          "title",
		Author:         "author",
		CIStatus:       "failure",
		ReviewDecision: "REVIEW_REQUIRED",
	}}, 0)
	m.mode = ModeConfirmCommand
	m.pendingCmd = viCommand{Name: "aa", Description: "comment \"atlantis apply\" on selected PR", RequireConfirm: true}

	panel := stripANSI(m.commandPanel())
	if !strings.Contains(panel, "Warning: applying before PR is approved and CI is passing.") {
		t.Fatalf("missing warning: %q", panel)
	}
	if !strings.Contains(panel, "Approval: awaiting approval") || !strings.Contains(panel, "CI: CI failed") {
		t.Fatalf("missing unsafe details: %q", panel)
	}
}

func TestAtlantisSummaryShowsCondensedPlan(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		Title:  "title",
		Author: "author",
		AtlantisPlan: &model.AtlantisPlan{
			Author:  "atlantis",
			Summary: "Plan: 1 to add, 2 to change, 3 to destroy",
			Add:     1,
			Change:  2,
			Destroy: 3,
			Body:    "noisy raw body",
			Layers: []model.AtlantisLayer{
				{Name: "network/prod", Add: 1, Change: 2, Destroy: 3},
				{Name: "app/dev", NoChanges: true},
			},
		},
	}}, 0)
	m.width = 60
	m.height = 16
	m.summaryMode = SummaryModeAtlantis

	view := stripANSI(m.summaryPanel(m.width, m.height))
	if !strings.Contains(view, "Atlantis plan") || !strings.Contains(view, "+1 ~2 -3") || !strings.Contains(view, "Changes") || !strings.Contains(view, "network/prod") || !strings.Contains(view, "No changes") || !strings.Contains(view, "app/dev") {
		t.Fatalf("missing Atlantis summary: %q", view)
	}
	if strings.Contains(view, "noisy raw body") || strings.Contains(view, "Unknown") {
		t.Fatalf("Atlantis summary should omit raw body and unknown label: %q", view)
	}
}

func TestAtlantisSummaryUsesAffectedLayersForUncountedLayers(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		AtlantisPlan: &model.AtlantisPlan{
			Author:  "atlantis",
			Summary: "Plan: 0 to add, 2 to change, 0 to destroy",
			Change:  2,
			Layers: []model.AtlantisLayer{
				{Name: "terraform/accounts/prod / default", Unknown: true},
			},
		},
	}}, 0)
	m.width = 80
	m.height = 16
	m.summaryMode = SummaryModeAtlantis

	view := stripANSI(m.summaryPanel(m.width, m.height))
	if !strings.Contains(view, "Affected layers") || !strings.Contains(view, "terraform/accounts/prod / default") {
		t.Fatalf("missing affected layers: %q", view)
	}
	if strings.Contains(view, "Unknown") || strings.Contains(view, "Plan Summary") || strings.Contains(view, "unchanged attributes hidden") {
		t.Fatalf("Atlantis summary leaked noisy labels: %q", view)
	}
}

func TestCommentSummaryStripsEmojiShortcodes(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		Title:  "title",
		Author: "author",
		Comments: []model.PRComment{{
			Author: "alice",
			Body:   ":rocket: ship it :white_check_mark:",
		}},
	}}, 0)
	m.width = 60
	m.height = 14
	m.summaryMode = SummaryModeComments

	view := stripANSI(m.summaryPanel(m.width, m.height))
	if strings.Contains(view, ":rocket:") || strings.Contains(view, ":white_check_mark:") {
		t.Fatalf("emoji shortcodes not stripped: %q", view)
	}
	if !strings.Contains(view, "ship it") {
		t.Fatalf("comment text missing: %q", view)
	}
}

func TestCommentSummaryWrapsLongLines(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		Title:  "title",
		Author: "author",
		Comments: []model.PRComment{{
			Author: "atlantis",
			Body:   strings.Repeat("0123456789", 12),
		}},
	}}, 0)
	m.width = 44
	m.height = 14
	m.summaryMode = SummaryModeComments
	view := m.summaryPanel(m.width, m.height)
	lines := strings.Split(view, "\n")
	bodyLines := 0
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(stripANSI(line)) > m.width-4 {
			t.Fatalf("comment summary line over width: %q", line)
		}
		if strings.Contains(line, "0123456789") {
			bodyLines++
		}
	}
	if bodyLines != 1 {
		t.Fatalf("long comment body wrapped into %d lines, want 1: %#v", bodyLines, lines)
	}
}

func TestCommitSummaryUsesAvailableWidth(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		Title:  "title",
		Author: "author",
		Commits: []model.PRCommit{{
			OID:      "1234567890abcdef",
			Headline: strings.Repeat("x", 80),
			Author:   "alice",
		}},
	}}, 0)
	m.width = 44
	m.height = 14
	m.summaryMode = SummaryModeCommits
	view := m.summaryPanel(m.width, m.height)
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		t.Fatal("empty commit summary view")
	}
	if got := lipgloss.Width(stripANSI(lines[0])); got < m.width-10 {
		t.Fatalf("commit headline used too little width: %d", got)
	}
}

func TestDiffPanelHonorsPassedWidth(t *testing.T) {
	m := minimalModel()
	m.width = 140
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("abcdefghijklmnopqrstuvwxyz", 8)
	m.syncDiffVP()

	paneW := 44
	view := m.diffPanel(paneW, 10)
	maxLineW := m.diffContentWidth(m.paneInnerWidth(paneW, FocusDiff))
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(stripANSI(line)); got != maxLineW {
			t.Fatalf("diff line width = %d, want %d: %q", got, maxLineW, line)
		}
	}
}

func TestDiffPanelHorizontalScrollShowsLaterContentWithoutWrapping(t *testing.T) {
	m := minimalModel()
	m.width = 120
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", 4)
	m.resizeViewports()

	initial := m.diffPanel(m.diffPaneOuterWidth(), 10)
	if !strings.Contains(initial, "0123456789") {
		t.Fatalf("initial diff missing prefix: %q", initial)
	}

	m = pressSpecial(m, tea.KeyRight)
	afterRight := m.diffPanel(m.diffPaneOuterWidth(), 10)
	if afterRight == initial {
		t.Fatal("diff did not change after horizontal scroll")
	}
	for _, line := range strings.Split(afterRight, "\n") {
		if got := lipgloss.Width(stripANSI(line)); got != m.diffVP.Width {
			t.Fatalf("scrolled diff line width = %d, want %d: %q", got, m.diffVP.Width, line)
		}
	}
}

func TestWideLayoutWithDiffUsesFullWidth(t *testing.T) {
	for _, width := range []int{119, 120, 121} {
		m := modelWithPRs([]model.PullRequest{{
			Number: 1,
			Repo:   "org/repo",
			Title:  "title",
			Author: "author",
		}}, 0)
		m.width = width
		m.height = 20
		m.mode = ModeDiff
		m.focus = FocusDiff
		m.diffEnabled = true
		m.diff = strings.Repeat("line\n", 20)
		m.resizeViewports()

		view := m.View()
		lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
		for _, line := range lines {
			if got := lipgloss.Width(stripANSI(line)); got != m.width {
				t.Fatalf("width %d: line width = %d, want %d: %q", width, got, m.width, line)
			}
		}
	}
}

func TestWideDiffHardTruncatesBeforeRightPadding(t *testing.T) {
	for _, width := range []int{119, 120, 121} {
		m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
		m.width = width
		m.height = 18
		m.mode = ModeDiff
		m.focus = FocusDiff
		m.diffEnabled = true
		m.diff = m.theme.Styles.File.Render("terraform/accounts/emea-production/storage/kafka.tf") + "\n" + strings.Repeat("data.terraform_remote_state.vpc_state.outputs.vpc_id", 4)
		m.resizeViewports()

		paneW := m.diffPaneOuterWidth()
		want := m.diffContentWidth(m.paneInnerWidth(paneW, FocusDiff))
		if m.diffVP.Width != want {
			t.Fatalf("width %d: diff viewport width = %d, want %d", width, m.diffVP.Width, want)
		}
		for _, line := range strings.Split(m.diffPanel(paneW, 8), "\n") {
			if got := lipgloss.Width(stripANSI(line)); got != want {
				t.Fatalf("width %d: diff line width = %d, want %d: %q", width, got, want, line)
			}
		}
	}
}

func TestWideLayoutLongDiffDoesNotWrapOrChangeHeight(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
	m.width = 120
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("data.terraform_remote_state.vpc_state.outputs.vpc_id", 6)
	m.resizeViewports()

	view := m.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) != m.height {
		t.Fatalf("view height = %d, want %d", len(lines), m.height)
	}
	for _, line := range lines {
		if got := lipgloss.Width(stripANSI(line)); got != m.width {
			t.Fatalf("line width = %d, want %d: %q", got, m.width, line)
		}
	}
}

func TestWideLayoutTabbedDiffDoesNotExceedTerminalWidth(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
	m.width = 120
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("\tdata.terraform_remote_state.vpc_state.outputs.vpc_id", 6)
	m.resizeViewports()

	for _, line := range strings.Split(strings.TrimRight(m.View(), "\n"), "\n") {
		if strings.Contains(line, "\t") {
			t.Fatalf("rendered line still contains tab: %q", line)
		}
		if got := lipgloss.Width(stripANSI(line)); got > m.width {
			t.Fatalf("line width = %d, want <= %d: %q", got, m.width, line)
		}
	}
}

func TestSummaryHeaderShowsActiveModeAndFooterDropsShortcut(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
	m.width = 100
	m.height = 18
	m.mode = ModeDetail
	m.focus = FocusSummary
	m.summaryMode = SummaryModeCommits
	view := m.View()
	if strings.Contains(view, "1/2/3/4 summary") {
		t.Fatalf("footer still shows summary shortcut: %q", view)
	}
	if !strings.Contains(stripANSI(view), "Summary 1 2 3 4") {
		t.Fatalf("summary header missing mode numbers: %q", view)
	}
}

// TestStripNonSGRRemovesEraseLineKeepsColor verifies that non-SGR CSI escape
// sequences emitted by delta (e.g. ESC[0K) are stripped while SGR color codes
// are preserved. Leaked ESC[0K caused diff lines to wrap/misalign in the
// terminal because it measures as zero width but erases columns when rendered.
func TestStripNonSGRRemovesEraseLineKeepsColor(t *testing.T) {
	in := "\x1b[31mred\x1b[0m text\x1b[0K end\x1b[2J\x1b[?25h done"
	got := stripNonSGR(in)
	if !strings.Contains(got, "\x1b[31m") || !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("stripNonSGR dropped SGR color codes: %q", got)
	}
	for _, seq := range []string{"\x1b[0K", "\x1b[2J", "\x1b[?25h"} {
		if strings.Contains(got, seq) {
			t.Fatalf("stripNonSGR kept non-SGR CSI %q: %q", seq, got)
		}
	}
	if stripANSI(got) != "red text end done" {
		t.Fatalf("unexpected visible text: %q", stripANSI(got))
	}
}

// TestPaneRendersExactAllocatedWidth guards Bug 2: lipgloss Width() sets the
// content+padding area and adds the border outside, so pane() must size the box
// with width-borderSize, not width-frameSize, or every pane renders 2 cols too
// narrow (leaving dead space at the terminal's right edge).
func TestPaneRendersExactAllocatedWidth(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "x"}}, 0)
	m.theme = theme.Default()
	m.width = 120
	m.height = 24
	for _, focus := range []Focus{FocusInbox, FocusDiff} {
		for _, w := range []int{10, 30, 69, 100} {
			box := m.pane("Title", "content line", w, 6, focus)
			top := strings.SplitN(box, "\n", 2)[0]
			if got := lipgloss.Width(stripANSI(top)); got != w {
				t.Fatalf("focus=%v alloc=%d: pane outer width = %d, want %d", focus, w, got, w)
			}
		}
	}
}

// TestWideLayoutBodyFillsTerminalWidth guards Bug 2 end-to-end with the real
// bordered theme: every rendered line (incl. the diff pane right border) must
// span exactly the terminal width, leaving no dead space on the right.
func TestWideLayoutBodyFillsTerminalWidth(t *testing.T) {
	for _, width := range []int{110, 113, 120, 160, 200} {
		m := modelWithPRs([]model.PullRequest{{Number: 7933, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
		m.theme = theme.Default()
		m.width = width
		m.height = 24
		m.mode = ModeDetail
		m.focus = FocusDiff
		m.diffEnabled = true
		m.diff = strings.Repeat("| EBS latency, IOPS saturation, full disk | ebs.md | EC2 |\n", 20)
		m.resizeViewports()
		for i, line := range strings.Split(m.View(), "\n") {
			if got := lipgloss.Width(stripANSI(line)); got != width {
				t.Fatalf("width=%d line %d width = %d, want %d: %q", width, i, got, width, stripANSI(line))
			}
		}
	}
}

// TestDiffContentNoLeakedNonSGREscapes ensures non-SGR CSI escapes are removed
// from delta output at ingest (guards Bug 1).
func TestDiffContentNoLeakedNonSGREscapes(t *testing.T) {
	in := "162\x1b[0K| line one |\n163| line two |\x1b[0K\n"
	if csiNonSGRPattern.MatchString(stripNonSGR(in)) {
		t.Fatalf("non-SGR CSI escape survived stripNonSGR: %q", stripNonSGR(in))
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
