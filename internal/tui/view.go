package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/model"
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
	b.WriteString("\nj/k navigate  enter inspect  a approve, y confirm  c comment  r refresh  v toggle diff  1/2/3/4 summary\n")
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
	if pr.Triage.AI != nil {
		writeAINotes(&b, pr.Triage)
	}
	b.WriteString("\nFiles:\n")
	for _, file := range pr.Files {
		fmt.Fprintf(&b, "%-8s %s (+%d -%d)\n", file.Status, file.Path, file.Additions, file.Deletions)
	}
	b.WriteString("\na approve, y confirm  c comment  r re-run analysis  v toggle diff  V full diff pager  1/2/3/4 summary  esc inbox\n")
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
	if pr.Triage.AI != nil {
		writeAINotes(&b, pr.Triage)
	}
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

func (m Model) settingsView() string {
	if m.width > 0 {
		return m.layoutView()
	}
	return m.settingsPanel() + "\nesc back  q quit\n"
}

func helpText() string {
	return `zappr help

Keys:
  j/k        move / scroll
  tab        cycle pane focus
  o          open PR in browser
  f          filters / sort settings
  enter      inspect PR
  v          toggle diff pane (load/hide diff)
  V          open full diff pager
  1/2/3/4    summary: files / commits / comments / atlantis
  shift+up/down  prev/next file
  s          side-by-side diff
  x          expand selected file / diff hunks
  r          refresh (inbox) / re-run AI analysis (detail)
  a then y   approve
  shift+a    approve (no confirmation)
  c          comment
  :          vim-style commands (:ap :aa)
  ? esc      toggle help / back
  q          quit`
}

func (m Model) helpPanel() string {
	return helpText()
}

func (m Model) layoutView() string {
	header := m.headerView()
	bodyHeight := max(4, m.height-chromeLines)
	fit := func(s string) string { return fitScreen(s, m.width, m.height) }

	if m.mode == ModeConfirmApprove {
		return fit(lipgloss.JoinVertical(lipgloss.Left, header, m.centerModal(m.approvePanel(), bodyHeight)))
	}
	if m.mode == ModeComment {
		return fit(lipgloss.JoinVertical(lipgloss.Left, header, m.centerModal(m.commentPanel(), bodyHeight)))
	}
	if m.mode == ModeHelp {
		return fit(lipgloss.JoinVertical(lipgloss.Left, header, m.centerModal(m.helpPanel(), bodyHeight)))
	}
	if m.mode == ModeSettings {
		return fit(lipgloss.JoinVertical(lipgloss.Left, header, m.centerModal(m.settingsPanel(), bodyHeight)))
	}

	var body string
	if m.width >= minWideWidth {
		body = m.wideLayout(bodyHeight)
	} else if m.width >= minMediumWidth {
		body = m.mediumLayout(bodyHeight)
	} else {
		body = m.compactLayout(bodyHeight)
	}

	if m.mode == ModeCommand || m.mode == ModeConfirmCommand {
		modal := m.renderCommandModal()
		body = overlayModal(body, modal, m.width, bodyHeight, m.selectedInboxPreserveRanges(bodyHeight))
	}

	return fit(lipgloss.JoinVertical(lipgloss.Left, header, body))
}

// renderCommandModal renders the command palette box (border + padding + content).
func (m Model) renderCommandModal() string {
	width := min(60, max(32, m.width-8))
	style := m.theme.Styles.Focused.Width(width).Padding(1, 2)
	return style.Render(m.commandPanel())
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
		selectedLine := m.selected * 3
		yOffset := m.selectedInboxYOffset(contentH)
		for _, itemLine := range []int{selectedLine, selectedLine + 1} {
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
	yOffset := m.inboxVP.YOffset
	totalLines := max(0, len(m.prs)*3-1)
	if totalLines <= contentH {
		return 0
	}
	selectedLine := m.selected * 3
	selectedBottom := selectedLine + 1
	if selectedLine < yOffset {
		yOffset = selectedLine
	} else if selectedBottom >= yOffset+contentH {
		yOffset = selectedBottom - contentH + 1
	}
	return yOffset
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
	// AI analysis progress is shown in the Summary pane instead.
	if m.statusActive && !m.aiAnalyzing {
		if m.statusErr {
			parts = append(parts, m.theme.Styles.Error.Render("✗ "+m.statusText))
		} else {
			parts = append(parts, strings.TrimRight(m.statusSpinner.View(), " ")+" "+m.theme.Styles.Pending.Render(m.statusText))
		}
	} else if m.statusActive && m.aiAnalyzing && m.statusErr {
		parts = append(parts, m.theme.Styles.Error.Render("✗ "+m.statusText))
	} else if m.loading && !m.aiAnalyzing {
		parts = append(parts, m.theme.Styles.Pending.Render("loading"))
	} else if m.error != "" {
		parts = append(parts, m.theme.Styles.Error.Render(m.error))
	}
	left := strings.Join(parts, " ")
	right := m.footerShortcuts([]shortcut{{"?", "help"}, {"q", "quit"}})
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	line := left + strings.Repeat(" ", gap) + right
	return railView(m.theme.Styles.Header.Width(m.width).Render(line), m.width)
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
			keys = []shortcut{{"v", "diff"}, {"a", "approve"}, {"c", "comment"}, {"r", "AI review"}, {"esc", "inbox"}}
		}
	default:
		keys = []shortcut{{"f", "filter"}, {"o", "open"}, {"v", "diff"}, {"a", "approve"}, {"c", "comment"}, {"r", "refresh"}}
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
	right := m.pane(m.titledPane("Diff / Details", m.diffScrollPct()), m.diffPanel(diffW, height), diffW, height, FocusDiff)
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
		inboxH = max(8, height*80/100)
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
	bottom := m.pane(m.titledPane("Diff / Details", m.diffScrollPct()), m.diffPanel(m.width, bottomH), m.width, bottomH, FocusDiff)
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
		m.summaryModeTitle(1, SummaryModeFiles),
		m.summaryModeTitle(2, SummaryModeCommits),
		m.summaryModeTitle(3, SummaryModeComments),
		m.summaryModeTitle(4, SummaryModeAtlantis),
	}
	return "Summary " + strings.Join(parts, " ")
}

func (m Model) summaryModeTitle(number int, mode SummaryMode) string {
	label := fmt.Sprintf("%d", number)
	if m.summaryMode == mode {
		return m.theme.Styles.TitleStrong.Render(label)
	}
	return m.theme.Styles.Muted.Render(label)
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
	if w <= 0 {
		return s
	}
	logicalLines := strings.Split(s, "\n")
	out := make([]string, 0, len(logicalLines))
	wrapper := lipgloss.NewStyle().Width(w).MaxWidth(w)
	for _, line := range logicalLines {
		if lipgloss.Width(line) <= w {
			out = append(out, line)
			continue
		}
		// Wrap via lipgloss: it word-wraps at w, producing multiple visual lines.
		rendered := wrapper.Render(line)
		// Strip the trailing padding spaces lipgloss adds to fill Width.
		for _, wl := range strings.Split(rendered, "\n") {
			out = append(out, strings.TrimRight(wl, " "))
		}
	}
	return strings.Join(out, "\n")
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
	s := m.theme.Styles
	innerW := max(1, width-paneHFrame)
	selectedStyle := lipgloss.NewStyle()
	var b strings.Builder
	// Track line number where selected item starts (for viewport scroll sync).
	lineCount := 0
	selectedLine := 0
	selectedHeight := 0
	for i, pr := range m.prs {
		if i == m.selected {
			selectedLine = lineCount
			selectedHeight = 2
		}
		selected := i == m.selected

		// Line 1: title
		title := truncateANSIWidth(pr.Title, innerW)
		titleStyle := s.Muted
		if selected {
			titleStyle = s.TitleStrong
		}
		titleStr := selectedStyle.Render(titleStyle.Render(title))
		fmt.Fprintf(&b, "%s\n", truncatePaneLineWidth(titleStr, innerW))
		lineCount++

		// Line 2: #N · author · CI · review (consolidated)
		dot := s.Muted.Render(" · ")
		metaParts := []string{fmt.Sprintf("#%d", pr.Number), pr.Author}
		if pr.Draft {
			metaParts = append(metaParts, "[draft]")
		}
		metaBase := strings.Join(metaParts, stripANSI(dot))
		ciStr := m.theme.CIStatus(pr.CIStatus)
		reviewStr := m.theme.ReviewStatus(pr.ReviewDecision)

		var metaLine string
		if selected {
			meta := s.TitleStrong.Render(metaBase)
			metaLine = meta + dot + ciStr
			if reviewStr != "" {
				metaLine += dot + reviewStr
			}
		} else {
			// Inactive: strip all color, render muted
			ci := stripANSI(ciStr)
			full := metaBase + stripANSI(dot) + ci
			if reviewStr != "" {
				full += stripANSI(dot) + stripANSI(reviewStr)
			}
			metaLine = s.Muted.Render(full)
		}
		fmt.Fprintf(&b, "%s\n", truncatePaneLineWidth(selectedStyle.Render(metaLine), innerW))
		lineCount++

		if i < len(m.prs)-1 {
			divider := s.Muted.Render(truncateANSIWidth(strings.Repeat("─", innerW), innerW))
			fmt.Fprintf(&b, "%s\n", divider)
			lineCount++
		}
	}
	vpH := max(1, height-paneFrame)
	vp := m.inboxVP
	vp.Width = innerW
	vp.Height = vpH
	vp.SetContent(fitPaneContentWidth(b.String(), innerW))
	// Scroll so that the selected item is always visible (only when overflowing).
	if vp.TotalLineCount() > vpH {
		selectedBottom := selectedLine + selectedHeight - 1
		if selectedLine < vp.YOffset {
			vp.SetYOffset(selectedLine)
		} else if selectedBottom >= vp.YOffset+vpH {
			vp.SetYOffset(selectedBottom - vpH + 1)
		}
	}
	return vp.View()
}

func (m Model) summaryPanel(width int, height int) string {
	pr, ok := m.current()
	if !ok {
		return m.theme.Styles.Muted.Render("select PR")
	}
	innerW := max(1, width-paneHFrame)
	rawContent := m.summaryContent(pr, innerW)
	// In commits mode apply horizontal scrolling: cut each line by x-offset.
	if m.summaryMode == SummaryModeCommits && m.summaryXOffset > 0 {
		lines := strings.Split(rawContent, "\n")
		for i, line := range lines {
			lines[i] = truncateANSIWidth(ansi.Cut(line, m.summaryXOffset, m.summaryXOffset+innerW), innerW)
		}
		rawContent = strings.Join(lines, "\n")
	} else {
		rawContent = fitPaneContentWidth(rawContent, innerW)
	}
	vp := m.summaryVP
	vp.Width = innerW
	vp.Height = max(1, height-paneFrame)
	vp.SetContent(rawContent)
	return vp.View()
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
	s := m.theme.Styles
	var b strings.Builder

	switch m.summaryMode {
	case SummaryModeCommits:
		m.writeCommitSummary(&b, pr, innerW)
	case SummaryModeComments:
		m.writeCommentSummary(&b, pr, innerW)
	case SummaryModeAtlantis:
		m.writeAtlantisSummary(&b, pr, innerW)
	default:
		m.writeFileSummary(&b, pr, innerW)
	}
	if m.aiAnalyzing {
		b.WriteByte('\n')
		b.WriteString(m.statusSpinner.View() + s.Pending.Render("Running AI analysis"))
		b.WriteByte('\n')
	} else if pr.Triage.AI != nil {
		writeStyledAINotes(&b, m, pr.Triage)
	} else {
		b.WriteByte('\n')
		b.WriteString(s.Muted.Render("press r to run AI review"))
		b.WriteByte('\n')
	}

	return b.String()
}

func (m Model) writeFileSummary(b *strings.Builder, pr model.PullRequest, innerW int) {
	s := m.theme.Styles
	for i, file := range pr.Files {
		status := truncateANSIWidth(s.Muted.Render(file.Status), 10)
		add := s.Add.Render(fmt.Sprintf("+%d", file.Additions))
		del := s.Delete.Render(fmt.Sprintf("-%d", file.Deletions))
		// Right-align: "add del" at innerW-1 (1 space from right edge).
		rightPart := add + " " + del
		rightW := lipgloss.Width(stripANSI(add)) + 1 + lipgloss.Width(stripANSI(del))
		statusW := lipgloss.Width(stripANSI(status))
		// pathW: available width for the path column.
		// Layout: path [gap] status [space] rightPart [space from edge]
		// Total used = statusW + 1 + rightW + 1 (right margin)
		rightTotalW := statusW + 1 + rightW + 1
		pathW := max(1, innerW-rightTotalW-1) // -1 for space between path and status
		// Left-truncate so the filename (rightmost segment) stays visible.
		truncPath := leftTruncatePath(file.Path, pathW)
		var pathStr string
		if i == m.fileIndex {
			pathStr = s.File.Render(truncPath)
		} else {
			pathStr = truncPath
		}
		pathVisW := lipgloss.Width(truncPath)
		// Right-align paths within the path column: left-pad so filename ends
		// flush against the status column.
		leftPad := max(0, pathW-pathVisW)
		row := strings.Repeat(" ", leftPad) + pathStr + " " + status + " " + rightPart
		fmt.Fprintf(b, "%s\n", truncateANSIWidth(row, innerW))
	}
	if len(pr.Files) == 0 {
		b.WriteString(s.Muted.Render("no changed files") + "\n")
	}
}

func (m Model) writeCommitSummary(b *strings.Builder, pr model.PullRequest, innerW int) {
	s := m.theme.Styles
	if len(pr.Commits) == 0 {
		b.WriteString(s.Muted.Render("no commits") + "\n")
		return
	}
	for i, commit := range pr.Commits {
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
			meta += " · " + commit.Date.Format("2006-01-02")
		}
		b.WriteString(s.Muted.Render(meta) + "\n")
	}
}

func (m Model) writeCommentSummary(b *strings.Builder, pr model.PullRequest, innerW int) {
	s := m.theme.Styles
	if len(pr.Comments) == 0 {
		b.WriteString(s.Muted.Render("no conversation comments") + "\n")
		return
	}
	for i, comment := range pr.Comments {
		meta := comment.Author
		if !comment.CreatedAt.IsZero() {
			meta += " · " + comment.CreatedAt.Format("2006-01-02 15:04")
		}
		b.WriteString(truncatePaneLineWidth(s.File.Render(meta), innerW) + "\n")
		for _, line := range strings.Split(comment.Body, "\n") {
			b.WriteString(truncatePaneLineWidth(stripEmojiShortcodes(line), innerW) + "\n")
		}
		if i < len(pr.Comments)-1 {
			b.WriteByte('\n')
		}
	}
}

func (m Model) writeAtlantisSummary(b *strings.Builder, pr model.PullRequest, innerW int) {
	s := m.theme.Styles
	if pr.AtlantisPlan == nil {
		b.WriteString(s.Muted.Render("no Atlantis plan output found") + "\n")
		return
	}
	plan := pr.AtlantisPlan
	b.WriteString(s.Subtitle.Render("Atlantis plan") + "\n")
	if plan.Summary != "" {
		b.WriteString(truncatePaneLineWidth(plan.Summary, innerW) + "\n")
	}
	meta := plan.Author
	if !plan.CreatedAt.IsZero() {
		meta += " · " + plan.CreatedAt.Format("2006-01-02 15:04")
	}
	if meta != "" {
		b.WriteString(truncatePaneLineWidth(s.Muted.Render(meta), innerW) + "\n")
	}
	if !plan.NoChanges {
		counts := fmt.Sprintf("+%d ~%d -%d", plan.Add, plan.Change, plan.Destroy)
		b.WriteString(truncatePaneLineWidth(s.Add.Render(counts), innerW) + "\n")
	}

	changed, noChanges, affected := splitAtlantisLayers(plan.Layers)
	if len(changed) == 0 && len(noChanges) == 0 && len(affected) == 0 {
		if plan.NoChanges {
			noChanges = []model.AtlantisLayer{{Name: "overall", NoChanges: true}}
		} else {
			affected = []model.AtlantisLayer{{Name: "overall", Add: plan.Add, Change: plan.Change, Destroy: plan.Destroy, Unknown: true}}
		}
	}
	writeAtlantisLayerGroup(b, m, "Changes", changed, innerW)
	writeAtlantisLayerGroup(b, m, "Affected layers", affected, innerW)
	writeAtlantisLayerGroup(b, m, "No changes", noChanges, innerW)
}

func splitAtlantisLayers(layers []model.AtlantisLayer) (changed, noChanges, affected []model.AtlantisLayer) {
	for _, layer := range layers {
		hasCounts := layer.Add != 0 || layer.Change != 0 || layer.Destroy != 0
		switch {
		case hasCounts:
			changed = append(changed, layer)
		case layer.NoChanges:
			noChanges = append(noChanges, layer)
		default:
			affected = append(affected, layer)
		}
	}
	return changed, noChanges, affected
}

func writeAtlantisLayerGroup(b *strings.Builder, m Model, title string, layers []model.AtlantisLayer, innerW int) {
	if len(layers) == 0 {
		return
	}
	s := m.theme.Styles
	b.WriteByte('\n')
	b.WriteString(s.Subtitle.Render(title) + "\n")
	for _, layer := range layers {
		line := "- " + layer.Name
		if layer.Add != 0 || layer.Change != 0 || layer.Destroy != 0 {
			line += fmt.Sprintf(" (+%d ~%d -%d)", layer.Add, layer.Change, layer.Destroy)
		}
		if title == "Changes" {
			b.WriteString(truncatePaneLineWidth(s.Add.Render(line), innerW) + "\n")
		} else {
			b.WriteString(truncatePaneLineWidth(line, innerW) + "\n")
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
	content := m.currentDiffContent()
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	top := max(0, min(m.diffVP.YOffset, len(lines)))
	bottom := min(len(lines), top+max(1, height))
	visible := lines[top:bottom]
	if len(visible) < height {
		visible = append(visible, make([]string, height-len(visible))...)
	}
	out := make([]string, len(visible))
	for i, line := range visible {
		cut := ansi.Cut(line, m.diffXOffset, m.diffXOffset+width)
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
// No ellipsis. Uses rune widths so wide glyphs stay inside border.
func truncateANSIWidth(s string, maxW int) string {
	maxW = max(0, maxW)
	var b strings.Builder
	inEscape := false
	width := 0
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
			spaces := tabStopWidth - width%tabStopWidth
			if spaces == 0 {
				spaces = tabStopWidth
			}
			if width+spaces > maxW {
				break
			}
			b.WriteString(strings.Repeat(" ", spaces))
			width += spaces
			continue
		}
		if r == '\r' || (r < ' ' && r != '\n') {
			continue
		}
		w := lipgloss.Width(string(r))
		if width+w > maxW {
			break
		}
		b.WriteRune(r)
		width += w
	}
	// Reset ANSI attributes so truncation doesn't bleed color into next line.
	if strings.Contains(b.String(), "\x1b[") {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// leftTruncatePath truncates a plain file path to fit within maxW columns by
// removing leading path segments and prefixing with "…". If the full path fits,
// it is returned unchanged. The result is always a plain string (no ANSI).
func leftTruncatePath(path string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if len(path) <= maxW {
		return path
	}
	// Split on "/" and rebuild from the right until it fits.
	segments := strings.Split(path, "/")
	for i := 1; i < len(segments); i++ {
		candidate := "…/" + strings.Join(segments[i:], "/")
		if len(candidate) <= maxW {
			return candidate
		}
	}
	// Even the last segment doesn't fit — hard-truncate from the right with ellipsis.
	last := segments[len(segments)-1]
	if maxW <= 1 {
		return "…"
	}
	if len(last) <= maxW-1 {
		return "…" + last
	}
	return "…" + last[len(last)-(maxW-1):]
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
	if pr.Triage.AI != nil {
		writeStyledAINotes(&b, m, pr.Triage)
	}
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

func (m Model) commandView() string {
	if m.width > 0 {
		return m.layoutView()
	}
	return fmt.Sprintf(":%s\n\nenter execute  esc cancel\n", m.commandInput)
}

func (m Model) commandPanel() string {
	s := m.theme.Styles
	var b strings.Builder

	if m.mode == ModeConfirmCommand {
		pr, ok := m.current()
		if !ok {
			return "no PR selected"
		}
		fmt.Fprintf(&b, "Run :%s on %s #%d?\n\n", m.pendingCmd.Name, pr.Repo, pr.Number)
		fmt.Fprintf(&b, "%s\n\n", s.Muted.Render(m.pendingCmd.Description))
		if isAtlantisApply(m.pendingCmd) && (!reviewApproved(pr.ReviewDecision) || !ciPassing(pr.CIStatus)) {
			b.WriteString(s.Error.Render("Warning: applying before PR is approved and CI is passing.") + "\n")
			b.WriteString(s.Muted.Render("Approval: ") + m.theme.ReviewStatus(pr.ReviewDecision) + "\n")
			b.WriteString(s.Muted.Render("CI: ") + m.theme.CIStatus(pr.CIStatus) + "\n\n")
		}
		b.WriteString("y confirm  n/esc cancel")
		return b.String()
	}

	// Command input line with cursor.
	fmt.Fprintf(&b, ":%s_\n\n", m.commandInput)

	matches := matchingCommands(m.commandInput)
	if len(matches) == 0 {
		b.WriteString(s.Muted.Render("no matching commands"))
	} else {
		for i, cmd := range matches {
			nameStyled := s.TitleStrong.Render(":" + cmd.Name)
			descStyled := s.Muted.Render(cmd.Description)
			confirmMark := ""
			if cmd.RequireConfirm {
				confirmMark = s.Muted.Render(" [confirm]")
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
	b.WriteString(s.Muted.Render("enter execute  esc cancel"))
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
	b.WriteString(s.Muted.Render("j/k select  enter/space toggle  esc close"))
	return b.String()
}

func writeAINotes(b *strings.Builder, triage model.Triage) {
	if triage.AI == nil {
		b.WriteString("\nAI: not run (advisory only)\n")
		return
	}
	b.WriteString("\nAI review\n")
	if triage.AI.Error != "" {
		fmt.Fprintf(b, "AI error: %s\n", triage.AI.Error)
		return
	}
	fmt.Fprintf(b, "- %s\n", triage.AI.Summary)
	writeList(b, "Risk", triage.AI.RiskNotes)
	writeList(b, "AI focus", triage.AI.SuggestedFocus)
}

func writeStyledAINotes(b *strings.Builder, m Model, triage model.Triage) {
	if triage.AI == nil {
		b.WriteString("\n" + m.theme.Styles.Muted.Render("AI: not run (advisory only)") + "\n")
		return
	}
	b.WriteString("\n" + m.theme.Styles.Subtitle.Render("AI review") + "\n")
	if triage.AI.Error != "" {
		b.WriteString(m.theme.Styles.Error.Render("AI error: "+triage.AI.Error) + "\n")
		return
	}
	fmt.Fprintf(b, "- %s\n", triage.AI.Summary)
	writeStyledList(b, m, "Risk", triage.AI.RiskNotes)
	writeStyledList(b, m, "AI focus", triage.AI.SuggestedFocus)
}

func writeStyledList(b *strings.Builder, m Model, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", m.theme.Styles.Subtitle.Render(title))
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
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
