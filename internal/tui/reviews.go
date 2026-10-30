package tui

import (
	"fmt"
	"strings"

	"github.com/senilio/zappr/internal/model"
)

const (
	expandedIndent     = 2
	expandedLabelWidth = 9
)

// expandedRow renders one of the selected PR's extra rows: an indented,
// muted label column followed by content, never wrapped.
func (m Model) expandedRow(label, content string, innerW int) string {
	head := strings.Repeat(" ", expandedIndent) + m.theme.Styles.Muted.Render(fmt.Sprintf("%-*s", expandedLabelWidth, label))
	return truncatePaneLineWidth(head+content, innerW)
}

func (m Model) reviewsRow(pr model.PullRequest, innerW int) string {
	s := m.theme.Styles
	if !pr.ReviewersLoaded {
		return m.expandedRow("reviews", s.Muted.Render("loading…"), innerW)
	}
	if len(pr.Reviewers) == 0 {
		return m.expandedRow("reviews", s.Muted.Render("none"), innerW)
	}
	names := make([]string, 0, len(pr.Reviewers))
	for _, r := range pr.Reviewers {
		style := s.Muted
		suffix := ""
		if r.Bot {
			suffix = " (bot)"
		} else {
			switch r.State {
			case "APPROVED":
				style = s.SuccessCI
				suffix = "✓"
			case "CHANGES_REQUESTED":
				style = s.Error
				suffix = "✗"
			case "REQUESTED":
				style = s.Wait
				suffix = "●"
			case "COMMENTED":
				suffix = " (commented)"
			case "DISMISSED":
				suffix = " (dismissed)"
			}
		}
		name := r.Login
		if m.theme.NoColor {
			name += suffix
		}
		names = append(names, style.Render(name))
	}
	return m.expandedRow("reviews", strings.Join(names, "  "), innerW)
}
