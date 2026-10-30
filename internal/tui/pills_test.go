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

func TestTFPillState(t *testing.T) {
	cases := []struct {
		name string
		a    model.AtlantisSummary
		want uitheme.PillState
	}{
		{"plan failed", model.AtlantisSummary{Plan: model.PlanFailed}, uitheme.PillFailed},
		{"policy failed", model.AtlantisSummary{Plan: model.PlanPassed, Policy: model.PolicyFailed}, uitheme.PillFailed},
		{"plan running", model.AtlantisSummary{Plan: model.PlanRunning}, uitheme.PillWait},
		{"policy running", model.AtlantisSummary{Plan: model.PlanPassed, Policy: model.PolicyRunning}, uitheme.PillWait},
		{"plan and policy passed", model.AtlantisSummary{Plan: model.PlanPassed, Policy: model.PolicyPassed}, uitheme.PillPassed},
		{"plan passed no policy", model.AtlantisSummary{Plan: model.PlanPassed}, uitheme.PillPassed},
		{"no plan", model.AtlantisSummary{}, uitheme.PillNone},
		{"apply failed ignored", model.AtlantisSummary{Plan: model.PlanPassed, Apply: model.ApplyFailed}, uitheme.PillPassed},
	}
	for _, tc := range cases {
		if got := tfPillState(tc.a); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
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
	// item 0: title, row 2, reviews row (selected); divider; item 1: title, row 2
	rows := map[int]string{0: stripANSI(lines[1]), 1: stripANSI(lines[len(lines)-1])}
	icons := map[int]string{0: "○", 1: "●"}
	col := -1
	for i, line := range rows {
		if !strings.HasSuffix(line, " CI  "+icons[i]) {
			t.Fatalf("item %d row 2 = %q", i, line)
		}
		if w := ansi.StringWidth(line); w != 60 {
			t.Fatalf("item %d row 2 width = %d", i, w)
		}
		c := ansi.StringWidth(line[:strings.LastIndex(line, " CI ")])
		if col >= 0 && c != col {
			t.Fatalf("CI column differs: %d vs %d", c, col)
		}
		col = c
	}
	if !strings.Contains(rows[0], " draft ") || !strings.Contains(rows[0], " behind ") {
		t.Fatalf("occasional pills missing: %q", rows[0])
	}
}

func TestInboxRow2ShowsTFOnlyInAtlantisRepo(t *testing.T) {
	pr := model.PullRequest{Number: 1, Title: "t", Author: "a", Checks: []model.Check{{Context: "atlantis/plan", State: "SUCCESS", StartedAt: time.Now()}}}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	m.cfg.Pills = "square"
	if row := stripANSI(inboxLines(m, 60)[1]); !strings.HasSuffix(row, " TF ") {
		t.Fatalf("row 2 lacks TF: %q", row)
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
	for _, want := range []string{" draft ", " auto ", " conflict ", " CI ", "#1"} {
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
	for _, want := range []string{" conflict ", " CI ", "#1"} {
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
