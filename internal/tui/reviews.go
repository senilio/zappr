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
	groups := make(map[string][]string)
	bots := 0
	for _, r := range pr.Reviewers {
		if r.Bot {
			bots++
			continue
		}
		style := s.Muted
		group := "reviewed"
		switch r.State {
		case "APPROVED":
			group, style = "approved", s.SuccessCI
		case "CHANGES_REQUESTED":
			group, style = "changes", s.Error
		case "REQUESTED":
			group, style = "waiting", s.Wait
		case "COMMENTED":
			group = "commented"
		case "DISMISSED":
			group = "dismissed"
		}
		groups[group] = append(groups[group], style.Render(r.Login))
	}
	parts := make([]string, 0, len(groups)+1)
	for _, group := range []string{"approved", "changes", "waiting", "commented", "dismissed", "reviewed"} {
		if names := groups[group]; len(names) > 0 {
			parts = append(parts, s.Muted.Render(group+" ")+strings.Join(names, s.Muted.Render(", ")))
		}
	}
	if bots > 0 {
		parts = append(parts, s.Muted.Render(fmt.Sprintf("bots %d", bots)))
	}
	return m.expandedRow("reviews", strings.Join(parts, s.Muted.Render(" · ")), innerW)
}
