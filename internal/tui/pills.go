package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func (m Model) pillShape() uitheme.PillShape { return uitheme.ParsePillShape(m.cfg.Pills) }

func ciPillState(status string) uitheme.PillState {
	switch strings.ToLower(status) {
	case "passing", "success":
		return uitheme.PillPassed
	case "pending", "queued", "in_progress":
		return uitheme.PillWait
	case "failing", "failed", "failure", "error":
		return uitheme.PillFailed
	default:
		return uitheme.PillNone
	}
}

// tfPillState summarizes plan and policy; apply state is shown only on the
// selected PR's atlantis row.
func tfPillState(a model.AtlantisSummary) uitheme.PillState {
	switch {
	case a.Plan == model.PlanFailed || a.Policy == model.PolicyFailed:
		return uitheme.PillFailed
	case a.Plan == model.PlanRunning || a.Policy == model.PolicyRunning:
		return uitheme.PillWait
	case a.Plan == model.PlanPassed:
		return uitheme.PillPassed
	default:
		return uitheme.PillNone
	}
}

type rowPill struct {
	label string
	state uitheme.PillState
}

// occasionalPills lists the pills shown only when their condition holds,
// in display order. They are dropped first when the row is too narrow.
func occasionalPills(pr model.PullRequest) []rowPill {
	var pills []rowPill
	if pr.Draft {
		pills = append(pills, rowPill{"draft", uitheme.PillNone})
	}
	if pr.AutoMerge {
		pills = append(pills, rowPill{"auto", uitheme.PillNone})
	}
	switch pr.MergeState {
	case "BEHIND":
		pills = append(pills, rowPill{"behind", uitheme.PillWait})
	case "DIRTY":
		pills = append(pills, rowPill{"conflict", uitheme.PillFailed})
	case "BLOCKED":
		if ciPillState(pr.CIStatus) == uitheme.PillPassed && strings.ToUpper(pr.ReviewDecision) == "APPROVED" {
			pills = append(pills, rowPill{"blocked", uitheme.PillWait})
		}
	}
	return pills
}

// inboxMetaRow renders row 2: "#number · author" on the left and the
// status pills right-aligned to innerW.
func (m Model) inboxMetaRow(pr model.PullRequest, selected, atlantisRepo bool, innerW int) string {
	shape := m.pillShape()
	fixed := []string{m.theme.Pill("CI", ciPillState(pr.CIStatus), shape), m.theme.ReviewIcon(pr.ReviewDecision, pr.Draft, shape)}
	if atlantisRepo {
		fixed = append(fixed, m.theme.Pill("TF", tfPillState(model.DeriveAtlantis(pr.Checks)), shape))
	}
	var occasional []string
	for _, p := range occasionalPills(pr) {
		occasional = append(occasional, m.theme.Pill(p.label, p.state, shape))
	}
	number := fmt.Sprintf("#%d", pr.Number)
	minLeftW := ansi.StringWidth(number)
	right := strings.Join(append(append([]string{}, occasional...), fixed...), " ")
	for len(occasional) > 0 && minLeftW+1+ansi.StringWidth(right) > innerW {
		occasional = occasional[1:]
		right = strings.Join(append(append([]string{}, occasional...), fixed...), " ")
	}
	rightW := ansi.StringWidth(right)
	if rightW > innerW {
		return truncatePaneLineWidth(right, innerW)
	}
	leftStyle := m.theme.Styles.Muted
	if selected {
		leftStyle = m.theme.Styles.TitleStrong
	}
	left := ""
	if leftW := innerW - rightW - 1; leftW >= minLeftW {
		left = leftStyle.Render(truncateANSIWidth(number+" · "+pr.Author, leftW))
	}
	pad := strings.Repeat(" ", max(0, innerW-ansi.StringWidth(left)-rightW))
	return truncatePaneLineWidth(left+pad+right, innerW)
}
