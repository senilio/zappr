package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

const minWideWidth = 110
const minMediumWidth = 76
const paneFrame = 2   // border top + border bottom (lipgloss frame vertical)
const paneHFrame = 4  // border (2) + padding (2) horizontal
const chromeLines = 1 // header only
const paneContentSafetyMargin = 0
const diffContentSafetyMargin = 0
const tabStopWidth = 8

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
	case ModeSettings:
		return m.settingsView()
	case ModeCommand, ModeConfirmCommand:
		return m.commandView()
	case ModeColorscheme:
		return m.layoutView()
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

	fmt.Fprintf(&b, "%-3s %-8s %-8s %s\n", "", "PR", "CI", "Title")
	for i, pr := range m.prs {
		line := fmt.Sprintf("#%-7d %-8s %s", pr.Number, pr.CIStatus, pr.Title)
		if i == m.selected {
			line = m.theme.Styles.TitleStrong.Render(line)
		} else {
			line = m.theme.Styles.Muted.Render(line)
		}
		fmt.Fprintf(&b, "%s\n", line)
	}
	fmt.Fprintf(&b, "\n%s/%s navigate  %s inspect  %s refresh  %s toggle diff  %s/%s/%s summary\n", m.actionAlias("move-down"), m.actionAlias("move-up"), m.actionAlias("inspect-pr"), m.actionAlias("refresh-pr"), m.actionAlias("toggle-diff"), m.actionAlias("summary-files"), m.actionAlias("summary-commits"), m.actionAlias("summary-comments"))
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
	fmt.Fprintf(&b, "%s #%d\n", pr.Repo, pr.Number)
	fmt.Fprintf(&b, "%s\n", pr.Title)
	fmt.Fprintf(&b, "Author: %s  CI: %s  Files: %d  +%d -%d\n\n", pr.Author, pr.CIStatus, pr.ChangedFiles, pr.Additions, pr.Deletions)
	b.WriteString("\nFiles:\n")
	for _, file := range pr.Files {
		fmt.Fprintf(&b, "%-8s %s (+%d -%d)\n", file.Status, file.Path, file.Additions, file.Deletions)
	}
	fmt.Fprintf(&b, "\n%s refresh  %s toggle diff  %s full diff pager  %s/%s/%s summary  %s inbox\n", m.actionAlias("refresh-pr"), m.actionAlias("toggle-diff"), m.actionAlias("open-diff-pager"), m.actionAlias("summary-files"), m.actionAlias("summary-commits"), m.actionAlias("summary-comments"), m.actionAlias("back-to-inbox"))
	return b.String()
}

func (m Model) diffView() string {
	if m.width > 0 {
		return m.layoutView()
	}
	return fmt.Sprintf("%s\n\n%s back  %s quit\n", m.diff, m.actionAlias("diff-close"), m.actionAlias("quit"))
}

func (m Model) approveView() string {
	if m.width > 0 {
		return m.layoutView()
	}

	pr, ok := m.current()
	if !ok {
		return m.inboxView()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Approve %s #%d?\n\n", pr.Repo, pr.Number)
	fmt.Fprintf(&b, "\nPress %s to approve via gh pr review --approve, %s to cancel.\n", m.actionAlias("approve-confirm"), m.actionAlias("approve-cancel"))
	return b.String()
}

func (m Model) commentView() string {
	pr, ok := m.current()
	if !ok {
		return m.inboxView()
	}
	return fmt.Sprintf("Comment on %s #%d\n\n%s\n\n%s submit  %s cancel\n", pr.Repo, pr.Number, m.comment, m.actionAlias("comment-submit"), m.actionAlias("comment-cancel"))
}

func (m Model) helpView() string {
	if m.width > 0 {
		return m.layoutView()
	}
	return m.helpPanel() + "\n"
}

func (m Model) settingsView() string {
	if m.width > 0 {
		return m.layoutView()
	}
	return fmt.Sprintf("%s\n%s back  %s quit\n", m.settingsPanel(), m.actionAlias("settings-close"), m.actionAlias("quit"))
}

func (m Model) helpPanel() string {
	var b strings.Builder
	width := 66
	if m.width > 0 {
		width = min(66, max(1, m.width-10))
	}
	line := func(format string, args ...any) {
		b.WriteString(truncateANSIWidth(fmt.Sprintf(format, args...), width))
		b.WriteByte('\n')
	}
	type shortcut struct{ key, label string }
	section := func(title string, shortcuts ...shortcut) {
		line("%s", m.theme.Styles.Title.Render(title))
		cell := func(shortcut shortcut, cellWidth int) string {
			keyWidth := min(16, max(11, cellWidth/3))
			key := truncateANSIWidth(shortcut.key, keyWidth)
			labelWidth := max(1, cellWidth-keyWidth-3)
			label := truncateANSIWidth(shortcut.label, labelWidth)
			return "  " + m.theme.Styles.TitleStrong.Render(key+strings.Repeat(" ", max(0, keyWidth-ansi.StringWidth(key)))) + " " + fmt.Sprintf("%-*s", labelWidth, label)
		}
		columns := 1
		cellWidth := width
		if columns == 2 {
			cellWidth = (width - 2) / 2
		}
		for i := 0; i < len(shortcuts); i += columns {
			row := cell(shortcuts[i], cellWidth)
			if columns == 2 && i+1 < len(shortcuts) {
				row += "  " + cell(shortcuts[i+1], cellWidth)
			}
			line("%s", row)
		}
		line("")
	}

	section("Navigate",
		shortcut{"↑/↓", "move"},
		shortcut{"enter", "inspect"},
		shortcut{"tab/shift+tab", "switch pane"},
		shortcut{"g/G", "top/bottom"},
		shortcut{"pgdown/space", "page down"},
		shortcut{"pgup", "page up"},
		shortcut{"esc", "back to inbox"},
		shortcut{m.actionAlias("quit"), "quit"},
	)
	section("Review",
		shortcut{"1/2/3", "files/commits/comments"},
		shortcut{"shift+↑/↓", "next/previous file/commit"},
		shortcut{m.actionAlias("toggle-diff"), "toggle diff"},
		shortcut{m.actionAlias("toggle-wrap"), "wrap focused pane"},
		shortcut{"←/→", "pan diff/commits"},
		shortcut{m.actionAlias("toggle-side-by-side"), "side-by-side diff"},
		shortcut{m.actionAlias("toggle-file-content"), "file content/diff"},
		shortcut{m.actionAlias("open-diff-pager"), "open diff pager"},
	)
	actions := []shortcut{
		{m.actionAlias("open-pr"), "open PR"},
		{m.actionAlias("copy-pr-url"), "copy PR URL"},
		{m.actionAlias("refresh-pr"), "refresh PR"},
		{m.actionAlias("refresh-inbox"), "refresh inbox"},
		{m.actionAlias("approve"), "approve [?]"},
		{m.actionAlias("approve-now"), "approve now"},
		{m.actionAlias("comment"), "write comment"},
		{m.actionAlias("toggle-draft"), "toggle draft/ready"},
		{m.actionAlias("toggle-automerge"), "toggle auto-merge"},
		{m.actionAlias("settings"), "inbox settings"},
	}
	builtInActions := map[string]bool{"approve": true, "approve-now": true, "comment": true}
	descriptors := make(map[string]bool, len(actionDescriptors))
	for _, descriptor := range actionDescriptors {
		descriptors[descriptor.ID] = true
	}
	for _, action := range m.actions.actions {
		if action.Operation != "" && !descriptors[action.ID] && !builtInActions[action.ID] && len(action.Keys) > 0 {
			actions = append(actions, shortcut{strings.Join(action.Keys, "/"), actionDisplayLabel(action)})
		}
	}
	section("Actions", actions...)
	section("PR badges", shortcut{"[auto]", "auto-merge enabled"}, shortcut{"[draft]", "draft pull request"})
	content := strings.TrimSuffix(b.String(), "\n")
	if m.helpInput == "" {
		return content
	}
	var matches []string
	query := strings.ToLower(m.helpInput)
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(strings.ToLower(line), query) {
			matches = append(matches, line)
		}
	}
	return strings.Join(matches, "\n")
}

func (m Model) actionHelpLine(action tuiAction) string {
	aliases := truncateANSIWidth(actionAliases(action), 18)
	label := actionDisplayLabel(action)
	if action.Confirmation != confirmationNone {
		label += " [?]"
	}
	width := min(66, m.width-10) - 22
	if m.width <= 0 {
		width = len(label)
	}
	return fmt.Sprintf("  %-18s %s", aliases, truncateANSIWidth(label, max(1, width)))
}

func actionAliases(action tuiAction) string {
	aliases := append([]string(nil), action.Keys...)
	for _, command := range action.Commands {
		aliases = append(aliases, ":"+command)
	}
	return strings.Join(aliases, "/")
}

func actionDisplayLabel(action tuiAction) string {
	label := action.Label
	if label == "" && action.Operation != "" {
		label = genericActionLabel(action)
	}
	return strings.NewReplacer("\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(label)
}

func confirmationActionLabel(action tuiAction) string {
	if action.Label != "" {
		return actionDisplayLabel(action)
	}
	if action.Operation == "comment" && action.Comment != "" {
		return fmt.Sprintf("Comment %q", action.Comment)
	}
	return actionDisplayLabel(action)
}

func (m Model) layoutView() string {
	header := m.headerView()
	bodyHeight := max(4, m.height-chromeLines)
	fit := func(s string) string { return fitScreen(s, m.width, m.height) }

	var body string
	if m.width >= minWideWidth {
		body = m.wideLayout(bodyHeight)
	} else if m.width >= minMediumWidth {
		body = m.mediumLayout(bodyHeight)
	} else {
		body = m.compactLayout(bodyHeight)
	}

	// Modal modes overlay a centered box over the dimmed body layout (matching
	// the command palette), rather than rendering over an empty black backdrop.
	var modal string
	switch {
	case m.mode == ModeConfirmApprove:
		modal = m.renderModalBox(m.approvePanel())
	case m.mode == ModeComment:
		modal = m.renderModalBox(m.commentPanel())
	case m.mode == ModeHelp:
		modal = m.renderHelpModal()
	case m.mode == ModeSettings:
		modal = m.renderModalBox(m.settingsPanel())
	case m.mode == ModeColorscheme:
		modal = m.renderModalBox(m.colorschemePanel())
	case m.mode == ModeCommand || m.mode == ModeConfirmCommand:
		modal = m.renderCommandModal()
	}
	if modal != "" {
		body = overlayModal(body, modal, m.width, bodyHeight, m.selectedInboxPreserveRanges(bodyHeight))
	}

	return fit(lipgloss.JoinVertical(lipgloss.Left, header, body))
}

func (m Model) renderHelpModal() string {
	width := min(60, max(32, m.width-8))
	height := m.helpModalHeight() - 3
	search := "Search: " + m.helpInput + "_"
	padding := max(1, (m.helpVP.Width+1-ansi.StringWidth(search))/2-ansi.StringWidth("zappr help"))
	content := fmt.Sprintf("zappr help%s%s\n\n%s\n\nup/down pgup/pgdown  esc close", strings.Repeat(" ", padding), search, m.helpViewportView())
	return m.theme.Styles.Focused.Width(width).Height(height).Padding(0, 2).Render(content)
}

func (m Model) helpModalHeight() int {
	return max(8, (m.height-chromeLines)/3)
}

func (m Model) helpViewportView() string {
	lines := strings.Split(m.helpVP.View(), "\n")
	if m.helpVP.TotalLineCount() <= m.helpVP.Height {
		return strings.Join(lines, "\n")
	}
	thumb := int(m.helpVP.ScrollPercent() * float64(max(0, len(lines)-1)))
	for i, line := range lines {
		bar := "│"
		if i == thumb {
			bar = "█"
		}
		line = truncateANSIWidth(line, m.helpVP.Width)
		lines[i] = line + strings.Repeat(" ", max(0, m.helpVP.Width-ansi.StringWidth(line))) + m.theme.Styles.Muted.Render(bar)
	}
	return strings.Join(lines, "\n")
}

// renderCommandModal renders the command palette box (border + padding + content).
func (m Model) renderCommandModal() string {
	width := m.commandModalWidth()
	style := m.theme.Styles.Focused.Width(width).Padding(1, 2)
	return style.Render(m.commandPanel())
}

func (m Model) commandModalWidth() int {
	return min(60, max(32, m.width-8))
}

// overlayModal composites a modal box centered over a dimmed background.
// bg is expected to be bodyHeight lines wide at width columns.
// The background is stripped of color and dimmed; the modal renders at full color.
type preserveRange struct {
	row        int
	start, end int
}

func overlayModal(bg, modal string, width, height int, preserve []preserveRange) string {
	// Strip ANSI from each background line, then apply uniform dim.
	// This ensures no internal reset sequences break the dim attribute.
	bgLines := strings.Split(bg, "\n")
	for len(bgLines) < height {
		bgLines = append(bgLines, "")
	}
	bgLines = bgLines[:height]
	dimmedBg := make([]string, height)
	for i, line := range bgLines {
		dimmedBg[i] = dimLineWithPreserve(line, width, rangesForRow(preserve, i))
	}

	// Split modal into lines and measure its width.
	modalLines := strings.Split(modal, "\n")
	if len(modalLines) > 0 && modalLines[len(modalLines)-1] == "" {
		modalLines = modalLines[:len(modalLines)-1]
	}
	modalH := len(modalLines)
	modalW := 0
	for _, l := range modalLines {
		if w := ansi.StringWidth(l); w > modalW {
			modalW = w
		}
	}

	// Center position.
	startRow := max(0, (height-modalH)/2)
	startCol := max(0, (width-modalW)/2)

	// Splice modal lines into dimmed background.
	result := make([]string, height)
	copy(result, dimmedBg)
	for i, mLine := range modalLines {
		row := startRow + i
		if row >= height {
			break
		}
		// bg line is plain text (no ANSI); slice by column directly.
		plain := stripANSI(bgLines[row])
		missing := width - ansi.StringWidth(plain)
		if missing > 0 {
			plain += strings.Repeat(" ", missing)
		}
		left := ""
		if startCol > 0 {
			left = "\x1b[2m" + ansi.Cut(plain, 0, startCol) + "\x1b[0m"
		}
		right := ""
		modalEnd := startCol + modalW
		if modalEnd < width {
			right = "\x1b[2m" + ansi.Cut(plain, modalEnd, width) + "\x1b[0m"
		}
		result[row] = left + mLine + right
	}

	return strings.Join(result, "\n")
}

func dimLineWithPreserve(line string, width int, ranges []preserveRange) string {
	plain := stripANSI(line)
	missing := width - ansi.StringWidth(plain)
	if missing > 0 {
		plain += strings.Repeat(" ", missing)
	}
	if len(ranges) == 0 {
		return "\x1b[2m" + plain + "\x1b[0m"
	}
	var b strings.Builder
	pos := 0
	for _, r := range ranges {
		start := max(pos, min(width, r.start))
		end := max(start, min(width, r.end))
		if start > pos {
			b.WriteString("\x1b[2m" + ansi.Cut(plain, pos, start) + "\x1b[0m")
		}
		if end > start {
			b.WriteString(ansi.Cut(line, start, end))
		}
		pos = end
	}
	if pos < width {
		b.WriteString("\x1b[2m" + ansi.Cut(plain, pos, width) + "\x1b[0m")
	}
	return b.String()
}

func rangesForRow(ranges []preserveRange, row int) []preserveRange {
	var out []preserveRange
	for _, r := range ranges {
		if r.row == row {
			out = append(out, r)
		}
	}
	return out
}

func (m Model) selectedInboxPreserveRanges(bodyHeight int) []preserveRange {
	if len(m.prs) == 0 || m.selected < 0 || m.selected >= len(m.prs) || m.width <= 0 {
		return nil
	}
	for _, region := range m.paneRegions() {
		if region.focus != FocusInbox {
			continue
		}
		var ranges []preserveRange
		contentH := max(1, region.h-paneFrame)
		_, spans := m.inboxContent(max(1, region.w-paneHFrame))
		yOffset := m.inboxOffset(max(1, region.w-paneHFrame), contentH)
		for itemLine := spans[m.selected].start; itemLine < spans[m.selected].end; itemLine++ {
			viewLine := itemLine - yOffset
			if viewLine < 0 || viewLine >= contentH {
				continue
			}
			row := region.y - 1 + 1 + viewLine
			if row < 0 || row >= bodyHeight {
				continue
			}
			start := region.x + 2
			end := start + max(1, region.w-paneHFrame)
			ranges = append(ranges, preserveRange{row: row, start: start, end: end})
		}
		return ranges
	}
	return nil
}

func (m Model) selectedInboxYOffset(contentH int) int {
	w, _, _ := m.paneInnerWidths()
	return m.inboxOffset(w, contentH)
}

func fitScreen(content string, width int, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = padANSIWidth(truncateANSIWidth(lines[i], width), width)
	}
	return strings.Join(lines, "\n")
}

// headerView renders the top status bar. When a background operation is
// active (statusActive), the spinner and message are appended here so the
// indicator is always visible and never overlaps pane borders.
func (m Model) headerView() string {
	parts := []string{m.theme.Styles.Title.Render("zappr ⚡"), m.repo}
	// Status indicator: shown inline in header so it never overlaps pane borders.
	if m.statusActive {
		if m.statusErr {
			parts = append(parts, m.theme.Styles.Error.Render("✗ "+m.statusText))
		} else {
			parts = append(parts, strings.TrimRight(m.statusSpinner.View(), " ")+" "+m.theme.Styles.Pending.Render(m.statusText))
		}
	} else if m.loading {
		parts = append(parts, m.theme.Styles.Pending.Render("loading"))
	} else if m.error != "" {
		parts = append(parts, m.theme.Styles.Error.Render(m.error))
	}
	left := strings.Join(parts, " ")
	right := m.footerShortcuts([]shortcut{{m.actionAlias("help"), "help"}, {m.actionAlias("quit"), "quit"}})
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	line := left + strings.Repeat(" ", gap) + right
	return railView(m.theme.Styles.Header.Width(m.width).Render(line), m.width)
}

func (m Model) actionAlias(id string) string {
	action, ok := m.actions.actionByID(id)
	if !ok || len(action.Keys) == 0 {
		return ""
	}
	return truncateANSIWidth(strings.Join(action.Keys, "/"), 18)
}

func (m Model) footerView() string {
	if m.width < minMediumWidth {
		return railView(m.theme.Styles.Footer.Width(m.width).Render(m.footerShortcuts([]shortcut{{"v", "diff"}, {"a", "approve"}, {"c", "comment"}})), m.width)
	}
	var keys []shortcut
	switch m.focus {
	case FocusDiff:
		keys = []shortcut{{"left/right", "pan"}, {"n/p", "file"}, {"x", m.diffMode.String()}, {"s", "side-by-side"}, {"V", "pager"}, {"esc", "back"}}
	case FocusSummary:
		if m.summaryMode == SummaryModeCommits {
			keys = []shortcut{{"j/k", "select"}, {"left/right", "pan"}, {"v", "diff"}, {"a", "approve"}, {"esc", "inbox"}}
		} else {
			keys = []shortcut{{"v", "diff"}, {"a", "approve"}, {"c", "comment"}, {"r", "refresh"}, {"esc", "inbox"}}
		}
	default:
		keys = []shortcut{{"f", "filter"}, {"o", "open"}, {"C", "copy URL"}, {"v", "diff"}, {"a", "approve"}, {"c", "comment"}, {"r", "refresh"}}
	}
	return railView(m.theme.Styles.Footer.Width(m.width).Render(m.footerShortcuts(keys)), m.width)
}

func railView(s string, width int) string {
	return strings.TrimRight(truncateANSIWidth(strings.ReplaceAll(s, "\n", " "), max(0, width)), "\n")
}

type shortcut struct {
	Key   string
	Label string
}

func (m Model) footerShortcuts(keys []shortcut) string {
	parts := make([]string, 0, len(keys))
	keyStyle := m.theme.Styles.TitleStrong
	labelStyle := m.theme.Styles.Muted
	for _, key := range keys {
		parts = append(parts, keyStyle.Render(key.Key)+" "+labelStyle.Render(key.Label))
	}
	return strings.Join(parts, "  ")
}

func (m Model) wideLayout(height int) string {
	leftW, diffW, gap := m.widePaneOuterWidths()
	inboxH, summaryH := m.leftColumnHeights(height)
	left := lipgloss.JoinVertical(lipgloss.Left,
		m.pane(m.titledPane(m.inboxTitle(), m.inboxScrollPct()), m.inboxPanel(leftW, inboxH), leftW, inboxH, FocusInbox),
		m.pane(m.titledPane(m.summaryTitle(), m.summaryScrollPct()), m.summaryPanel(leftW, summaryH), leftW, summaryH, FocusSummary),
	)
	right := m.pane(m.titledPane(m.diffTitle(), m.diffScrollPct()), m.diffPanel(diffW, height), diffW, height, FocusDiff)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
}

func (m Model) widePaneOuterWidths() (left, diff, gap int) {
	gap = 1
	left = max(28, m.width*40/100) - 2
	diff = max(30, m.width-left-gap)
	return left, diff, gap
}

func (m Model) paneStyle(focus Focus) lipgloss.Style {
	style := m.theme.Styles.Pane
	if m.focus == focus {
		style = m.theme.Styles.Focused
	}
	return style
}

func (m Model) paneInnerWidth(width int, focus Focus) int {
	return max(1, width-m.paneStyle(focus).GetHorizontalFrameSize())
}

func (m Model) paneInnerHeight(height int, focus Focus) int {
	return max(1, height-m.paneStyle(focus).GetVerticalFrameSize())
}

func (m Model) diffPaneOuterWidth() int {
	if m.width >= minWideWidth {
		_, diffW, _ := m.widePaneOuterWidths()
		return diffW
	}
	return m.width
}

// leftColumnHeights returns stable pane heights for left column.
func (m Model) leftColumnHeights(height int) (inboxH, summaryH int) {
	if m.focus == FocusInbox || m.mode == ModeInbox {
		inboxH = max(8, height*64/100)
	} else {
		summaryH = max(8, height*80/100)
		inboxH = max(4, height-summaryH)
		return
	}
	summaryH = max(4, height-inboxH)
	return
}

func (m Model) mediumLayout(height int) string {
	topH := max(8, height/2)
	bottomH := max(6, height-topH)
	top := lipgloss.JoinHorizontal(lipgloss.Top,
		m.pane(m.titledPane(m.inboxTitle(), m.inboxScrollPct()), m.inboxPanel(m.width/2-1, topH), m.width/2-1, topH, FocusInbox),
		" ",
		m.pane(m.titledPane(m.summaryTitle(), m.summaryScrollPct()), m.summaryPanel(m.width/2, topH), m.width/2, topH, FocusSummary),
	)
	bottom := m.pane(m.titledPane(m.diffTitle(), m.diffScrollPct()), m.diffPanel(m.width, bottomH), m.width, bottomH, FocusDiff)
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (m Model) compactLayout(height int) string {
	if m.mode == ModeDiff {
		return m.pane(m.titledPane("Diff", m.diffScrollPct()), m.diffPanel(m.width, height), m.width, height, FocusDiff)
	}
	if m.mode == ModeDetail {
		return m.pane(m.titledPane(m.summaryTitle(), m.summaryScrollPct()), m.summaryPanel(m.width, height), m.width, height, FocusSummary)
	}
	return m.pane(m.titledPane(m.inboxTitle(), m.inboxScrollPct()), m.inboxPanel(m.width, height), m.width, height, FocusInbox)
}

func (m Model) inboxTitle() string {
	if len(m.prs) == 0 {
		return "Inbox (0/0)"
	}
	return fmt.Sprintf("Inbox (%d/%d)", m.selected+1, len(m.prs))
}

func (m Model) summaryTitle() string {
	parts := []string{
		m.summaryModeTitle("summary-files", SummaryModeFiles),
		m.summaryModeTitle("summary-commits", SummaryModeCommits),
		m.summaryModeTitle("summary-comments", SummaryModeComments),
	}
	return "Summary " + strings.Join(parts, " ")
}

func (m Model) summaryModeTitle(actionID string, mode SummaryMode) string {
	alias := m.actionAlias(actionID)
	label := fmt.Sprintf("[%s]", alias)
	if m.summaryMode == mode {
		label = fmt.Sprintf("[%s %s]", alias, mode.String())
		return m.theme.Styles.TitleStrong.Render(label)
	}
	return m.theme.Styles.Muted.Render(label)
}

func (m Model) diffTitle() string {
	if m.summaryMode == SummaryModeCommits {
		return "Commit diff"
	}
	if m.diffMode == DiffModeFile {
		return "File"
	}
	return "Diff"
}

// titledPane appends a scroll percentage to a pane title when pct > 0.
// Returns "Title 42%" or just "Title" when pct is 0 (at top or no overflow).
func (m Model) titledPane(base string, pct int) string {
	if pct > 0 {
		return fmt.Sprintf("%s %d%%", base, pct)
	}
	return base
}

// inboxScrollPct returns the scroll percentage of the inbox viewport (0 = at top or no overflow).
func (m Model) inboxScrollPct() int {
	return vpScrollPct(m.inboxVP.YOffset, m.inboxVP.TotalLineCount(), m.inboxVP.VisibleLineCount())
}

// summaryScrollPct returns the scroll percentage of the summary viewport.
func (m Model) summaryScrollPct() int {
	return vpScrollPct(m.summaryVP.YOffset, m.summaryVP.TotalLineCount(), m.summaryVP.VisibleLineCount())
}

// diffScrollPct returns the scroll percentage of the diff viewport.
func (m Model) diffScrollPct() int {
	return vpScrollPct(m.diffVP.YOffset, m.diffVP.TotalLineCount(), m.diffVP.VisibleLineCount())
}

// vpScrollPct computes a [0,100] scroll percentage from viewport metrics.
// Returns 0 when at the top, content fits in view, or metrics are unset.
func vpScrollPct(yOffset, totalLines, visibleLines int) int {
	scrollable := totalLines - visibleLines
	if scrollable <= 0 || yOffset <= 0 {
		return 0
	}
	pct := int(float64(yOffset) / float64(scrollable) * 100)
	if pct > 99 {
		pct = 99
	}
	return pct
}

// paneInnerWidths returns the content (inner) width for each pane given the
// current layout and terminal width. Outer pane width minus paneHFrame (4).
func (m Model) paneInnerWidths() (inbox, summary, diff int) {
	if m.width >= minWideWidth {
		leftW, diffW, _ := m.widePaneOuterWidths()
		inbox = max(1, leftW-paneHFrame)
		summary = inbox
		diff = max(1, diffW-paneHFrame)
	} else if m.width >= minMediumWidth {
		half := m.width / 2
		inbox = max(1, half-1-paneHFrame)
		summary = max(1, half-paneHFrame)
		diff = max(1, m.width-paneHFrame)
	} else {
		w := max(1, m.width-paneHFrame)
		inbox, summary, diff = w, w, w
	}
	return
}

// wrapLines word-wraps each logical line in s to at most w visible columns.
// Preserves explicit newlines; hard-breaks words longer than w.
// Uses lipgloss so ANSI escape codes do not count toward width.
func wrapLines(s string, w int) string {
	return ansi.Wrap(s, max(1, w), "")
}

func (m Model) pane(title string, content string, width int, height int, focus Focus) string {
	style := m.paneStyle(focus)
	// Treat width/height as the total outer size; subtract the frame so the
	// rendered box (border + padding + content) fits exactly within the
	// allotted columns. This keeps the rightmost pane's right border visible.
	innerW := max(1, width-style.GetHorizontalFrameSize())
	innerH := max(1, height-style.GetVerticalFrameSize())
	// Hard-fit content before border rendering. lipgloss sizing can pad, but it
	// must not be the only overflow guard because wide lines can reflow the box.
	content = fitPaneContent(content, innerW, innerH)
	// lipgloss Width() sets the content+padding area; the border is added
	// outside. To make the rendered box exactly `width` columns wide, pass
	// width minus the horizontal border only (not the full frame), otherwise
	// padding is double-subtracted and the box renders 2 columns too narrow.
	boxW := max(1, width-style.GetHorizontalBorderSize())
	box := style.Width(boxW).Height(innerH).Render(content)
	return embedTitle(box, title, style, m.theme.NoColor)
}

func fitPaneContent(content string, width int, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = truncateANSIWidth(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

// embedTitle overwrites the center of the first line of a rendered box with
// the pane title, centered and surrounded by a single space on each side.
// The title text is colored to match the border foreground. Corner glyphs are
// preserved. Falls back to returning the box unchanged if there is not enough
// room for the title.
func embedTitle(box string, title string, style lipgloss.Style, noColor bool) string {
	lines := strings.SplitN(box, "\n", 2)
	if len(lines) == 0 {
		return box
	}
	topLine := lines[0]
	rest := ""
	if len(lines) > 1 {
		rest = "\n" + lines[1]
	}

	// Measure visible width of the top border line.
	lineWidth := lipgloss.Width(topLine)
	// We need at least: 2 corner chars + 2 spaces + 1 title char.
	needed := lipgloss.Width(title) + 2 // " title "
	if lineWidth < needed+2 {
		return box
	}

	// Build styled title segment " title ".
	titleStyle := lipgloss.NewStyle()
	if !noColor {
		titleStyle = titleStyle.Foreground(style.GetBorderTopForeground())
	}
	segment := titleStyle.Render(" " + title + " ")
	segWidth := lipgloss.Width(title) + 2 // visual width of the segment

	// Position: center within the line, keeping 1 char margin for corners.
	available := lineWidth - 2 // exclude corner cols
	leftPad := (available - segWidth) / 2
	rightPad := available - segWidth - leftPad

	// Strip ANSI from topLine to slice by rune position (corners + filler).
	plain := stripANSI(topLine)
	runes := []rune(plain)
	if len(runes) < lineWidth {
		return box
	}

	// Rebuild the top border glyph-by-glyph (no ANSI in original anyway for
	// rounded/thick borders from lipgloss — it applies color via the border
	// foreground which wraps the whole line). We reconstruct using the border
	// top glyph and corner glyphs directly from the style's border definition.
	bs := style.GetBorderStyle()
	cornerL := bs.TopLeft
	cornerR := bs.TopRight
	filler := bs.Top
	if filler == "" {
		filler = "─"
	}
	if cornerL == "" {
		cornerL = string(runes[0])
	}
	if cornerR == "" {
		cornerR = string(runes[len(runes)-1])
	}

	borderStyle := lipgloss.NewStyle()
	if !noColor {
		borderStyle = borderStyle.Foreground(style.GetBorderTopForeground())
	}

	left := borderStyle.Render(cornerL + strings.Repeat(filler, leftPad))
	right := borderStyle.Render(strings.Repeat(filler, rightPad) + cornerR)
	newTop := left + segment + right

	return newTop + rest
}

func (m Model) inboxPanel(width int, height int) string {
	if len(m.prs) == 0 && !m.loading {
		return m.theme.Styles.Muted.Render("no open pull requests")
	}
	innerW := max(1, width-paneHFrame)
	layout := m.inboxPane(innerW)
	vpH := max(1, height-paneFrame)
	vp := layout.viewport
	vp.Height = vpH
	vp.SetYOffset(m.inboxOffsetFor(layout.spans, vpH))
	return vp.View()
}

func (m Model) summaryPanel(width int, height int) string {
	pr, ok := m.current()
	if !ok {
		return m.theme.Styles.Muted.Render("select PR")
	}
	innerW := max(1, width-paneHFrame)
	layout := m.summaryPane(pr, innerW)
	vp := layout.viewport
	vp.Height = max(1, height-paneFrame)
	vp.SetYOffset(m.summaryVP.YOffset)
	return vp.View()
}

func (m Model) summaryViewportContent(rawContent string, innerW int) string {
	// In commits mode apply horizontal scrolling: cut each line by x-offset.
	if m.summaryMode == SummaryModeCommits && m.summaryXOffset > 0 && !m.cfg.Wrap.Summary {
		lines := strings.Split(rawContent, "\n")
		for i, line := range lines {
			lines[i] = truncateANSIWidth(ansi.Cut(line, m.summaryXOffset, m.summaryXOffset+innerW), innerW)
		}
		rawContent = strings.Join(lines, "\n")
	} else {
		rawContent = fitPaneContentWidth(rawContent, innerW)
	}
	return rawContent
}

func fitPaneContentWidth(content string, width int) string {
	if width <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = truncatePaneLineWidth(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func fitContentWidthExact(content string, width int) string {
	if width <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = truncateANSIWidth(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func truncatePaneLineWidth(s string, width int) string {
	return truncateANSIWidth(s, max(1, width-paneContentSafetyMargin))
}

func (m Model) summaryContent(pr model.PullRequest, innerW int) string {
	content, _ := m.summaryLayout(pr, innerW)
	return content
}

func (m Model) unwrappedSummaryContent(pr model.PullRequest, innerW int) string {
	var b strings.Builder

	switch m.summaryMode {
	case SummaryModeCommits:
		m.writeCommitSummary(&b, pr, innerW)
	case SummaryModeComments:
		m.writeCommentSummary(&b, pr, innerW)
	default:
		m.writeFileSummary(&b, pr, innerW)
	}
	return b.String()
}

const fileSummaryIndent = "  "

// splitFileDir splits a changed-file path into its directory (with a trailing
// "/", or "./" for repository-root files) and its base name.
func splitFileDir(path string) (dir, name string) {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "./", path
	}
	return path[:i+1], path[i+1:]
}

// fileSummaryLines returns the rendered line of each file in the Files
// summary, where a directory header precedes each run of files sharing a
// directory.
func fileSummaryLines(files []model.ChangedFile) []int {
	lines := make([]int, len(files))
	line, prevDir := 0, ""
	for i, file := range files {
		dir, _ := splitFileDir(file.Path)
		if i == 0 || dir != prevDir {
			line++
			prevDir = dir
		}
		lines[i] = line
		line++
	}
	return lines
}

func (m Model) writeFileSummary(b *strings.Builder, pr model.PullRequest, innerW int) {
	s := m.theme.Styles
	if len(pr.Files) == 0 {
		b.WriteString(s.Muted.Render("no changed files") + "\n")
		return
	}
	var nameW, statusW, addW, delW int
	for _, file := range pr.Files {
		_, name := splitFileDir(file.Path)
		nameW = max(nameW, ansi.StringWidth(name))
		statusW = max(statusW, min(10, ansi.StringWidth(file.Status)))
		addW = max(addW, len(fmt.Sprintf("+%d", file.Additions)))
		delW = max(delW, len(fmt.Sprintf("-%d", file.Deletions)))
	}
	// Layout: indent name [space] status [space] add [space] del [right margin]
	rightW := 1 + statusW + 1 + addW + 1 + delW
	nameW = max(1, min(nameW, innerW-len(fileSummaryIndent)-rightW-1))
	prevDir := ""
	for i, file := range pr.Files {
		dir, name := splitFileDir(file.Path)
		if i == 0 || dir != prevDir {
			// Left-truncate directories so the deepest segments stay visible.
			header := s.Muted.Render(leftTruncatePath(dir, max(1, innerW-1)))
			fmt.Fprintf(b, "%s\n", truncateANSIWidth(header, innerW))
			prevDir = dir
		}
		name = ansi.Truncate(name, nameW, "…")
		namePad := strings.Repeat(" ", max(0, nameW-ansi.StringWidth(name)))
		if i == m.fileIndex {
			name = s.File.Render(name)
		}
		status := truncateANSIWidth(file.Status, statusW)
		status += strings.Repeat(" ", max(0, statusW-ansi.StringWidth(status)))
		add := fmt.Sprintf("%*s", addW, fmt.Sprintf("+%d", file.Additions))
		del := fmt.Sprintf("%*s", delW, fmt.Sprintf("-%d", file.Deletions))
		row := fileSummaryIndent + name + namePad + " " + s.Muted.Render(status) + " " + s.Add.Render(add) + " " + s.Delete.Render(del)
		fmt.Fprintf(b, "%s\n", truncateANSIWidth(row, innerW))
	}
}

func (m Model) writeCommitSummary(b *strings.Builder, pr model.PullRequest, innerW int) {
	s := m.theme.Styles
	if len(pr.Commits) == 0 {
		b.WriteString(s.Muted.Render("no commits") + "\n")
		return
	}
	now := time.Now()
	for i, commit := range newestCommits(pr.Commits) {
		sha := commit.OID
		if len(sha) > 7 {
			sha = sha[:7]
		}
		selected := i == m.commitIndex
		shaStyled := s.File.Render(sha)
		headline := commit.Headline
		if selected {
			headline = s.TitleStrong.Render(headline)
		} else {
			shaStyled = s.Muted.Render(sha)
			headline = s.Muted.Render(headline)
		}
		// Use full-width headline (horizontal scroll handles truncation).
		fmt.Fprintf(b, "%s\n", fmt.Sprintf("%s %s", shaStyled, headline))
		meta := commit.Author
		if !commit.Date.IsZero() {
			meta += " · " + summaryTimestamp(commit.Date, now)
		}
		if selected {
			b.WriteString(s.TitleStrong.Render(meta) + "\n")
		} else {
			b.WriteString(s.Muted.Render(meta) + "\n")
		}
	}
}

func (m Model) writeCommentSummary(b *strings.Builder, pr model.PullRequest, innerW int) {
	s := m.theme.Styles
	if len(pr.Comments) == 0 {
		b.WriteString(s.Muted.Render("no comments") + "\n")
		return
	}
	now := time.Now()
	for i, comment := range newestComments(pr.Comments) {
		kind := comment.Kind
		if kind == "" {
			kind = "comment"
		}
		meta := comment.Author + " · " + kind
		if !comment.CreatedAt.IsZero() {
			meta += " · " + summaryTimestamp(comment.CreatedAt, now)
		}
		b.WriteString(truncatePaneLineWidth(s.File.Render(meta), innerW) + "\n")
		if comment.Path != "" {
			location := comment.Path
			if comment.Line > 0 {
				location += fmt.Sprintf(":%d", comment.Line)
			} else if comment.OriginalLine > 0 {
				location += fmt.Sprintf(":%d (outdated)", comment.OriginalLine)
			}
			if comment.Side == "LEFT" {
				location += " (left)"
			}
			b.WriteString(truncatePaneLineWidth(s.Muted.Render(location), innerW) + "\n")
		}
		for _, line := range strings.Split(m.renderCommentMarkdown(comment.Body), "\n") {
			b.WriteString(truncatePaneLineWidth(line, innerW) + "\n")
		}
		if i < len(pr.Comments)-1 {
			b.WriteByte('\n')
		}
	}
}

func (m Model) diffPanel(width int, height int) string {
	if !m.diffEnabled {
		return m.theme.Styles.Muted.Render("press v to load diff")
	}
	if _, ok := m.current(); !ok {
		return m.theme.Styles.Muted.Render("select PR")
	}
	if m.diffMode == DiffModeFile && m.currentFileContent() == "" {
		return m.theme.Styles.Muted.Render("loading file…")
	}
	if m.summaryMode == SummaryModeCommits {
		if m.renderedCommitDiff == "" {
			return m.theme.Styles.Muted.Render("loading commit diff…")
		}
	} else if m.diff == "" {
		return m.theme.Styles.Muted.Render("loading diff…")
	}
	vpW := m.paneInnerWidth(width, FocusDiff)
	vpH := m.paneInnerHeight(height, FocusDiff)
	return m.renderDiffViewport(m.diffContentWidth(vpW), vpH)
}

func (m Model) diffContentWidth(innerWidth int) int {
	return max(1, innerWidth-diffContentSafetyMargin)
}

func (m Model) renderDiffViewport(width int, height int) string {
	layout := m.diffPane(width)
	if layout.content == "" {
		return ""
	}
	lines := layout.lines
	top := max(0, min(m.diffVP.YOffset, len(lines)))
	bottom := min(len(lines), top+max(1, height))
	visible := lines[top:bottom]
	out := make([]string, max(len(visible), height))
	for i := range out {
		out[i] = strings.Repeat(" ", max(0, width))
	}
	for i, line := range visible {
		xOffset := m.diffXOffset
		if m.cfg.Wrap.Diff {
			xOffset = 0
		}
		cut := ansi.Cut(line, xOffset, xOffset+width)
		out[i] = padANSIWidth(truncateANSIWidth(cut, width), width)
	}
	return strings.Join(out, "\n")
}

func (m Model) renderFullFileContent(content string, marks diffMarks) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	lineNoW := len(fmt.Sprintf("%d", len(lines)))
	s := m.theme.Styles
	var b strings.Builder
	for i, line := range lines {
		lineNo := i + 1
		for _, deleted := range marks.deletedBefore[lineNo] {
			prefix := s.Muted.Render(fmt.Sprintf("%*s │ ", lineNoW, ""))
			b.WriteString(s.Delete.Render("-") + prefix + s.Delete.Render(deleted) + "\n")
		}
		prefix := s.Muted.Render(fmt.Sprintf("%*d │ ", lineNoW, lineNo))
		if marks.added[lineNo] {
			b.WriteString(s.Add.Render("+") + prefix + s.Add.Render(line))
		} else {
			b.WriteString(" " + prefix + line)
		}
		if i < len(lines)-1 || len(marks.deletedBefore[len(lines)+1]) > 0 {
			b.WriteByte('\n')
		}
	}
	for i, deleted := range marks.deletedBefore[len(lines)+1] {
		prefix := s.Muted.Render(fmt.Sprintf("%*s │ ", lineNoW, ""))
		b.WriteString(s.Delete.Render("-") + prefix + s.Delete.Render(deleted))
		if i < len(marks.deletedBefore[len(lines)+1])-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

var emojiShortcodePattern = regexp.MustCompile(`:[A-Za-z0-9_+\-]+:`)

func stripEmojiShortcodes(s string) string {
	s = emojiShortcodePattern.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

var diffHunkHeaderPattern = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

type diffMarks struct {
	added         map[int]bool
	deletedBefore map[int][]string
}

func diffFileMarks(diff string) diffMarks {
	marks := diffMarks{added: make(map[int]bool), deletedBefore: make(map[int][]string)}
	newLine := 0
	for _, line := range strings.Split(diff, "\n") {
		if matches := diffHunkHeaderPattern.FindStringSubmatch(line); len(matches) == 2 {
			fmt.Sscanf(matches[1], "%d", &newLine)
			continue
		}
		if newLine == 0 || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			marks.added[newLine] = true
			newLine++
		case strings.HasPrefix(line, "-"):
			marks.deletedBefore[newLine] = append(marks.deletedBefore[newLine], strings.TrimPrefix(line, "-"))
		case strings.HasPrefix(line, `\`):
			// "\ No newline at end of file" marker; not file content.
		default:
			newLine++
		}
	}
	return marks
}

func padANSIWidth(s string, width int) string {
	missing := width - ansi.StringWidth(s)
	if missing <= 0 {
		return s
	}
	return s + strings.Repeat(" ", missing)
}

// truncateANSIWidth clips a string with ANSI escape codes to maxW visible columns.
// No ellipsis. Truncates by grapheme so emoji sequences stay intact.
func truncateANSIWidth(s string, maxW int) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			b.WriteRune(r)
			continue
		}
		if inEscape {
			b.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		if r == '\t' {
			spaces := tabStopWidth - ansi.StringWidth(b.String())%tabStopWidth
			b.WriteString(strings.Repeat(" ", spaces))
			continue
		}
		if r == '\r' || (r < ' ' && r != '\n') {
			continue
		}
		b.WriteRune(r)
	}
	out := ansi.Truncate(b.String(), max(0, maxW), "")
	// Reset ANSI attributes so truncation doesn't bleed color into next line.
	if strings.Contains(out, "\x1b[") {
		out += "\x1b[0m"
	}
	return out
}

// leftTruncatePath keeps the longest trailing path that fits within maxW terminal
// columns, prefixing truncated paths with "…" without splitting graphemes.
func leftTruncatePath(path string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	width := ansi.StringWidth(path)
	if width <= maxW {
		return path
	}
	for width > maxW-1 {
		cluster, clusterWidth := ansi.FirstGraphemeCluster(path, ansi.GraphemeWidth)
		path = path[len(cluster):]
		width -= clusterWidth
	}
	return "…" + path
}

// renderModalBox renders a centered modal box (border + padding) without
// placing it on a backdrop. It is composited over the dimmed body layout by
// overlayModal, matching the command palette's dimmed-background presentation.
func (m Model) renderModalBox(content string) string {
	width := min(72, max(32, m.width-4))
	style := m.theme.Styles.Focused.Width(width).Padding(1, 2)
	return style.Render(content)
}

func (m Model) approvePanel() string {
	pr, ok := m.current()
	if !ok {
		return "no PR selected"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Approve %s #%d?\n\n", pr.Repo, pr.Number)
	b.WriteString("\n" + m.actionAlias("approve-confirm") + " approve  " + m.actionAlias("approve-cancel") + " cancel")
	return b.String()
}

func (m Model) commentPanel() string {
	pr, ok := m.current()
	if !ok {
		return "no PR selected"
	}
	return fmt.Sprintf("Comment on %s #%d\n\n%s\n\n%s submit  %s cancel", pr.Repo, pr.Number, m.comment, m.actionAlias("comment-submit"), m.actionAlias("comment-cancel"))
}

func (m Model) commandView() string {
	if m.width > 0 {
		return m.layoutView()
	}
	return fmt.Sprintf(":%s\n\n%s execute  %s cancel\n", m.commandInput, m.actionAlias("command-submit"), m.actionAlias("command-cancel"))
}

func (m Model) commandPanel() string {
	s := m.theme.Styles
	var b strings.Builder

	if m.mode == ModeConfirmCommand {
		pr, ok := m.current()
		if m.pendingAction.ID == "toggle-automerge" {
			pr, ok = m.pendingPR, true
		}
		if !ok {
			return "no PR selected"
		}
		name := confirmationActionLabel(m.pendingAction)
		fmt.Fprintf(&b, "%s on #%d?\n\n", name, pr.Number)
		if m.pendingAction.ID == "toggle-automerge" {
			b.WriteString("GitHub may merge immediately if requirements pass.\n\n")
		}
		b.WriteString(m.actionAlias("command-confirm") + " confirm  " + m.actionAlias("command-cancel-confirmation") + " cancel")
		return b.String()
	}

	// Command input line with cursor.
	fmt.Fprintf(&b, ":%s_\n\n", m.commandInput)

	matches := m.actions.matchingCommands(m.commandInput)
	if len(matches) == 0 {
		b.WriteString(s.Muted.Render("no matching commands"))
	} else {
		for i, match := range matches {
			match.action = m.selectedPRAction(match.action)
			name := truncateANSIWidth(match.alias, 18)
			nameStyled := s.TitleStrong.Render(":" + name)
			width := min(66, m.width-12) - len(name) - 6
			descStyled := s.Muted.Render(truncateANSIWidth(actionDisplayLabel(match.action), max(1, width)))
			confirmMark := ""
			if match.action.Confirmation != confirmationNone {
				confirmMark = s.Muted.Render(" [?]")
			}
			if i == m.commandCursor {
				cursor := s.File.Render(">")
				fmt.Fprintf(&b, "%s %s  %s%s\n", cursor, nameStyled, descStyled, confirmMark)
			} else {
				fmt.Fprintf(&b, "  %s  %s%s\n", nameStyled, descStyled, confirmMark)
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(s.Muted.Render(m.actionAlias("command-submit") + " execute  " + m.actionAlias("command-cancel") + " cancel"))
	return b.String()
}

func (m Model) settingsPanel() string {
	s := m.theme.Styles
	var b strings.Builder
	b.WriteString(s.Title.Render("Inbox settings"))
	b.WriteString("\n\n")

	draftsValue := "show drafts"
	if m.hideDrafts {
		draftsValue = "hide drafts"
	}
	rows := []struct {
		label string
		value string
	}{
		{"Draft PRs", draftsValue},
		{"Sort", m.sortMode.String()},
	}
	for i, row := range rows {
		cursor := "  "
		lineStyle := s.Muted
		if i == m.settingsCursor {
			cursor = "› "
			lineStyle = s.TitleStrong
		}
		b.WriteString(cursor)
		b.WriteString(lineStyle.Render(row.label))
		b.WriteString(": ")
		b.WriteString(row.value)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(s.Muted.Render(m.actionAlias("settings-next") + "/" + m.actionAlias("settings-prev") + " select  " + m.actionAlias("settings-toggle") + " toggle  " + m.actionAlias("settings-close") + " close"))
	return b.String()
}

func (m Model) colorschemePanel() string {
	s := m.theme.Styles
	palettes := uitheme.AllPalettes()
	var b strings.Builder
	b.WriteString(s.Title.Render("Color scheme"))
	b.WriteString("\n\n")
	for i, p := range palettes {
		cursor := "  "
		lineStyle := s.Muted
		if i == m.colorschemeIndex {
			cursor = "› "
			lineStyle = s.TitleStrong
		}
		suffix := ""
		if strings.EqualFold(p.Name, m.cfg.Theme) || (m.cfg.Theme == "" && strings.EqualFold(p.Name, "dark")) {
			suffix = s.Muted.Render("  (active)")
		}
		b.WriteString(cursor)
		b.WriteString(lineStyle.Render(p.Name))
		b.WriteString(suffix)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(s.Muted.Render(m.actionAlias("colorscheme-next") + "/" + m.actionAlias("colorscheme-prev") + " move  " + m.actionAlias("colorscheme-confirm") + " select  " + m.actionAlias("colorscheme-cancel") + " cancel"))
	return b.String()
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
	return s[:width]
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// csiNonSGRPattern matches CSI escape sequences whose final byte is anything
// other than 'm' (the SGR/color terminator). Tools like delta emit non-SGR
// sequences such as ESC[0K (Erase in Line) that measure as zero width but are
// executed by the terminal, corrupting column accounting and causing visible
// wrapping/misalignment. We strip these while preserving color (SGR) codes.
var csiNonSGRPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[A-LN-Za-ln-z~]`)

// stripNonSGR removes non-SGR CSI escape sequences (e.g. ESC[0K) while leaving
// SGR color codes intact.
func stripNonSGR(s string) string {
	return csiNonSGRPattern.ReplaceAllString(s, "")
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
