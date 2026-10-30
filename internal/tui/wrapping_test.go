package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/senilio/zappr/internal/config"
	"github.com/senilio/zappr/internal/model"
	"go.yaml.in/yaml/v3"
)

func TestWrappedSummaryRetainsContentAndSelectionRows(t *testing.T) {
	for _, mode := range []SummaryMode{SummaryModeCommits} {
		m := wrappedModel(t, "summary")
		m.width, m.height, m.summaryMode = 24, 12, mode
		m.prs[0].Commits = []model.PRCommit{{OID: "abcdef0", Headline: "alpha bravo charlie delta echo FINISH"}, {OID: "1234567", Headline: "second"}}
		content := stripANSI(m.summaryContent(m.prs[0], 20))
		joined := strings.Join(strings.Fields(content), "")
		if want := "FINISH"; !strings.Contains(joined, want) {
			t.Errorf("mode %d lost content: %q", mode, content)
		}
		for _, row := range strings.Split(content, "\n") {
			if lipgloss.Width(row) > 20 {
				t.Errorf("mode %d overflow: %q", mode, row)
			}
		}
		m.resizeViewports()
		m.summaryVP.SetHeight(4)
		m.fileIndex, m.commitIndex, m.commentIndex = 1, 1, 1
		m.syncSummaryVP()
		m.syncSummaryVPScroll()
		view := stripANSI(m.summaryPanel(24, 6))
		if !strings.Contains(view, "second") {
			t.Errorf("mode %d selection not visible: %q", mode, view)
		}
	}
}

func TestWrappedSummaryKeepsCommentsOnOneRow(t *testing.T) {
	m := wrappedModel(t, "summary")
	m.summaryMode = SummaryModeComments
	m.prs[0].Comments = []model.PRComment{{Author: "a-very-long-commenter-name-FINISH", CreatedAt: time.Now()}, {Author: "second"}}
	content, spans := m.summaryLayout(m.prs[0], 20)
	lines := strings.Split(strings.TrimSuffix(stripANSI(content), "\n"), "\n")
	if len(lines) != 2 || strings.Contains(content, "FINISH") || !strings.HasPrefix(lines[0], "Today ") {
		t.Fatalf("comment list wrapped or not cut off: %q", lines)
	}
	if len(spans) != 2 || spans[1] != (rowSpan{1, 2}) {
		t.Fatalf("unexpected spans: %#v", spans)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 20 {
			t.Fatalf("comment row exceeds width: %q", line)
		}
	}
}

func TestWrappedSummaryKeepsFilesOnOneRow(t *testing.T) {
	m := wrappedModel(t, "summary")
	m.summaryMode = SummaryModeFiles
	m.prs[0].Files = []model.ChangedFile{{Path: "dir/a.go", Status: "modified", Additions: 1, Deletions: 1}, {Path: "dir/b.go", Status: "added", Additions: 2}}
	content, spans := m.summaryLayout(m.prs[0], 80)
	lines := strings.Split(strings.TrimSuffix(stripANSI(content), "\n"), "\n")
	if len(lines) != 3 || !strings.Contains(lines[1], "a.go") || !strings.Contains(lines[1], "modified") {
		t.Fatalf("file summary wrapped: %q", lines)
	}
	if len(spans) != 2 || spans[1] != (rowSpan{2, 3}) {
		t.Fatalf("unexpected spans: %#v", spans)
	}
}

func TestWrappedDiffPreservesGuttersAndSource(t *testing.T) {
	m := wrappedModel(t, "diff")
	m.diffEnabled = true
	m.diff = "header\n  1 ⋮  1 │ alpha bravo charlie delta echo FINISH\n  2 ⋮  2 │ second\n"
	view := stripANSI(m.renderDiffViewport(24, 20))
	if !strings.Contains(view, "FINISH") {
		t.Fatalf("diff still clipped: %q", view)
	}
	for _, row := range strings.Split(view, "\n") {
		if strings.Contains(row, "delta") && !strings.Contains(row, "│") {
			t.Errorf("continuation lost gutter: %q", row)
		}
		if lipgloss.Width(row) > 24 {
			t.Errorf("diff overflow: %q", row)
		}
	}
	// Wrap an arbitrarily long token without dropping any characters.
	m.diff = "+" + strings.Repeat("X", 100) + "END"
	view = stripANSI(m.renderDiffViewport(12, 30))
	if strings.Count(view, "X") != 100 || !strings.Contains(view, "END") {
		t.Fatalf("source characters lost: %q", view)
	}
}

func TestWrappedFullFilePreservesLineNumbers(t *testing.T) {
	m := wrappedModel(t, "diff")
	m.diffEnabled, m.diffMode = true, DiffModeFile
	m.prs[0].Files = []model.ChangedFile{{Path: "test.go"}}
	m.fileContentKey = m.selectedFileContentKey()
	m.fileContent = "alpha bravo charlie delta echo FINISH\nsecond\n"
	view := stripANSI(m.renderDiffViewport(20, 15))
	if !strings.Contains(view, "FINISH") || strings.Count(view, "1 │") != 1 || !strings.Contains(view, "2 │ second") {
		t.Fatalf("incorrect full-file wrap: %q", view)
	}
}

func TestDeltaWrapsSideBySideColumns(t *testing.T) {
	if _, err := exec.LookPath("delta"); err != nil {
		t.Skip("delta not installed")
	}
	raw := "diff --git a/test b/test\n--- a/test\n+++ b/test\n@@ -1 +1 @@\n-old " + strings.Repeat("word ", 25) + "ENDOLD\n+new " + strings.Repeat("word ", 25) + "ENDNEW\n"
	content, err := renderDiff(context.Background(), raw, 60, true, true)
	if err != nil {
		t.Fatal(err)
	}
	// Native side-by-side rendering must not silently discard the ends.
	if !strings.Contains(stripANSI(content), "ENDOLD") || !strings.Contains(stripANSI(content), "ENDNEW") {
		t.Fatalf("side-by-side source was truncated: %q", stripANSI(content))
	}
}

func wrappedModel(t *testing.T, pane string) Model {
	t.Helper()
	m := modelWithPRs([]model.PullRequest{{Title: "alpha bravo charlie delta echo foxtrot", Number: 1}, {Title: "second", Number: 2}}, 0)
	if err := yaml.Unmarshal([]byte("wrap:\n  "+pane+": true\n"), &m.cfg); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestWrapShortcutFocusAndPersistence(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, focus := range []Focus{FocusInbox, FocusSummary, FocusDiff} {
		m := minimalModel()
		m.cfg = config.Default()
		m.focus = focus
		if focus == FocusDiff {
			m.mode = ModeDiff
			m.width = 60
		}
		next, cmd := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
		m = next.(Model)
		if cmd == nil {
			t.Fatalf("focus %d: w did not save wrapping", focus)
		}
		runCmd(cmd)
		data, _ := yaml.Marshal(m.cfg)
		pane := []string{"inbox", "summary", "diff"}[focus]
		if !strings.Contains(string(data), pane+": true") {
			t.Fatalf("wrong pane toggled: %s", data)
		}
		loaded, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		data, _ = yaml.Marshal(loaded)
		if !strings.Contains(string(data), pane+": true") {
			t.Fatalf("wrap not persisted: %s", data)
		}
	}
}

func TestRapidWrapSavesCannotOverwriteNewerSettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := minimalModel()
	m.cfg = config.Default()
	next, first := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(Model)
	m.focus = FocusSummary
	next, second := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(Model)
	if first == nil || second == nil {
		t.Fatal("missing save commands")
	}
	runCmd(second)
	runCmd(first)
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := yaml.Marshal(m.cfg)
	got, _ := yaml.Marshal(loaded)
	if string(got) != string(want) {
		t.Fatalf("older save overwrote current config\ngot %s\nwant %s", got, want)
	}
}

func TestWrapSaveErrorPreservesPaneAndMode(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocked)
	m := minimalModel()
	m.focus, m.mode = FocusSummary, ModeDetail
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("missing save")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.error == "" || m.mode != ModeDetail || m.focus != FocusSummary {
		t.Fatalf("save error not surfaced safely: %q mode=%d focus=%d", m.error, m.mode, m.focus)
	}
}

func TestWrappedInboxTitlesAndClickRows(t *testing.T) {
	m := wrappedModel(t, "inbox")
	view := stripANSI(m.inboxPanel(20, 40))
	if !strings.Contains(view, "foxtrot") {
		t.Fatalf("title still truncated: %q", view)
	}
	rows := strings.Split(strings.TrimRight(view, "\n"), "\n")
	divider := -1
	for i, row := range rows {
		if strings.Contains(row, "──") {
			divider = i
			break
		}
	}
	if divider < 3 {
		t.Fatalf("expected variable title height: %q", view)
	}
	region := paneRegion{focus: FocusInbox, w: 20, h: 40}
	for row := 0; row < divider; row++ {
		if idx, ok := m.inboxIndexAt(region, 2, row+1); !ok || idx != 0 {
			t.Fatalf("continuation row %d selects %d, %v", row, idx, ok)
		}
	}
	if _, ok := m.inboxIndexAt(region, 2, divider+1); ok {
		t.Fatal("divider selected a PR")
	}
	if idx, ok := m.inboxIndexAt(region, 2, divider+2); !ok || idx != 1 {
		t.Fatalf("second title selects %d, %v", idx, ok)
	}
	for _, row := range rows {
		if lipgloss.Width(row) > 16 {
			t.Fatalf("overflow: %q", row)
		}
	}
}

func TestWrapShortcutDoesNotInterceptInput(t *testing.T) {
	for _, mode := range []Mode{ModeComment, ModeCommand} {
		m := minimalModel()
		m.mode = mode
		next, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
		m = next.(Model)
		if m.comment != "w" && m.commandInput != "w" {
			t.Fatalf("w intercepted in mode %d", mode)
		}
	}
}

func TestWrappedInboxPreservesUnicodeGraphemes(t *testing.T) {
	m := wrappedModel(t, "inbox")
	m.prs[0].Title = "👩‍🔬👩‍🔬Z"
	content, _ := m.inboxContent(6)
	if !strings.Contains(stripANSI(content), m.prs[0].Title) {
		t.Fatalf("Unicode title lost characters: %q", stripANSI(content))
	}
}

func TestWrappedInboxTallItemCanBePaged(t *testing.T) {
	m := wrappedModel(t, "inbox")
	m.width, m.height = 24, 8
	m.prs[0].Title = strings.Repeat("alpha bravo charlie ", 20)
	m.resizeViewports()
	first := stripANSI(m.inboxPanel(24, 7))
	if !strings.HasPrefix(first, "alpha") {
		t.Fatalf("tall selection must start at title: %q", first)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = next.(Model)
	if m.inboxVP.YOffset() == 0 {
		t.Fatal("cannot page within tall title")
	}
	if m.inboxOffset(20, 5) != m.inboxVP.YOffset() {
		t.Fatal("rendering snaps back after manual scrolling")
	}
	// The first title must remain visible on further renders, without resetting scroll.
	if !strings.Contains(stripANSI(m.inboxPanel(24, 7)), "bravo") {
		t.Fatal("paged title disappeared")
	}
}

func TestWrapToggleAndResizePreserveDiffLineAnchor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := minimalModel()
	m.width, m.height, m.focus, m.diffEnabled = 40, 9, FocusDiff, true
	m.diff = strings.Repeat("alpha bravo charlie delta echo foxtrot\n", 12) + "ANCHOR\n" + strings.Repeat("tail\n", 20)
	m.resizeViewports()
	m.diffVP.SetYOffset(12)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(Model)
	if m.diffAnchor() != 12 {
		t.Fatalf("toggle changed source anchor to %d", m.diffAnchor())
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 28, Height: 9})
	m = next.(Model)
	if m.diffAnchor() != 12 {
		t.Fatalf("resize changed source anchor to %d", m.diffAnchor())
	}
	if !strings.HasPrefix(stripANSI(m.renderDiffViewport(m.diffVP.Width(), 6)), "ANCHOR") {
		t.Fatal("anchored diff line not visible")
	}
}

func TestWrapBindingCanBeRemapped(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Actions["toggle-wrap"] = config.ActionConfig{Keys: []string{"W"}}
	m, _ := configuredModel(t, cfg, true)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(Model)
	if cmd != nil || m.cfg.Wrap.Inbox {
		t.Fatal("old binding still toggles wrapping")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'W', Text: "W"})
	m = next.(Model)
	if !m.cfg.Wrap.Inbox || !strings.Contains(stripANSI(m.helpPanel()), "W") {
		t.Fatal("remapped binding is not used")
	}
}

func TestManualInboxScrollKeepsPRWhenWrappingChanges(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := wrappedModel(t, "inbox")
	for i := 3; i < 15; i++ {
		m.prs = append(m.prs, model.PullRequest{Number: i, Title: "another long title with words"})
	}
	m.width, m.height = 24, 9
	m.resizeViewports()
	_, spans := m.inboxContent(20)
	m.inboxManualScroll = true
	m.inboxVP.SetYOffset(spans[3].start)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(Model)
	_, spans = m.inboxContent(20)
	if m.inboxVP.YOffset() != spans[3].start {
		t.Fatalf("wrap moved top PR: offset=%d want=%d", m.inboxVP.YOffset(), spans[3].start)
	}
	next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = next.(Model)
	if !m.inboxManualScroll || m.inboxOffset(20, m.inboxVP.Height()) != m.inboxVP.YOffset() {
		t.Fatal("mouse wheel scrolling snaps to selection")
	}
}

func TestInboxWheelEnablesManualScroll(t *testing.T) {
	m := wrappedModel(t, "inbox")
	m.width, m.height = 24, 8
	m.prs[0].Title = strings.Repeat("alpha bravo charlie ", 20)
	m.resizeViewports()
	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = next.(Model)
	if !m.inboxManualScroll || m.inboxVP.YOffset() == 0 {
		t.Fatal("wheel does not retain scroll within tall item")
	}
}

func TestSideBySideWrapReflowsCachedRawDiff(t *testing.T) {
	if _, err := exec.LookPath("delta"); err != nil {
		t.Skip("delta not installed")
	}
	m := wrappedModel(t, "diff")
	m.width, m.height, m.sideBySide, m.diffEnabled = 64, 25, true, true
	m.rawDiff = "diff --git a/test b/test\n--- a/test\n+++ b/test\n@@ -1 +1 @@\n-old " + strings.Repeat("word ", 25) + "ENDOLD\n+new " + strings.Repeat("word ", 25) + "ENDNEW\n"
	m.diff, _ = renderDiff(context.Background(), m.rawDiff, 6000, false)
	m.resizeViewports()
	cmd := m.sideLayoutCmd(0)
	if cmd == nil {
		t.Fatal("missing side-by-side layout")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	view := stripANSI(m.renderDiffViewport(m.diffVP.Width(), 30))
	for _, marker := range []string{"ENDOLD", "ENDNEW"} {
		if !strings.Contains(view, marker) {
			t.Fatalf("missing %s in side-by-side pane: %q", marker, view)
		}
	}
	for _, row := range strings.Split(view, "\n") {
		if lipgloss.Width(row) > m.diffVP.Width() {
			t.Fatalf("side-by-side overflow: %q", row)
		}
	}
	if cmd := m.sideLayoutCmd(0); cmd != nil {
		t.Fatal("same layout rerendered")
	}
	// A resize must queue local formatting, not a provider fetch.
	next, cmd = m.Update(tea.WindowSizeMsg{Width: 54, Height: 25})
	m = next.(Model)
	if cmd == nil || m.sideDiffRequested == m.sideDiffKey {
		t.Fatal("resize did not reformat cached raw diff")
	}
}

func TestSideLayoutFileNavigationStartsAtTop(t *testing.T) {
	m := wrappedModel(t, "diff")
	m.width, m.height, m.sideBySide, m.diffEnabled = 64, 12, true, true
	m.diff = strings.Repeat("context\n", 50)
	m.diffFiles = []diffFile{{Path: "first", Body: "first raw diff"}, {Path: "second", Body: "second raw diff"}}
	m.rawDiff = "whole PR diff"
	m.resizeViewports()
	m.diffVP.SetYOffset(12)
	m.actions.keys[actionContextNormal]["n"] = tuiAction{ID: "next-item"}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if cmd == nil {
		t.Fatal("missing layout command")
	}
	msg, ok := cmd().(sideLayoutMsg)
	if !ok {
		t.Fatal("expected one local layout command")
	}
	if msg.anchor != 0 {
		t.Fatalf("file navigation retained previous file offset %d", msg.anchor)
	}
}

func TestDiscardedSideLayoutAllowsRequestingSameLayoutAgain(t *testing.T) {
	m := wrappedModel(t, "diff")
	m.width, m.height, m.sideBySide, m.diffEnabled = 64, 12, true, true
	m.rawDiff, m.diff = "raw diff", strings.Repeat("context\n", 50)
	m.resizeViewports()
	m.sideDiffKey = m.sideLayoutKey()
	m.sideDiffContent = m.diff
	widthA := m.diffVP.Width()
	m.diffVP.SetWidth(m.diffVP.Width() + 10)
	if m.sideLayoutCmd(0) == nil {
		t.Fatal("missing layout B request")
	}
	keyB := m.sideDiffRequested
	m.diffVP.SetWidth(widthA)
	next, _ := m.Update(sideLayoutMsg{key: keyB, content: "stale B"})
	m = next.(Model)
	if m.sideDiffRequested != "" {
		t.Fatal("discarded completion retained a pending request")
	}
	m.diffVP.SetWidth(m.diffVP.Width() + 10)
	if m.sideLayoutCmd(0) == nil {
		t.Fatal("discarded B layout permanently suppressed another request")
	}
	// A stale completion must not clear a different, newer request.
	m.sideDiffRequested = "newer request"
	m.diffVP.SetWidth(widthA)
	next, _ = m.Update(sideLayoutMsg{key: keyB})
	if next.(Model).sideDiffRequested != "newer request" {
		t.Fatal("stale completion cleared the newer request")
	}
}

func TestSideLayoutCompletionPreservesSubsequentScrolling(t *testing.T) {
	m := wrappedModel(t, "diff")
	m.width, m.height, m.sideBySide, m.diffEnabled = 64, 12, true, true
	m.rawDiff = "raw diff"
	m.diff = strings.Repeat("context\n", 50)
	m.resizeViewports()
	cmd := m.sideLayoutCmd(0)
	if cmd == nil {
		t.Fatal("missing layout request")
	}
	msg := cmd().(sideLayoutMsg)
	// Formatter output is irrelevant here; keep a deterministic line mapping.
	msg.content, msg.err = m.diff, nil
	m.diffVP.SetYOffset(7)
	next, _ := m.Update(msg)
	if got := next.(Model).diffAnchor(); got != 7 {
		t.Fatalf("formatter completion undid scrolling: logical row=%d want=7", got)
	}
}

func TestSideLayoutFailureSettlesRequestWithoutRetryLoop(t *testing.T) {
	m := wrappedModel(t, "diff")
	m.sideBySide, m.diffEnabled = true, true
	m.rawDiff = "raw diff"
	key := m.sideLayoutKey()
	m.sideDiffRequested = key
	next, _ := m.Update(sideLayoutMsg{key: key, err: errors.New("formatter failed")})
	m = next.(Model)
	if m.sideDiffRequested != "" {
		t.Fatal("failed formatter request still marked pending")
	}
	if m.sideLayoutCmd(0) != nil {
		t.Fatal("failed layout automatically retries forever")
	}
	m.diffVP.SetWidth(m.diffVP.Width() + 1)
	if m.sideLayoutCmd(0) == nil {
		t.Fatal("changed layout cannot retry formatting")
	}
}

func atlantisInboxModel(t *testing.T, wrapInbox bool) Model {
	t.Helper()
	checks := atlantisChecks("SUCCESS", time.Now().Add(-4*time.Minute))
	m := modelWithPRs([]model.PullRequest{
		{Number: 1, Title: "first", Author: "a", Checks: checks},
		{Number: 2, Title: "second", Author: "b", Checks: checks},
	}, 0)
	m.cfg.Wrap.Inbox = wrapInbox
	return m
}

func TestInboxShowsAtlantisRowOnSelectedOnly(t *testing.T) {
	m := atlantisInboxModel(t, false)
	content, spans := m.inboxContent(80)
	if got := spans[0].end - spans[0].start; got != 3 {
		t.Errorf("selected span = %d rows, want 3", got)
	}
	if got := spans[1].end - spans[1].start; got != 2 {
		t.Errorf("unselected span = %d rows, want 2", got)
	}
	if got := strings.Count(stripANSI(content), "atlantis ·"); got != 1 {
		t.Errorf("atlantis row appears %d times, want 1", got)
	}

	m.prs = append(m.prs, model.PullRequest{Number: 3, Title: "unplanned", Author: "c"})
	m.allPRs = m.prs
	m.selected = 2
	content, spans = m.inboxContent(80)
	if got := spans[2].end - spans[2].start; got != 3 {
		t.Errorf("unplanned selected span = %d rows, want 3", got)
	}
	if !strings.Contains(stripANSI(content), "no plan") {
		t.Errorf("unplanned selected PR missing no plan row: %q", stripANSI(content))
	}

	plain := modelWithPRs([]model.PullRequest{{Number: 1, Title: "a"}, {Number: 2, Title: "b"}}, 0)
	_, spans = plain.inboxContent(80)
	if got := spans[0].end - spans[0].start; got != 2 {
		t.Errorf("non-atlantis selected span = %d rows, want 2", got)
	}
}

func TestInboxAtlantisRowStaysOneLineWhenWrapped(t *testing.T) {
	m := atlantisInboxModel(t, true)
	m.prs[0].Title = "alpha bravo charlie delta echo foxtrot golf hotel"
	content, spans := m.inboxContent(14)
	titleRows := len(strings.Split(strings.TrimSpace(stripANSI(wrapLines(m.prs[0].Title, 14))), "\n"))
	if got, want := spans[0].end-spans[0].start, titleRows+1+1; got != want {
		t.Errorf("selected span = %d rows, want %d (title %d + meta + atlantis)", got, want, titleRows)
	}
	lines := strings.Split(content, "\n")
	if got := strings.TrimSpace(stripANSI(lines[spans[0].end-1])); !strings.HasPrefix(got, "atlantis") {
		t.Errorf("last selected row = %q, want atlantis row", got)
	}
}
