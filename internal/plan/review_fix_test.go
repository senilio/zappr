package plan

import (
	"strings"
	"testing"

	"github.com/senilio/zappr/internal/model"
)

func TestInterleavedRootsNeverJoinAContinuation(t *testing.T) {
	rootB := strings.ReplaceAll(fixture(t, "split-root"), "example-app", "example-b")
	comments := []model.PRComment{
		planComment("1", fixture(t, "split-root"), 0),
		planComment("2", rootB, 0),
		planComment("3", fixture(t, "split-terminal"), 0),
	}
	doc := Parse(comments, true)
	for _, r := range doc.Reports {
		if r.SourceComplete || r.Outcome != OutcomeUnknown {
			t.Fatalf("report %s: complete=%v outcome=%v from an ambiguous chain", r.ID, r.SourceComplete, r.Outcome)
		}
		for _, p := range r.Projects {
			if p.Name == "example-b" && strings.Contains(p.Text, "Last line of the new value.") {
				t.Fatal("project B absorbed project A's continuation")
			}
		}
	}
}

func TestRecordedInconsistencyPreventsConfidentOutcome(t *testing.T) {
	r := Report{SourceComplete: true, Issues: []string{"Aggregate result not provided"}, Projects: []Project{{Outcome: OutcomeNoChanges}}}
	indexReport(&r)
	if r.Outcome != OutcomeUnknown {
		t.Fatalf("outcome = %v despite a recorded inconsistency", r.Outcome)
	}
}

func TestAggregateRootRequestingContinuationIsIncomplete(t *testing.T) {
	body := fixture(t, "aggregate-no-changes") + "\n<br>\n\n" + continuationWarning + "\n"
	doc := Parse([]model.PRComment{planComment("1", body, 0)}, true)
	if len(doc.Reports) != 1 {
		t.Fatalf("reports = %d", len(doc.Reports))
	}
	if r := doc.Reports[0]; r.SourceComplete || r.Outcome != OutcomeUnknown {
		t.Fatalf("aggregate awaiting continuation: complete=%v outcome=%v", r.SourceComplete, r.Outcome)
	}
}
