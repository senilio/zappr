package plan

import (
	"fmt"
	"strings"
)

const minimumFoldLines = 12

func indexFolds(p *Project, complete bool) {
	if !complete || len(p.Issues) > 0 || !p.supported {
		return
	}
	lines := strings.Split(p.Text, "\n")
	for _, resource := range p.Resources {
		var state scanState
		start := -1
		label := ""
		changing := false
		for i := resource.Start; i < resource.End; i++ {
			previous := state.heredoc
			payload := state.payload(lines[i])
			if previous != "" && state.heredoc == "" && start >= 0 {
				if changing && i-start >= minimumFoldLines {
					p.Folds = append(p.Folds, Fold{ID: fmt.Sprintf("%s/value:%s:%d", resource.ID, label, len(p.Folds)), ResourceID: resource.ID, Label: label, Start: start, End: i})
				}
				start = -1
			}
			if !payload && previous == "" && state.heredoc != "" {
				start = i + 1
				label = strings.TrimSpace(strings.SplitN(diffText(lines[i]), "=", 2)[0])
				trim := strings.TrimSpace(lines[i])
				changing = resource.Action == "create" || resource.Action == "delete" || strings.HasPrefix(trim, "+") || strings.HasPrefix(trim, "-") || strings.HasPrefix(trim, "~")
			}
		}
	}
}

func RenderResource(project Project, resource Resource, expanded map[string]bool, revealLine int) Rendered {
	lines := strings.Split(project.Text, "\n")
	var result Rendered
	var output []string
	for i := resource.Start; i < resource.End && i < len(lines); i++ {
		folded := false
		for _, f := range project.Folds {
			if f.ResourceID == resource.ID && f.Start == i && !expanded[f.ID] && (revealLine < f.Start || revealLine >= f.End) {
				output = append(output, fmt.Sprintf("    … %s: changed value, %d lines folded …", f.Label, f.End-f.Start))
				result.SourceLines = append(result.SourceLines, i)
				i = f.End - 1
				folded = true
				break
			}
		}
		if !folded {
			output = append(output, lines[i])
			result.SourceLines = append(result.SourceLines, i)
		}
	}
	result.Text = strings.Join(output, "\n")
	return result
}
