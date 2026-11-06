package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func inboxLines(m Model, innerW int) []string {
	content, _ := m.inboxContent(innerW)
	return strings.Split(content, "\n")
}

func TestCIPillState(t *testing.T) {
	cases := map[string]uitheme.PillState{
		"passing": uitheme.PillPassed, "success": uitheme.PillPassed, "PASSING": uitheme.PillPassed,
		"pending": uitheme.PillWait, "queued": uitheme.PillWait, "in_progress": uitheme.PillWait,
		"failing": uitheme.PillFailed, "failed": uitheme.PillFailed, "failure": uitheme.PillFailed, "error": uitheme.PillFailed,
		"unknown": uitheme.PillNone, "": uitheme.PillNone,
	}
	for in, want := range cases {
		if got := ciPillState(in); got != want {
			t.Errorf("ciPillState(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestOccasionalPills(t *testing.T) {
	labels := func(pr model.PullRequest) string {
		var out []string
		for _, p := range occasionalPills(pr) {
			out = append(out, p.label)
		}
		return strings.Join(out, " ")
	}
	cases := []struct {
		pr   model.PullRequest
		want string
	}{
		{model.PullRequest{Draft: true, AutoMerge: true}, "draft auto"},
		{model.PullRequest{MergeState: "BEHIND"}, "behind"},
		{model.PullRequest{MergeState: "DIRTY"}, "conflict"},
		{model.PullRequest{MergeState: "BLOCKED", CIStatus: "passing", ReviewDecision: "APPROVED"}, "blocked"},
		{model.PullRequest{MergeState: "BLOCKED", CIStatus: "pending", ReviewDecision: "APPROVED"}, ""},
		{model.PullRequest{MergeState: "BLOCKED", CIStatus: "passing", ReviewDecision: "REVIEW_REQUIRED"}, ""},
		{model.PullRequest{MergeState: "UNKNOWN"}, ""},
		{model.PullRequest{MergeState: "CLEAN"}, ""},
	}
	for _, tc := range cases {
		if got := labels(tc.pr); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.pr, got, tc.want)
		}
	}
}

func TestInboxRow2RightAligned(t *testing.T) {
	prs := []model.PullRequest{
		{Number: 1, Title: "one", Author: "alice", Draft: true, MergeState: "BEHIND", CIStatus: "passing", ReviewDecision: "APPROVED"},
		{Number: 22, Title: "two", Author: "bob", CIStatus: "failing", ReviewDecision: "REVIEW_REQUIRED"},
	}
	m := modelWithPRs(prs, 0)
	m.cfg.Pills = "square"
	lines := inboxLines(m, 60)
	// Each item has a title and metadata row, separated by a divider.
	rows := map[int]string{0: stripANSI(lines[1]), 1: stripANSI(lines[len(lines)-1])}
	icons := map[int]string{0: "✓", 1: "…"}
	col := -1
	for i, line := range rows {
		if !strings.HasSuffix(line, " ci   "+icons[i]+" ") {
			t.Fatalf("item %d row 2 = %q", i, line)
		}
		if w := ansi.StringWidth(line); w != 60 {
			t.Fatalf("item %d row 2 width = %d", i, w)
		}
		c := ansi.StringWidth(line[:strings.LastIndex(line, " ci ")])
		if col >= 0 && c != col {
			t.Fatalf("CI column differs: %d vs %d", c, col)
		}
		col = c
	}
	if !strings.Contains(rows[0], " draft ") || !strings.Contains(rows[0], " behind ") {
		t.Fatalf("occasional pills missing: %q", rows[0])
	}
}

func TestInboxReviewPillFitsNarrowRows(t *testing.T) {
	pr := model.PullRequest{Number: 123, Author: "author", Draft: true, AutoMerge: true, MergeState: "BEHIND", CIStatus: "passing", ReviewDecision: "APPROVED"}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	for _, noColor := range []bool{false, true} {
		m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, noColor)
		for _, shape := range []string{"rounded", "square"} {
			m.cfg.Pills = shape
			for _, width := range []int{1, 3, 7, 8, 9, 12, 30, 60} {
				row := m.inboxMetaRow(pr, true, width)
				if ansi.StringWidth(row) != width {
					t.Errorf("noColor=%v shape=%s width=%d: row width=%d", noColor, shape, width, ansi.StringWidth(row))
				}
				if width >= 8 {
					fixed := m.theme.Pill("ci", uitheme.PillPassed, m.pillShape(), m.cfg.EffectivePillOpacity()) + " " + m.theme.ReviewPill(pr.ReviewDecision, pr.Draft, m.pillShape(), m.cfg.EffectivePillOpacity())
					if !strings.HasSuffix(stripANSI(row), stripANSI(fixed)) {
						t.Errorf("fixed status pills dropped or misaligned at width %d: %q", width, stripANSI(row))
					}
				}
			}
		}
	}
}

func TestInboxTwoRowsWithAtlantisPillsAndPlainAuthor(t *testing.T) {
	pr := model.PullRequest{Number: 1234, Title: "title", Author: "alice", ReviewersLoaded: true,
		Reviewers: []model.Reviewer{{Login: "approver", State: "APPROVED"}},
		Checks:    atlantisChecks("SUCCESS", time.Now()), ReviewDecision: "APPROVED"}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	content, spans := m.inboxContent(80)
	lines := strings.Split(content, "\n")
	if len(lines) != 2 || spans[0] != (rowSpan{0, 2}) {
		t.Fatalf("want two rows, got spans %v: %q", spans, content)
	}
	if !strings.Contains(lines[1], m.theme.Styles.App.Render("#1234 by alice")) {
		t.Fatalf("selected metadata is not plain text: %q", lines[1])
	}
	if !strings.Contains(stripANSI(lines[1]), "plan ok") || !strings.Contains(stripANSI(lines[1]), "ci") {
		t.Fatalf("metadata lacks status pills: %q", lines[1])
	}
	if strings.Contains(stripANSI(content), "approver") || strings.Contains(stripANSI(content), "reviews") {
		t.Fatalf("dedicated review details remain: %q", content)
	}
	disabled := false
	m.cfg.Atlantis.Enabled = &disabled
	content, _ = m.inboxContent(80)
	if strings.Contains(stripANSI(content), "plan ok") {
		t.Fatalf("Atlantis pill shown when disabled: %q", content)
	}
}

func TestInboxRow2NeverShowsTF(t *testing.T) {
	pr := model.PullRequest{Number: 1, Title: "t", Author: "a", Checks: []model.Check{{Context: "atlantis/plan", State: "SUCCESS", StartedAt: time.Now()}}}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	m.cfg.Pills = "square"
	if row := stripANSI(inboxLines(m, 60)[1]); strings.Contains(row, "TF") || !strings.Contains(row, " ci ") {
		t.Fatalf("row 2 should retain CI without TF: %q", row)
	}
	disabled := false
	m.cfg.Atlantis.Enabled = &disabled
	m.inboxLayoutCache = nil
	if row := stripANSI(inboxLines(m, 60)[1]); strings.Contains(row, "TF") {
		t.Fatalf("TF shown with atlantis disabled: %q", row)
	}
}

func TestInboxRow2NarrowWidthDropsOccasionalPillsFirst(t *testing.T) {
	pr := model.PullRequest{Number: 1, Title: "t", Author: "a-very-long-author-name", Draft: true, AutoMerge: true, MergeState: "DIRTY"}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	m.cfg.Pills = "square"
	row := stripANSI(inboxLines(m, 40)[1])
	for _, want := range []string{" draft ", " auto ", " conflict ", " ci ", "#1"} {
		if !strings.Contains(row, want) {
			t.Fatalf("width 40 row lacks %q: %q", want, row)
		}
	}
	if strings.Contains(row, "a-very-long-author-name") {
		t.Fatalf("width 40 author not truncated: %q", row)
	}
	m.inboxLayoutCache = nil
	row = stripANSI(inboxLines(m, 22)[1])
	if strings.Contains(row, "draft") || strings.Contains(row, "auto") {
		t.Fatalf("width 22 kept draft/auto: %q", row)
	}
	for _, want := range []string{" conflict ", " ci ", "#1"} {
		if !strings.Contains(row, want) {
			t.Fatalf("width 22 row lacks %q: %q", want, row)
		}
	}
	for _, w := range []int{18, 10, 4} {
		m.inboxLayoutCache = nil
		for _, line := range inboxLines(m, w) {
			if ansi.StringWidth(line) > w {
				t.Fatalf("width %d line too wide: %q", w, line)
			}
		}
	}
}

func TestInboxTitleRowHasNoBadges(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Title: "title", Author: "a", Draft: true, AutoMerge: true}}, 0)
	m.cfg.Pills = "square"
	lines := inboxLines(m, 60)
	title := stripANSI(lines[0])
	for _, bad := range []string{"[draft]", "[auto]", "draft", "auto"} {
		if strings.Contains(title, bad) {
			t.Fatalf("title row has %q: %q", bad, title)
		}
	}
	row := stripANSI(lines[1])
	if !strings.Contains(row, " draft ") || !strings.Contains(row, " auto ") {
		t.Fatalf("row 2 lacks draft/auto pills: %q", row)
	}
}

func TestInboxLayoutCacheInvalidatesOnPillShape(t *testing.T) {
	m := scrollModel(t, 2)
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	before, _ := m.inboxContent(60)
	m.cfg.Pills = "square"
	after, _ := m.inboxContent(60)
	if after == before || strings.Contains(after, "\ue0b6") {
		t.Fatalf("pill shape change not rendered: %q", after)
	}
}

func TestInboxLayoutCacheInvalidatesOnMergeState(t *testing.T) {
	m := scrollModel(t, 2)
	before, _ := m.inboxContent(60)
	m.prs[0].MergeState = "BEHIND"
	after, _ := m.inboxContent(60)
	if after == before || !strings.Contains(stripANSI(after), "behind") {
		t.Fatalf("merge state change not rendered: %q", after)
	}
}
