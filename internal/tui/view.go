package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/senilio/zappr/internal/model"
)

func (m Model) View() string {
	switch m.mode {
	case ModeDetail:
		return m.detailView()
	case ModeDiff:
		return m.diffView()
	case ModeConfirmApprove:
		return m.approveView()
	case ModeComment:
		return m.commentView()
	default:
		return m.inboxView()
	}
}

func (m Model) inboxView() string {
	var b strings.Builder
	fmt.Fprintf(&b, "zappr inbox %s", m.repo)
	if !m.lastFetch.IsZero() {
		fmt.Fprintf(&b, "  refreshed %s ago", shortDuration(time.Since(m.lastFetch)))
	}
	b.WriteString("\n\n")
	if m.loading {
		b.WriteString("loading...\n")
	}
	if m.error != "" {
		fmt.Fprintf(&b, "error: %s\n", m.error)
	}
	if m.message != "" {
		fmt.Fprintf(&b, "%s\n", m.message)
	}
	if len(m.prs) == 0 && !m.loading {
		b.WriteString("no open pull requests\n")
	}

	fmt.Fprintf(&b, "%-3s %-8s %-8s %-8s %s\n", "", "PR", "Risk", "CI", "Title")
	for i, pr := range m.prs {
		cursor := " "
		if i == m.selected {
			cursor = ">"
		}
		skip := ""
		if pr.Skipped {
			skip = " [skipped]"
		}
		fmt.Fprintf(&b, "%-3s #%-7d %-8s %-8s %s%s\n", cursor, pr.Number, riskLabel(pr.Triage.Risk), pr.CIStatus, pr.Title, skip)
	}
	b.WriteString("\nj/k navigate  enter inspect  a approve  c comment  s skip  r refresh  v diff  q quit\n")
	return b.String()
}

func (m Model) detailView() string {
	pr, ok := m.current()
	if !ok {
		return m.inboxView()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s #%d %s\n", pr.Repo, pr.Number, riskLabel(pr.Triage.Risk))
	fmt.Fprintf(&b, "%s\n", pr.Title)
	fmt.Fprintf(&b, "Author: %s  CI: %s  Files: %d  +%d -%d\n\n", pr.Author, pr.CIStatus, pr.ChangedFiles, pr.Additions, pr.Deletions)
	writeTriage(&b, pr.Triage)
	b.WriteString("\nFiles:\n")
	for _, file := range pr.Files {
		fmt.Fprintf(&b, "%-8s %s (+%d -%d)\n", file.Status, file.Path, file.Additions, file.Deletions)
	}
	b.WriteString("\na approve  c comment  s skip  r re-run analysis  v view diff  esc inbox  q quit\n")
	return b.String()
}

func (m Model) diffView() string {
	return m.diff + "\n\nesc/enter back  q quit\n"
}

func (m Model) approveView() string {
	pr, ok := m.current()
	if !ok {
		return m.inboxView()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Approve %s #%d?\n\n", pr.Repo, pr.Number)
	writeTriage(&b, pr.Triage)
	b.WriteString("\nPress y to approve via gh pr review --approve, n/esc to cancel.\n")
	return b.String()
}

func (m Model) commentView() string {
	pr, ok := m.current()
	if !ok {
		return m.inboxView()
	}
	return fmt.Sprintf("Comment on %s #%d\n\n%s\n\nenter submit  esc cancel\n", pr.Repo, pr.Number, m.comment)
}

func writeTriage(b *strings.Builder, triage model.Triage) {
	fmt.Fprintf(b, "Risk: %s  Confidence: %.2f\n", riskLabel(triage.Risk), triage.Confidence)
	fmt.Fprintf(b, "Summary: %s\n", triage.Summary)
	writeList(b, "Reasons", triage.Reasons)
	writeList(b, "Concerns", triage.Concerns)
	writeList(b, "High-risk files", triage.HighRiskFiles)
	writeList(b, "Review targets", triage.ReviewTargets)
	if triage.AI != nil {
		b.WriteString("AI notes, advisory only:\n")
		if triage.AI.Error != "" {
			fmt.Fprintf(b, "- %s\n", triage.AI.Error)
		} else {
			fmt.Fprintf(b, "- %s\n", triage.AI.Summary)
			writeList(b, "AI focus", triage.AI.SuggestedFocus)
		}
	}
}

func writeList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", title)
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func shortDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func timeNow() time.Time { return time.Now() }
