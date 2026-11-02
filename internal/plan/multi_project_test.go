package plan

import (
	"strings"
	"testing"

	"github.com/senilio/zappr/internal/model"
)

func TestMultiProjectMarkdownSections(t *testing.T) {
	for _, numbered := range []bool{true, false} {
		body := fixture(t, "multi-project")
		if !numbered {
			body = strings.ReplaceAll(strings.ReplaceAll(body, "### 1. project:", "### project:"), "### 4. project:", "### project:")
		}
		r := Parse([]model.PRComment{planComment("1", body, 0)}, true).Reports[0]
		if r.Outcome != OutcomeChanges || len(r.Issues) != 0 || r.Coverage != CoverageProjects {
			t.Fatalf("numbered=%v: report = %+v", numbered, r)
		}
		for i, p := range r.Projects {
			if i == 0 || i == 3 {
				if p.Outcome != OutcomeChanges || !p.DetailAvailable || len(p.Resources) != 1 || p.Totals == nil {
					t.Fatalf("numbered=%v: missing project output: %+v", numbered, p)
				}
				for line, source := range p.LineSources {
					if strings.Split(body, "\n")[source.Line] != strings.Split(p.Text, "\n")[line] || source.CommentKey != "comment:1" {
						t.Fatal("incorrect source mapping", source)
					}
				}
			} else if p.Outcome != OutcomeNoChanges || p.DetailAvailable || p.Evidence != "aggregate" {
				t.Fatalf("numbered=%v: unchanged project = %+v", numbered, p)
			}
		}
	}
}

func TestMultiProjectInferenceRequiresCompleteConsistentEvidence(t *testing.T) {
	for _, scenario := range []string{"incomplete", "missing section", "wrong totals", "duplicate section"} {
		body := fixture(t, "multi-project")
		switch scenario {
		case "missing section":
			start := strings.Index(body, "### 4.")
			end := strings.Index(body, "### Plan Summary")
			body = body[:start] + body[end:]
		case "wrong totals":
			body = strings.Replace(body, "2 with changes, 2 with no changes", "1 with changes, 3 with no changes", 1)
		case "duplicate section":
			body = strings.Replace(body, "### Plan Summary", body[strings.Index(body, "### 1."):strings.Index(body, "### 4.")]+"### Plan Summary", 1)
		}
		r := Parse([]model.PRComment{planComment("1", body, 0)}, scenario != "incomplete").Reports[0]
		for _, p := range r.Projects {
			if !p.DetailAvailable && p.Outcome != OutcomeUnknown {
				t.Fatalf("%s: inferred outcome without sufficient evidence: %+v", scenario, p)
			}
		}
	}
}
