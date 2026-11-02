package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/senilio/zappr/internal/plan"
)

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func planSourceCount(report plan.Report) string {
	return plural(len(report.Sources), "source comment", "source comments")
}

// planContext describes report evidence quality in the pane content.
func planContext(report plan.Report) string {
	complete := "source complete"
	if !report.SourceComplete {
		complete = "source incomplete"
	}
	parts := []string{complete}
	if report.Coverage == plan.CoverageAggregate {
		parts = append(parts, "aggregate only")
	}
	return strings.Join(append(parts, planSourceCount(report)), " · ")
}

func (m Model) planSourceLabel(key string) string {
	if pr, ok := m.current(); ok {
		for _, c := range pr.Comments {
			if c.Key() == key {
				label := "Source · " + c.Author
				if stamp := summaryTimestamp(c.CreatedAt, time.Now()); stamp != "" {
					label += " · " + stamp
				}
				return label
			}
		}
	}
	return "Source comment"
}

func (m Model) planNodeLabel(node plan.Node) string {
	report, _ := m.selectedPlanReport()
	switch node.Kind {
	case plan.NodeSummary:
		return "Overall Plan Summary"
	case plan.NodeProject:
		for _, p := range report.Projects {
			if p.ID == node.ProjectID {
				return p.Name + " · " + planOutcome(p.Outcome)
			}
		}
	case plan.NodeResource:
		for _, p := range report.Projects {
			for _, r := range p.Resources {
				if r.ID == node.ResourceID {
					return r.Action + " " + r.Address
				}
			}
		}
	case plan.NodeSource:
		return m.planSourceLabel(node.CommentKey)
	case plan.NodeReport:
		if m.planState.doc != nil {
			for _, other := range m.planState.doc.Reports {
				if other.ID == node.ReportID {
					return fmt.Sprintf("Report %s · %s", other.LatestActivity.Format("Jan 02 15:04"), planOutcome(other.Outcome))
				}
			}
		}
	}
	return "unavailable"
}

func (m Model) planSummaryLayout(width int) (string, []rowSpan) {
	var blocks []string
	var spans []rowSpan
	row := 0
	for _, node := range m.planNodes() {
		marker := "  "
		if node.ID == m.planState.nodeID {
			marker = "> "
		}
		line := marker + strings.Repeat("  ", node.Depth) + commentPlainText(m.planNodeLabel(node))
		if node.ID == m.planState.nodeID {
			line = m.theme.Styles.TitleStrong.Render(line)
		}
		if m.cfg.Wrap.Summary {
			line = wrapStructured(line, max(1, width))
		} else {
			line = truncatePaneLineWidth(line, width)
		}
		end := row + len(strings.Split(line, "\n"))
		spans = append(spans, rowSpan{row, end})
		row = end
		blocks = append(blocks, line)
	}
	if len(blocks) == 0 {
		return "selected report is unavailable\n", nil
	}
	return strings.Join(blocks, "\n") + "\n", spans
}

func planTotals(totals *plan.Totals) string {
	if totals == nil {
		return "resource counts unavailable"
	}
	return fmt.Sprintf("%d to add, %d to change, %d to destroy", totals.Add, totals.Change, totals.Destroy)
}

func planOverall(report plan.Report) string {
	var b strings.Builder
	b.WriteString("Overall Plan Summary\n")
	if aggregate := report.Aggregate; aggregate != nil {
		fmt.Fprintf(&b, "Reported by Atlantis: %s, %d with changes, %d with no changes, %d failed\n", plural(aggregate.Projects, "project", "projects"), aggregate.Changed, aggregate.NoChanges, aggregate.Failed)
	} else {
		changed, unchanged, failed, unknown := 0, 0, 0, 0
		for _, p := range report.Projects {
			switch p.Outcome {
			case plan.OutcomeChanges:
				changed++
			case plan.OutcomeNoChanges:
				unchanged++
			case plan.OutcomeFailed:
				failed++
			default:
				unknown++
			}
		}
		fmt.Fprintf(&b, "Counted from project output: %s, %d with changes, %d with no changes, %d failed, %d unknown\n", plural(len(report.Projects), "project", "projects"), changed, unchanged, failed, unknown)
	}
	for _, p := range report.Projects {
		fmt.Fprintf(&b, "\n%s · %s\n  %s\n", p.Name, planOutcome(p.Outcome), planTotals(p.Totals))
	}
	return b.String()
}

func (m Model) planDetail() string {
	lines := strings.Split(m.planRenderedDetail().Text, "\n")
	s := m.theme.Styles
	for i, line := range lines {
		switch trimmed := strings.TrimLeft(line, " "); {
		case strings.HasPrefix(trimmed, "+ "), trimmed == "+":
			lines[i] = s.Add.Render(line)
		case strings.HasPrefix(trimmed, "- "), trimmed == "-":
			lines[i] = s.Delete.Render(line)
		case strings.HasPrefix(trimmed, "~ "), strings.HasPrefix(trimmed, "-/+ "), strings.HasPrefix(trimmed, "+/- "), strings.HasPrefix(trimmed, "<= "):
			lines[i] = s.Pending.Render(line)
		case strings.HasPrefix(trimmed, "# "), strings.HasPrefix(trimmed, "Warning: "):
			lines[i] = s.Muted.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) planRenderedDetail() plan.Rendered {
	report, ok := m.selectedPlanReport()
	if !ok {
		return plan.Rendered{Text: "Selected plan report is unavailable. Choose another report or view."}
	}
	var b strings.Builder
	b.WriteString(planContext(report) + "\n\n")
	bodyStart := -1
	var bodyRows []int
	writeBody := func(text string, rows []int) {
		bodyStart, bodyRows = strings.Count(b.String(), "\n"), rows
		b.WriteString(text)
	}
	for _, issue := range report.Issues {
		b.WriteString("Warning: " + issue + "\n")
	}
	if len(report.Issues) > 0 {
		b.WriteByte('\n')
	}
	node, _ := m.selectedPlanNode()
	switch node.Kind {
	case plan.NodeReport:
		if m.planState.doc != nil {
			for _, other := range m.planState.doc.Reports {
				if other.ID == node.ReportID {
					b.WriteString("Report preview\n" + planContext(other) + "\n" + planOverall(other))
				}
			}
		}
	case plan.NodeSource:
		if pr, found := m.current(); found {
			for _, c := range pr.Comments {
				if c.Key() == node.CommentKey {
					fmt.Fprintf(&b, "%s\n%s\n\n%s", m.planSourceLabel(c.Key()), readerInput(c.URL), m.commentDetail(c))
				}
			}
		}
	case plan.NodeProject, plan.NodeResource:
		for _, p := range report.Projects {
			if p.ID != node.ProjectID {
				continue
			}
			fmt.Fprintf(&b, "%s · %s\nDirectory: %s\nWorkspace: %s\n%s\n", p.Name, planOutcome(p.Outcome), p.Dir, p.Workspace, planTotals(p.Totals))
			for _, issue := range p.Issues {
				b.WriteString("Warning: " + issue + "\n")
			}
			if node.Kind == plan.NodeResource {
				for i, r := range p.Resources {
					if r.ID == node.ResourceID {
						fmt.Fprintf(&b, "\nResource %d/%d · %s · %s\n\n", i+1, len(p.Resources), r.Action, r.Address)
						expanded := make(map[string]bool, len(p.Folds))
						for _, fold := range p.Folds {
							expanded[fold.ID] = true
						}
						rendered := plan.RenderResource(p, r, expanded, -1)
						writeBody(rendered.Text, rendered.SourceLines)
					}
				}
			} else if !p.DetailAvailable {
				b.WriteString("\nindividual project output is unavailable; outcome is reported by the aggregate.\n")
			} else if len(p.Resources) == 0 {
				b.WriteString("\nOriginal project output\n")
				rows := make([]int, len(strings.Split(p.Text, "\n")))
				for i := range rows {
					rows[i] = i
				}
				writeBody(p.Text, rows)
			} else {
				fmt.Fprintf(&b, "\n%d indexed resources:\n", len(p.Resources))
				for _, r := range p.Resources {
					fmt.Fprintf(&b, "%s %s\n", r.Action, r.Address)
				}
			}
		}
	default:
		b.WriteString(planOverall(report))
	}
	text := commentPlainText(strings.TrimRight(b.String(), "\n"))
	rows := make([]int, len(strings.Split(text, "\n")))
	for i := range rows {
		rows[i] = -1
	}
	if bodyStart >= 0 && bodyStart < len(rows) {
		copy(rows[bodyStart:], bodyRows)
	}
	return plan.Rendered{Text: text, SourceLines: rows}
}
