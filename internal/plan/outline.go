package plan

import "slices"

func sortedProjects(r Report) []Project {
	p := slices.Clone(r.Projects)
	priority := func(o Outcome) int {
		switch o {
		case OutcomeChanges:
			return 0
		case OutcomeFailed:
			return 1
		case OutcomeUnknown:
			return 2
		default:
			return 3
		}
	}
	slices.SortStableFunc(p, func(a, b Project) int { return priority(a.Outcome) - priority(b.Outcome) })
	return p
}

func Outline(doc Document, reportID string, expanded map[string]bool) []Node {
	var r *Report
	for i := range doc.Reports {
		if doc.Reports[i].ID == reportID {
			r = &doc.Reports[i]
			break
		}
	}
	if r == nil {
		return nil
	}
	nodes := []Node{{ID: r.ID + "/summary", Kind: NodeSummary, ReportID: r.ID}}
	for _, p := range sortedProjects(*r) {
		nodes = append(nodes, Node{ID: p.ID, Kind: NodeProject, ReportID: r.ID, ProjectID: p.ID, Depth: 1})
		if expanded[p.ID] {
			for _, resource := range p.Resources {
				nodes = append(nodes, Node{ID: resource.ID, Kind: NodeResource, ReportID: r.ID, ProjectID: p.ID, ResourceID: resource.ID, Depth: 2})
			}
		}
	}
	for _, s := range r.Sources {
		nodes = append(nodes, Node{ID: r.ID + "/source:" + s.CommentKey, Kind: NodeSource, ReportID: r.ID, CommentKey: s.CommentKey, Depth: 1})
	}
	for _, other := range doc.Reports {
		if other.ID != r.ID {
			nodes = append(nodes, Node{ID: "report:" + other.ID, Kind: NodeReport, ReportID: other.ID, Depth: 1})
		}
	}
	return nodes
}
