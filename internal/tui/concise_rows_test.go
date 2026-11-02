package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/plan"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func TestCommentRowsAreMetadataOnly(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		m := nativeCommentModel(t)
		m.cfg.Wrap.Summary = wrap
		m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
		view, spans := m.commentSummaryLayout(m.prs[0], 100)
		lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
		if len(lines) != 3 || len(spans) != 3 {
			t.Fatalf("wrap=%v: want one row per comment: %q", wrap, view)
		}
		for i, line := range lines {
			if strings.HasPrefix(stripANSI(line), " ") || strings.HasPrefix(stripANSI(line), ">") || spans[i] != (rowSpan{i, i + 1}) {
				t.Fatalf("indent/marker or incorrect span: %q %+v", line, spans)
			}
		}
		if strings.Contains(stripANSI(view), "Plan result") || !strings.Contains(lines[0], m.theme.Styles.TitleStrong.Render(stripANSI(lines[0]))) || !strings.Contains(lines[1], m.theme.Styles.Muted.Render(stripANSI(lines[1]))) {
			t.Fatalf("preview or incorrect selection styles: %q", view)
		}
		view, _ = m.commentSummaryLayout(m.prs[0], 18)
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > 18 {
				t.Fatalf("overflow: %q", line)
			}
		}
	}
}

func TestAtlantisRowFocusesOnPlanResult(t *testing.T) {
	for _, tc := range []struct{ check, want string }{{"SUCCESS", "plan ok"}, {"FAILURE", "plan failed"}, {"PENDING", "planning"}} {
		m := planModel(t)
		m.prs[0].Checks = atlantisChecks(tc.check, time.Now())
		row := stripANSI(m.atlantisRow(m.prs[0], 160))
		if !strings.Contains(row, tc.want) || !strings.Contains(row, "changes") {
			t.Fatalf("%s: row = %q", tc.check, row)
		}
		for _, unwanted := range []string{"commit unverified", "updated", "project", "aggregate", "limited evidence"} {
			if strings.Contains(row, unwanted) {
				t.Fatalf("unwanted %q in row %q", unwanted, row)
			}
		}
	}
	// A complete report can establish the result when no check is present.
	m := planModel(t)
	if row := stripANSI(m.atlantisRow(m.prs[0], 80)); !strings.Contains(row, "plan ok") {
		t.Fatal(row)
	}
	m.planState.doc.Reports[0].SourceComplete = false
	m.planState.doc.Reports[0].Outcome = plan.OutcomeUnknown
	row := stripANSI(m.atlantisRow(m.prs[0], 80))
	if !strings.Contains(row, "incomplete") || strings.Contains(row, "plan ok") {
		t.Fatalf("incomplete report must not claim success: %q", row)
	}
}

func TestPlanContextOmitsUnverifiedCommit(t *testing.T) {
	m := planModel(t)
	m.summaryMode = SummaryModePlan
	if got := m.currentDiffContent(); strings.Contains(got, "commit unverified") {
		t.Fatal(got)
	}
}
