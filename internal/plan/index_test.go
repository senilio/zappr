package plan

import (
	"github.com/senilio/zappr/internal/model"
	"strings"
	"testing"
)

func outputBody(output string) string {
	return "Ran Plan for project: `example-app` dir: `infra/example-app` workspace: `default`\n\n<details><summary>Show Output</summary>\n\n```diff\n" + output + "\n```\n</details>\n"
}
func projectFor(t *testing.T, output string) Project {
	t.Helper()
	return Parse([]model.PRComment{planComment("1", outputBody(output), 0)}, true).Reports[0].Projects[0]
}

func TestIndexCreateUpdateDeleteReplace(t *testing.T) {
	p := projectFor(t, "  # example.one will be created\n  + resource {}\n  # example.two will be updated in-place\n  ~ resource {}\n  # example.three will be destroyed\n  - resource {}\n  # example.four must be replaced\n-/+ resource {}\nPlan: 2 to add, 1 to change, 2 to destroy.")
	if len(p.Resources) != 4 {
		t.Fatal(p.Resources)
	}
	for i, want := range []string{"create", "update", "delete", "replace"} {
		if p.Resources[i].Action != want {
			t.Fatal(p.Resources)
		}
	}
	if p.Outcome != OutcomeChanges {
		t.Fatal(p)
	}
}
func TestIndexIgnoresResourceHeadingsInsideValues(t *testing.T) {
	p := projectFor(t, "  # example.one will be created\n  + resource {\n    + content = <<-EOT\n  # fake.two will be destroyed\nPlan: 99 to add, 0 to change, 0 to destroy.\nEOT\n}\nPlan: 1 to add, 0 to change, 0 to destroy.")
	if len(p.Resources) != 1 || p.Totals == nil || p.Totals.Add != 1 {
		t.Fatal(p)
	}
}
func TestIndexResourceSpansCrossComments(t *testing.T) {
	p := Parse(fivePartComments(t), true).Reports[0].Projects[0]
	if len(p.Resources) != 1 || len(p.Resources[0].Sources) != 5 {
		t.Fatal(p.Resources)
	}
}

func TestIndexHeredocDiffLikeContentIsStillLiteral(t *testing.T) {
	p := projectFor(t, "# example.one will be created\n+ resource {\n + text = <<-EOT\n    + EOT\n# fake.two will be destroyed\nPlan: 99 to add, 0 to change, 0 to destroy.\nEOT\n}\nPlan: 1 to add, 0 to change, 0 to destroy.")
	if len(p.Resources) != 1 || p.Totals == nil || p.Totals.Add != 1 {
		t.Fatal("heredoc text escaped literal boundary")
	}
}
func TestOutcomeRepeatedFooterCountsOnce(t *testing.T) {
	r := Parse([]model.PRComment{planComment("1", fixture(t, "single-project"), 0)}, true).Reports[0]
	if r.Outcome != OutcomeChanges || r.Projects[0].Totals == nil || r.Projects[0].Totals.Add != 1 {
		t.Fatal(r)
	}
}
func TestOutcomeConflictingTotalsIsUnknown(t *testing.T) {
	b := fixture(t, "single-project") + "\nPlan: 2 to add, 0 to change, 0 to destroy.\n"
	p := Parse([]model.PRComment{planComment("1", b, 0)}, true).Reports[0].Projects[0]
	if p.Outcome != OutcomeUnknown || len(p.Issues) == 0 {
		t.Fatal(p)
	}
}
func TestOutcomeZeroResourcesIsNotNoChanges(t *testing.T) {
	if p := projectFor(t, "Plan: 0 to add, 0 to change, 0 to destroy."); p.Outcome != OutcomeUnknown {
		t.Fatal(p)
	}
}
func TestOutcomeExplicitNoChangesRequiresCompleteEvidence(t *testing.T) {
	body := outputBody("No changes. Your infrastructure matches the configuration.")
	for _, complete := range []bool{true, false} {
		r := Parse([]model.PRComment{planComment("1", body, 0)}, complete).Reports[0]
		if (r.Outcome == OutcomeNoChanges) != complete {
			t.Fatal(r)
		}
	}
}
func TestOutcomeReadImportOutputAndFailure(t *testing.T) {
	for _, tc := range []struct {
		out  string
		want Outcome
	}{
		{"# data.example.one will be read during apply\n <= data {}\nPlan: 0 to add, 0 to change, 0 to destroy.", OutcomeUnknown},
		{"# example.one will be imported\n resource {}\nPlan: 0 to add, 0 to change, 0 to destroy.", OutcomeChanges},
		{"Changes to Outputs:\n  + address = \"new\"\nPlan: 0 to add, 0 to change, 0 to destroy.", OutcomeChanges},
		{"Planning failed. OpenTofu encountered an error.", OutcomeFailed},
		{"# example.one will be transformed\nNo changes. Your infrastructure matches the configuration.", OutcomeUnknown},
	} {
		if p := projectFor(t, tc.out); p.Outcome != tc.want {
			t.Fatalf("%s: %s != %s", tc.out, p.Outcome, tc.want)
		}
	}
}
func TestAggregateOnlyNoChangesHasNoIndividualOutput(t *testing.T) {
	r := Parse([]model.PRComment{planComment("1", fixture(t, "aggregate-no-changes"), 0)}, true).Reports[0]
	if r.Aggregate == nil || r.Outcome != OutcomeNoChanges || len(r.Projects) != 3 {
		t.Fatal(r)
	}
	for _, p := range r.Projects {
		if p.DetailAvailable || p.Outcome != OutcomeNoChanges || p.Evidence != "aggregate" {
			t.Fatal(p)
		}
	}
	bad := strings.Replace(fixture(t, "aggregate-no-changes"), "3 projects, 0", "4 projects, 0", 1)
	r = Parse([]model.PRComment{planComment("1", bad, 0)}, true).Reports[0]
	if r.Outcome != OutcomeUnknown || len(r.Issues) == 0 {
		t.Fatal(r)
	}
}
func TestMixedAggregateDoesNotGuessProjectOutcomes(t *testing.T) {
	b := strings.Replace(fixture(t, "aggregate-no-changes"), "0 with changes, 3 with no changes", "1 with changes, 2 with no changes", 1)
	r := Parse([]model.PRComment{planComment("1", b, 0)}, true).Reports[0]
	if r.Outcome != OutcomeChanges {
		t.Fatal(r)
	}
	for _, p := range r.Projects {
		if p.Outcome != OutcomeUnknown {
			t.Fatal(p)
		}
	}
}
func TestAggregateBoundedProjectSections(t *testing.T) {
	b := "Ran Plan for 2 projects:\n1. project: `example-app` dir: `infra/example-app` workspace: `default`\n1. project: `example-data` dir: `infra/example-data` workspace: `default`\n\n"
	b += outputBody("# example.one will be created\n+ resource {\n + text = <<-EOT\n1. project: `fake` dir: `fake` workspace: `default`\nEOT\n}\nPlan: 1 to add, 0 to change, 0 to destroy.")
	b += strings.ReplaceAll(outputBody("No changes. Your infrastructure matches the configuration."), "example-app", "example-data")
	b += "\n### Plan Summary\n2 projects, 1 with changes, 1 with no changes, 0 failed\n"
	r := Parse([]model.PRComment{planComment("1", b, 0)}, true).Reports[0]
	if len(r.Projects) != 2 || !r.Projects[0].DetailAvailable || r.Projects[0].Outcome != OutcomeChanges || r.Projects[1].Outcome != OutcomeNoChanges {
		t.Fatal(r)
	}
	for _, p := range r.Projects {
		for i, s := range p.LineSources {
			lines := strings.Split(b, "\n")
			if lines[s.Line] != strings.Split(p.Text, "\n")[i] {
				t.Fatal("source offset")
			}
		}
	}
}
