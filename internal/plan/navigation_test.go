package plan

import (
	"github.com/senilio/zappr/internal/model"
	"strings"
	"testing"
)

func longValueReport(t *testing.T) Report {
	t.Helper()
	body := ""
	for i := 0; i < 14; i++ {
		body += "        row\n"
	}
	body = strings.Replace(body, "row", "NEEDLE a.*[b] Ångström", 1)
	out := "# example.one will be created\n+ resource {\n  + content = <<-EOT\n" + body + "EOT\n}\nPlan: 1 to add, 0 to change, 0 to destroy."
	return Parse([]model.PRComment{planComment("1", outputBody(out), 0)}, true).Reports[0]
}
func TestRenderResourceFoldsOnlyLongKnownValues(t *testing.T) {
	r := longValueReport(t)
	p := r.Projects[0]
	resource := p.Resources[0]
	if len(p.Folds) != 1 || p.Folds[0].End-p.Folds[0].Start != 14 {
		t.Fatal(p.Folds)
	}
	view := RenderResource(p, resource, nil, -1)
	if strings.Contains(view.Text, "NEEDLE") || !strings.Contains(view.Text, "14 lines") || !strings.Contains(view.Text, "content = <<-EOT") || !strings.Contains(view.Text, "EOT") {
		t.Fatal(view)
	}
	full := RenderResource(p, resource, map[string]bool{p.Folds[0].ID: true}, -1)
	want := strings.Join(strings.Split(p.Text, "\n")[resource.Start:resource.End], "\n")
	if full.Text != want {
		t.Fatal("expanded text changed")
	}
	partial := Parse([]model.PRComment{planComment("1", outputBody(strings.TrimSuffix(p.Text, "EOT")), 0)}, false).Reports[0]
	if len(partial.Projects[0].Folds) != 0 {
		t.Fatal("incomplete report folded")
	}
}
func TestOutlineKeepsNoChangeAndMissingDetailProjects(t *testing.T) {
	r := longValueReport(t)
	p := newProject(r.ID, "unchanged", "infra/other", "default")
	p.Outcome = OutcomeNoChanges
	r.Projects = append([]Project{p}, r.Projects...)
	d := Document{Reports: []Report{r, {ID: "older"}}}
	nodes := Outline(d, r.ID, map[string]bool{r.Projects[1].ID: true})
	if nodes[0].Kind != NodeSummary || nodes[1].ProjectID != r.Projects[1].ID {
		t.Fatal(nodes)
	}
	counts := map[NodeKind]int{}
	for _, n := range nodes {
		counts[n.Kind]++
	}
	if counts[NodeProject] != 2 || counts[NodeResource] != 1 || counts[NodeSource] != 1 || counts[NodeReport] != 1 {
		t.Fatal(counts)
	}
}
func TestRenderedSourceLinesSurviveFolding(t *testing.T) {
	r := longValueReport(t)
	p := r.Projects[0]
	v := RenderResource(p, p.Resources[0], nil, -1)
	if len(v.SourceLines) != len(strings.Split(v.Text, "\n")) {
		t.Fatal(v)
	}
	for i, line := range strings.Split(v.Text, "\n") {
		if strings.Contains(line, "14 lines") && v.SourceLines[i] != p.Folds[0].Start {
			t.Fatal(v)
		}
	}
}
