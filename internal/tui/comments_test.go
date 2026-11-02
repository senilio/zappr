package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func nativeCommentModel(t *testing.T) Model {
	t.Helper()
	m := commentPaneModel(t)
	at := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	m.prs[0].Comments = []model.PRComment{
		{ID: "101", Author: "alex", Body: "\n\nPlan result a.*[b] ÉCOLE\nsecond line", CreatedAt: at},
		{ID: "101", Kind: "inline", Author: "alexander", Path: "src/ÉCOLE.go", Line: 9, Body: "Inline result", CreatedAt: at},
		{ID: "103", Kind: "review", Author: "alex", Body: "Human review", CreatedAt: at.Add(-time.Minute)},
	}
	m.allPRs = m.prs
	m.commentState = commentViewState{}
	m.commentIndex = 0
	m.reconcileComments()
	m.syncSummaryVP()
	return m
}

func TestVisibleCommentsPreservesNativeSelectionOnEdit(t *testing.T) {
	m := nativeCommentModel(t)
	m = pressSpecial(m, tea.KeyDown)
	if c, _ := m.selectedComment(); c.Key() != "inline:101" {
		t.Fatalf("native kind collision: %#v", c)
	}
	m.prs[0].Comments[1].Body = "edited inline body"
	m.prs[0].Comments[1].UpdatedAt = time.Now()
	m.prs[0].Comments = append([]model.PRComment{{ID: "new", Author: "newer", Body: "new", CreatedAt: time.Now()}}, m.prs[0].Comments...)
	m.reconcileComments()
	if c, _ := m.selectedComment(); c.Key() != "inline:101" || c.Body != "edited inline body" || m.commentIndex != 2 {
		t.Fatalf("edit/insert moved native selection: %#v index=%d", c, m.commentIndex)
	}
	m = pressKey(m, "1")
	m = pressKey(m, "3")
	if c, _ := m.selectedComment(); c.Key() != "inline:101" {
		t.Fatalf("tab change lost selection: %#v", c)
	}
	m.prs[0].Comments = append(m.prs[0].Comments[:2], m.prs[0].Comments[3:]...)
	m.reconcileComments()
	if c, _ := m.selectedComment(); c.ID != "103" {
		t.Fatalf("removed selection did not choose nearest row: %#v", c)
	}
}

func TestCommentMetadataMeasuredSpans(t *testing.T) {
	m := nativeCommentModel(t)
	m.prs[0].Comments[0].Author = strings.Repeat("long author ", 8) + "FINISH"
	content, spans := m.summaryLayout(m.prs[0], 24)
	if len(spans) != 3 || spans[0] != (rowSpan{0, 1}) || strings.Contains(content, "FINISH") {
		t.Fatalf("default comment not bounded to one row: %#v %q", spans, content)
	}
	m.cfg.Wrap.Summary = true
	content, spans = m.summaryLayout(m.prs[0], 24)
	if len(spans) != 3 || spans[0].end <= 2 || spans[1].start != spans[0].end || !strings.Contains(content, "FINISH") {
		t.Fatalf("wrapped metadata not measured: %#v %q", spans, content)
	}
	for _, line := range strings.Split(content, "\n") {
		if ansi.StringWidth(line) > 24 {
			t.Fatalf("metadata overflow: %q", line)
		}
	}
	m.summaryVP.SetHeight(3)
	m = pressSpecial(m, tea.KeyDown)
	m.syncSummaryVPScroll()
	_, actual := m.summaryLayout(m.prs[0], m.summaryVP.Width())
	selected := actual[m.commentIndex]
	if actual[0].end <= 2 || selected.start < m.summaryVP.YOffset() || selected.end > m.summaryVP.YOffset()+m.summaryVP.Height() {
		t.Fatalf("selection used fixed-stride scrolling: y=%d spans=%#v", m.summaryVP.YOffset(), actual)
	}
}

func TestCommentContextAndSelectionWithoutColor(t *testing.T) {
	m := nativeCommentModel(t)
	m.theme = uitheme.New(uitheme.DarkANSI(), colorprofile.ASCII, true)
	view := stripANSI(m.summaryPanel(70, 16))
	if strings.HasPrefix(view, "> ") || strings.Contains(view, "Plan result a.*[b]") || !strings.Contains(view, "alex · comment") {
		t.Fatalf("incorrect metadata-only list: %q", view)
	}
	m = pressSpecial(m, tea.KeyDown)
	if title := m.diffTitle(); title != "Comment" {
		t.Fatalf("comment title is not sparse: %q", title)
	}
	if detail := stripANSI(m.currentDiffContent()); !strings.Contains(detail, "src/ÉCOLE.go:9") {
		t.Fatalf("inline location missing from detail: %q", detail)
	}
}
