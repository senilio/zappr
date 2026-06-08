package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/senilio/zappr/internal/model"
)

const minWideWidth = 110
const minMediumWidth = 76

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
	case ModeHelp:
		return m.helpView()
	default:
		return m.inboxView()
	}
}

func (m Model) inboxView() string {
	if m.width > 0 {
		return m.layoutView()
	}

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

	fmt.Fprintf(&b, "%-3s %-8s %-12s %-8s %s\n", "", "PR", "Risk", "CI", "Title")
	for i, pr := range m.prs {
		cursor := " "
		if i == m.selected {
			cursor = ">"
		}
		skip := ""
		if pr.Skipped {
			skip = " [skipped]"
		}
		fmt.Fprintf(&b, "%-3s #%-7d %-12s %-8s %s%s\n", cursor, pr.Number, riskLabel(pr.Triage.Risk), pr.CIStatus, pr.Title, skip)
	}
	b.WriteString("\nj/k navigate  enter inspect  a approve, y confirm  c comment  s skip  r refresh  v diff  ? help  q quit\n")
	return b.String()
}

func (m Model) detailView() string {
	if m.width > 0 {
		return m.layoutView()
	}

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
	b.WriteString("\na approve, y confirm  c comment  s skip  r re-run analysis  v view diff  esc inbox  ? help  q quit\n")
	return b.String()
}

func (m Model) diffView() string {
	if m.width > 0 {
		return m.layoutView()
	}
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

func (m Model) helpView() string {
	if m.width > 0 {
		return m.layoutView()
	}
	return helpText() + "\nesc back  q quit\n"
}

func helpText() string {
	return `zappr help

Risk legend (deterministic, authoritative):
  SAFE  ok   safe for quick approval
  REVIEW !   human inspection recommended
  BLOCK  x   quick approval refused

AI triage is advisory only and never changes risk.

Keys:
  j/k        move / scroll
  tab        cycle pane focus
  o          cycle sort
  enter      inspect PR
  v          load/view diff
  V          open full diff pager
  n/p        next/prev file in diff
  s          skip PR (inbox) / toggle side-by-side (diff)
  r          refresh (inbox) / re-run analysis (detail)
  a then y   approve (y confirms; refused for BLOCK)
  c          comment
  ? esc      toggle help / back
  q          quit`
}

func (m Model) helpPanel() string {
	return helpText()
}

func (m Model) layoutView() string {
	header := m.headerView()
	footer := m.footerView()
	bodyHeight := max(4, m.height-lipgloss.Height(header)-lipgloss.Height(footer))

	if m.mode == ModeConfirmApprove {
		return lipgloss.JoinVertical(lipgloss.Left, header, m.centerModal(m.approvePanel(), bodyHeight), footer)
	}
	if m.mode == ModeComment {
		return lipgloss.JoinVertical(lipgloss.Left, header, m.centerModal(m.commentPanel(), bodyHeight), footer)
	}
	if m.mode == ModeHelp {
		return lipgloss.JoinVertical(lipgloss.Left, header, m.centerModal(m.helpPanel(), bodyHeight), footer)
	}

	var body string
	if m.width >= minWideWidth {
		body = m.wideLayout(bodyHeight)
	} else if m.width >= minMediumWidth {
		body = m.mediumLayout(bodyHeight)
	} else {
		body = m.compactLayout(bodyHeight)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m Model) headerView() string {
	parts := []string{m.theme.Styles.Title.Render("zappr"), m.repo}
	if !m.lastFetch.IsZero() {
		parts = append(parts, "refreshed "+shortDuration(time.Since(m.lastFetch))+" ago")
	}
	parts = append(parts, fmt.Sprintf("%d PRs", len(m.prs)))
	parts = append(parts, "sort: "+m.sortMode.String())
	if m.aiEnabled {
		parts = append(parts, m.theme.Styles.Muted.Render("AI: on"))
	} else {
		parts = append(parts, m.theme.Styles.Muted.Render("AI: off"))
	}
	if m.loading {
		parts = append(parts, m.theme.Styles.Pending.Render("loading"))
	}
	if m.error != "" {
		parts = append(parts, m.theme.Styles.Error.Render(m.error))
	} else if m.message != "" {
		parts = append(parts, m.theme.Styles.Muted.Render(m.message))
	}
	return m.theme.Styles.Header.Width(m.width).Render(strings.Join(parts, "  "))
}

func (m Model) footerView() string {
	if m.width < minMediumWidth {
		return m.theme.Styles.Footer.Width(m.width).Render("j/k move  enter details  v diff  ? help  q quit")
	}
	var keys string
	switch m.mode {
	case ModeDiff:
		keys = "j/k scroll  n/p file  s side-by-side  V pager  esc back  ? help  q quit"
	case ModeDetail:
		keys = "j/k scroll  tab focus  v diff  a approve, y confirm  c comment  s skip  r re-run analysis  esc inbox  ? help  q quit"
	default:
		keys = "j/k move  tab focus  o sort  enter inspect  v diff  a approve, y confirm  c comment  s skip  r refresh  ? help  q quit"
	}
	return m.theme.Styles.Footer.Width(m.width).Render(keys)
}

func (m Model) wideLayout(height int) string {
	gap := 1
	inboxW := max(28, m.width*28/100)
	summaryW := max(34, m.width*34/100)
	diffW := max(30, m.width-inboxW-summaryW-gap*2)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.pane("Inbox", m.inboxPanel(inboxW, height), inboxW, height, FocusInbox),
		strings.Repeat(" ", gap),
		m.pane("Summary", m.summaryPanel(summaryW, height), summaryW, height, FocusSummary),
		strings.Repeat(" ", gap),
		m.pane("Diff / Details", m.diffPanel(diffW, height), diffW, height, FocusDiff),
	)
}

func (m Model) mediumLayout(height int) string {
	topH := max(8, height/2)
	bottomH := max(6, height-topH)
	top := lipgloss.JoinHorizontal(lipgloss.Top,
		m.pane("Inbox", m.inboxPanel(m.width/2-1, topH), m.width/2-1, topH, FocusInbox),
		" ",
		m.pane("Summary", m.summaryPanel(m.width/2, topH), m.width/2, topH, FocusSummary),
	)
	bottom := m.pane("Diff / Details", m.diffPanel(m.width, bottomH), m.width, bottomH, FocusDiff)
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (m Model) compactLayout(height int) string {
	if m.mode == ModeDiff {
		return m.pane("Diff", m.diffPanel(m.width, height), m.width, height, FocusDiff)
	}
	if m.mode == ModeDetail {
		return m.pane("Details", m.summaryPanel(m.width, height), m.width, height, FocusSummary)
	}
	return m.pane("Inbox", m.inboxPanel(m.width, height), m.width, height, FocusInbox)
}

func (m Model) pane(title string, content string, width int, height int, focus Focus) string {
	style := m.theme.Styles.Pane
	if m.focus == focus {
		style = m.theme.Styles.Focused
	}
	innerW := max(1, width-style.GetHorizontalFrameSize())
	innerH := max(1, height-style.GetVerticalFrameSize())
	titleLine := m.theme.Styles.Title.Render(title)
	content = lipgloss.NewStyle().Width(innerW).Height(max(1, innerH-1)).Render(content)
	return style.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, titleLine, content))
}

func (m Model) inboxPanel(width int, height int) string {
	if len(m.prs) == 0 && !m.loading {
		return m.theme.Styles.Muted.Render("no open pull requests")
	}
	var b strings.Builder
	for i, pr := range m.prs {
		cursor := " "
		if i == m.selected {
			cursor = ">"
		}
		title := truncate(pr.Title, max(8, width-7))
		fmt.Fprintf(&b, "%s %s #%d\n", cursor, m.theme.RiskBadge(pr.Triage.Risk), pr.Number)
		fmt.Fprintf(&b, "  %s  +%d -%d  %d files\n", m.theme.CIStatus(pr.CIStatus), pr.Additions, pr.Deletions, pr.ChangedFiles)
		fmt.Fprintf(&b, "  %s\n", title)
		if pr.Skipped {
			fmt.Fprintf(&b, "  %s\n", m.theme.Styles.Muted.Render("skipped"))
		}
		if i < len(m.prs)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m Model) summaryPanel(width int, height int) string {
	pr, ok := m.current()
	if !ok {
		return m.theme.Styles.Muted.Render("select PR")
	}
	content := m.summaryContent(pr)
	vp := m.summaryVP
	vp.Width = max(1, width-4)
	vp.Height = max(1, height-4)
	vp.SetContent(content)
	return vp.View()
}

func (m Model) summaryContent(pr model.PullRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s #%d\n", pr.Title, pr.Number)
	fmt.Fprintf(&b, "Risk: %s  author: %s  CI: %s\n", plainRiskLabel(pr.Triage.Risk), pr.Author, pr.CIStatus)
	fmt.Fprintf(&b, "files: %d  +%d -%d\n\n", pr.ChangedFiles, pr.Additions, pr.Deletions)
	writePlainTriage(&b, pr.Triage)
	if len(pr.Files) > 0 {
		b.WriteString("\nFiles\n")
		for i, file := range pr.Files {
			marker := " "
			if i == m.fileIndex {
				marker = ">"
			}
			fmt.Fprintf(&b, "%s %-10s %s (+%d -%d)\n", marker, file.Status, file.Path, file.Additions, file.Deletions)
		}
	}
	return b.String()
}

func (m Model) diffPanel(width int, height int) string {
	if m.diff == "" {
		pr, ok := m.current()
		if !ok {
			return m.theme.Styles.Muted.Render("select PR")
		}
		var b strings.Builder
		b.WriteString("Files\n")
		for _, file := range pr.Files {
			fmt.Fprintf(&b, "%s  +%d -%d\n", file.Path, file.Additions, file.Deletions)
		}
		b.WriteString("\npress v to load diff, V to open full pager\n")
		return b.String()
	}
	content := m.diff
	if len(m.diffFiles) > 0 {
		content = m.diffFilesHeader() + "\n" + content
	}
	vp := m.diffVP
	vp.Width = max(1, width-4)
	vp.Height = max(1, height-4)
	vp.SetContent(content)
	return vp.View()
}

func (m Model) diffFilesHeader() string {
	var b strings.Builder
	limit := min(len(m.diffFiles), 8)
	for i := 0; i < limit; i++ {
		marker := " "
		if i == m.fileIndex {
			marker = ">"
		}
		fmt.Fprintf(&b, "%s %s\n", marker, m.theme.Styles.File.Render(m.diffFiles[i].Path))
	}
	if len(m.diffFiles) > limit {
		fmt.Fprintf(&b, "%s\n", m.theme.Styles.Muted.Render(fmt.Sprintf("... %d more files", len(m.diffFiles)-limit)))
	}
	return b.String()
}

func (m Model) centerModal(content string, height int) string {
	width := min(72, max(32, m.width-4))
	style := m.theme.Styles.Focused.Width(width).Padding(1, 2)
	rendered := style.Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, rendered)
}

func (m Model) approvePanel() string {
	pr, ok := m.current()
	if !ok {
		return "no PR selected"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Approve %s #%d?\n\n", pr.Repo, pr.Number)
	fmt.Fprintf(&b, "%s\n", m.theme.RiskBadge(pr.Triage.Risk))
	if pr.Triage.Risk == model.RiskBlock {
		b.WriteString(m.theme.Styles.Error.Render("Quick approval disabled for BLOCK risk."))
		b.WriteString("\n")
	}
	writeStyledTriage(&b, m, pr.Triage)
	b.WriteString("\ny approve  n/esc cancel")
	return b.String()
}

func (m Model) commentPanel() string {
	pr, ok := m.current()
	if !ok {
		return "no PR selected"
	}
	return fmt.Sprintf("Comment on %s #%d\n\n%s\n\nenter submit  esc cancel", pr.Repo, pr.Number, m.comment)
}

func writeTriage(b *strings.Builder, triage model.Triage) {
	b.WriteString("Rule-based verdict (authoritative)\n")
	fmt.Fprintf(b, "Risk: %s  Confidence: %s\n", riskLabel(triage.Risk), confidencePct(triage.Confidence))
	fmt.Fprintf(b, "Summary: %s\n", triage.Summary)
	writeList(b, "Reasons", triage.Reasons)
	writeList(b, "Concerns", triage.Concerns)
	writeList(b, "High-risk files", triage.HighRiskFiles)
	writeList(b, "Review targets", triage.ReviewTargets)
	writeAINotes(b, triage)
}

func writeStyledTriage(b *strings.Builder, m Model, triage model.Triage) {
	b.WriteString(m.theme.Styles.Title.Render("Rule-based verdict (authoritative)") + "\n")
	fmt.Fprintf(b, "Risk: %s  Confidence: %s\n", m.theme.RiskBadge(triage.Risk), confidencePct(triage.Confidence))
	fmt.Fprintf(b, "Summary: %s\n", triage.Summary)
	writeList(b, "Reasons", triage.Reasons)
	writeList(b, "Concerns", triage.Concerns)
	writeList(b, "High-risk files", triage.HighRiskFiles)
	writeList(b, "Review targets", triage.ReviewTargets)
	writeStyledAINotes(b, m, triage)
}

func writePlainTriage(b *strings.Builder, triage model.Triage) {
	b.WriteString("Rule-based verdict (authoritative)\n")
	fmt.Fprintf(b, "Confidence: %s\n", confidencePct(triage.Confidence))
	fmt.Fprintf(b, "Summary: %s\n", triage.Summary)
	writeList(b, "Reasons", triage.Reasons)
	writeList(b, "Concerns", triage.Concerns)
	writeList(b, "High-risk files", triage.HighRiskFiles)
	writeList(b, "Review targets", triage.ReviewTargets)
	writeAINotes(b, triage)
}

func writeAINotes(b *strings.Builder, triage model.Triage) {
	if triage.AI == nil {
		b.WriteString("\nAI: not run (advisory only)\n")
		return
	}
	b.WriteString("\nAI (advisory - does not affect risk)\n")
	if triage.AI.Error != "" {
		fmt.Fprintf(b, "AI error: %s\n", triage.AI.Error)
		return
	}
	fmt.Fprintf(b, "- %s\n", triage.AI.Summary)
	writeList(b, "AI focus", triage.AI.SuggestedFocus)
}

func writeStyledAINotes(b *strings.Builder, m Model, triage model.Triage) {
	if triage.AI == nil {
		b.WriteString("\n" + m.theme.Styles.Muted.Render("AI: not run (advisory only)") + "\n")
		return
	}
	b.WriteString("\n" + m.theme.Styles.Subtitle.Render("AI (advisory - does not affect risk)") + "\n")
	if triage.AI.Error != "" {
		b.WriteString(m.theme.Styles.Error.Render("AI error: "+triage.AI.Error) + "\n")
		return
	}
	fmt.Fprintf(b, "- %s\n", triage.AI.Summary)
	writeList(b, "AI focus", triage.AI.SuggestedFocus)
}

func confidencePct(c float64) string {
	return fmt.Sprintf("%d%%", int(c*100+0.5))
}

func plainRiskLabel(r model.Risk) string {
	switch r {
	case model.RiskSafe:
		return "SAFE ok"
	case model.RiskReview:
		return "REVIEW !"
	case model.RiskBlock:
		return "BLOCK x"
	default:
		return "unknown"
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

func truncate(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	if width <= 1 {
		return "."
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
