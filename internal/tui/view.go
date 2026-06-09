package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/senilio/zappr/internal/model"
)

const minWideWidth = 110
const minMediumWidth = 76
const paneFrame = 2   // border top + border bottom (lipgloss frame vertical)
const paneHFrame = 4  // border (2) + padding (2) horizontal
const chromeLines = 2 // header + footer
const truncateSafetyMargin = 2

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

	fmt.Fprintf(&b, "%-3s %-8s %-8s %s\n", "", "PR", "CI", "Title")
	for i, pr := range m.prs {
		skip := ""
		if pr.Skipped {
			skip = " [skipped]"
		}
		line := fmt.Sprintf("#%-7d %-8s %s%s", pr.Number, pr.CIStatus, pr.Title, skip)
		if i == m.selected {
			if !m.theme.NoColor {
				line = lipgloss.NewStyle().Background(m.theme.Palette.Focus).Foreground(m.theme.Palette.Panel).Render(line)
			}
		}
		fmt.Fprintf(&b, "%s\n", line)
	}
	b.WriteString("\nj/k navigate  enter inspect  a approve, y confirm  c comment  s toggle skip  r refresh  v toggle diff  ? help  q quit\n")
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
	b.WriteString("\na approve, y confirm  c comment  s skip  r re-run analysis  v toggle diff  V full diff pager  esc inbox  ? help  q quit\n")
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

func helpText() string {
	return `zappr help

Keys:
  j/k        move / scroll
  tab        cycle pane focus
  o          cycle sort
  enter      inspect PR
  v          toggle diff pane (load/hide diff)
  V          open full diff pager
  shift+up/down  prev/next file
  s          toggle skip (inbox) / side-by-side (diff)
  r          refresh (inbox) / re-run AI analysis (detail)
  a then y   approve
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
	bodyHeight := max(4, m.height-chromeLines)

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
			parts = append(parts, m.statusSpinner.View()+" "+m.theme.Styles.Pending.Render(m.statusText))
		}
	} else if m.loading {
		parts = append(parts, m.theme.Styles.Pending.Render("loading"))
	} else if m.error != "" {
		parts = append(parts, m.theme.Styles.Error.Render(m.error))
	}
	return railView(m.theme.Styles.Header.Width(m.width).Render(strings.Join(parts, " ")), m.width)
}

func (m Model) footerView() string {
	if m.width < minMediumWidth {
		return railView(m.theme.Styles.Footer.Width(m.width).Render(m.footerShortcuts([]shortcut{{"v", "diff"}, {"?", "help"}, {"q", "quit"}})), m.width)
	}
	var keys []shortcut
	switch m.focus {
	case FocusDiff:
		keys = []shortcut{{"j/k", "scroll"}, {"left/right", "pan"}, {"n/p", "file"}, {"s", "side-by-side"}, {"V", "pager"}, {"esc", "back"}, {"?", "help"}, {"q", "quit"}}
	case FocusSummary:
		keys = []shortcut{{"j/k", "scroll"}, {"v", "diff"}, {"a", "approve"}, {"c", "comment"}, {"s", "skip"}, {"r", "AI review"}, {"esc", "inbox"}, {"?", "help"}, {"q", "quit"}}
	default:
		keys = []shortcut{{"o", "sort"}, {"v", "diff"}, {"a", "approve"}, {"c", "comment"}, {"s", "skip"}, {"r", "refresh"}, {"?", "help"}, {"q", "quit"}}
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
	gap := 1
	leftW := max(28, m.width*40/100)
	diffW := max(30, m.width-leftW-gap)
	inboxH, summaryH := m.leftColumnHeights(height)
	left := lipgloss.JoinVertical(lipgloss.Left,
		m.pane(m.titledPane(m.inboxTitle(), m.inboxScrollPct()), m.inboxPanel(leftW, inboxH), leftW, inboxH, FocusInbox),
		m.pane(m.titledPane("Summary", m.summaryScrollPct()), m.summaryPanel(leftW, summaryH), leftW, summaryH, FocusSummary),
	)
	right := m.pane(m.titledPane("Diff / Details", m.diffScrollPct()), m.diffPanel(diffW, height), diffW, height, FocusDiff)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
}

// leftColumnHeights returns stable pane heights for left column.
func (m Model) leftColumnHeights(height int) (inboxH, summaryH int) {
	inboxH = max(8, height*70/100)
	summaryH = max(4, height-inboxH)
	return
}

func (m Model) mediumLayout(height int) string {
	topH := max(8, height/2)
	bottomH := max(6, height-topH)
	top := lipgloss.JoinHorizontal(lipgloss.Top,
		m.pane(m.titledPane(m.inboxTitle(), m.inboxScrollPct()), m.inboxPanel(m.width/2-1, topH), m.width/2-1, topH, FocusInbox),
		" ",
		m.pane(m.titledPane("Summary", m.summaryScrollPct()), m.summaryPanel(m.width/2, topH), m.width/2, topH, FocusSummary),
	)
	bottom := m.pane(m.titledPane("Diff / Details", m.diffScrollPct()), m.diffPanel(m.width, bottomH), m.width, bottomH, FocusDiff)
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (m Model) compactLayout(height int) string {
	if m.mode == ModeDiff {
		return m.pane(m.titledPane("Diff", m.diffScrollPct()), m.diffPanel(m.width, height), m.width, height, FocusDiff)
	}
	if m.mode == ModeDetail {
		return m.pane(m.titledPane("Details", m.summaryScrollPct()), m.summaryPanel(m.width, height), m.width, height, FocusSummary)
	}
	return m.pane(m.titledPane(m.inboxTitle(), m.inboxScrollPct()), m.inboxPanel(m.width, height), m.width, height, FocusInbox)
}

func (m Model) inboxTitle() string {
	if len(m.prs) == 0 {
		return "Inbox (00/00)"
	}
	return fmt.Sprintf("Inbox (%02d/%02d)", m.selected+1, len(m.prs))
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
		gap := 1
		leftW := max(28, m.width*40/100)
		diffW := max(30, m.width-leftW-gap)
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
	style := m.theme.Styles.Pane
	if m.focus == focus {
		style = m.theme.Styles.Focused
	}
	// Treat width/height as the total outer size; subtract the frame so the
	// rendered box (border + padding + content) fits exactly within the
	// allotted columns. This keeps the rightmost pane's right border visible.
	innerW := max(1, width-style.GetHorizontalFrameSize())
	innerH := max(1, height-style.GetVerticalFrameSize())
	// Hard-fit content before border rendering. lipgloss sizing can pad, but it
	// must not be the only overflow guard because wide lines can reflow the box.
	content = fitPaneContent(content, innerW, innerH)
	box := style.Width(innerW).Height(innerH).MaxWidth(width).MaxHeight(height).Render(content)
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
	selectedStyle := lipgloss.NewStyle().Width(innerW)
	if !m.theme.NoColor {
		selectedStyle = selectedStyle.Background(m.theme.Palette.Border)
	}
	var b strings.Builder
	// Track line number where selected item starts (for viewport scroll sync).
	lineCount := 0
	selectedLine := 0
	selectedHeight := 0
	for i, pr := range m.prs {
		if i == m.selected {
			selectedLine = lineCount
			selectedHeight = 3
			if pr.Skipped {
				selectedHeight++
			}
		}
		title := truncateANSIWidth(pr.Title, innerW)
		titleStr := s.TitleStrong.Render(title)
		if i == m.selected {
			titleStr = selectedStyle.Render(titleStr)
		}
		fmt.Fprintf(&b, "%s\n", titleStr)
		lineCount++
		ciStr := m.theme.CIStatus(pr.CIStatus)
		if reviewStr := m.theme.ReviewStatus(pr.ReviewDecision); reviewStr != "" {
			ciStr = ciStr + s.Muted.Render(" · ") + reviewStr
		}
		ciLine := truncateANSIWidth(ciStr, innerW)
		if i == m.selected {
			ciLine = selectedStyle.Render(ciLine)
		}
		fmt.Fprintf(&b, "%s\n", ciLine)
		lineCount++
		authorLine := truncateANSIWidth(s.Muted.Render(fmt.Sprintf("#%d · %s", pr.Number, pr.Author)), innerW)
		if i == m.selected {
			authorLine = selectedStyle.Render(authorLine)
		}
		fmt.Fprintf(&b, "%s\n", authorLine)
		lineCount++
		if pr.Skipped {
			skipped := truncateANSIWidth(s.Muted.Render("skipped"), innerW)
			if i == m.selected {
				skipped = selectedStyle.Render(skipped)
			}
			fmt.Fprintf(&b, "%s\n", skipped)
			lineCount++
		}
		if i < len(m.prs)-1 {
			b.WriteByte('\n')
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
	content := fitPaneContentWidth(m.summaryContent(pr, innerW), innerW)
	vp := m.summaryVP
	vp.Width = innerW
	vp.Height = max(1, height-paneFrame)
	vp.SetContent(content)
	return vp.View()
}

func fitPaneContentWidth(content string, width int) string {
	if width <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = truncateANSIWidth(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func (m Model) summaryContent(pr model.PullRequest, innerW int) string {
	s := m.theme.Styles
	var b strings.Builder

	// Risk badge + author line.
	riskBadge := m.theme.RiskBadge(pr.Triage.Risk)
	fmt.Fprintf(&b, "%s  %s  +%d -%d\n", riskBadge, s.Muted.Render(pr.Author), pr.Additions, pr.Deletions)

	// Files browser (primary content).
	if len(pr.Files) > 0 {
		b.WriteByte('\n')
		b.WriteString(s.Subtitle.Render("Files"))
		b.WriteByte('\n')
		for i, file := range pr.Files {
			path := file.Path
			if i == m.fileIndex {
				path = s.File.Render(path)
			}
			status := truncateANSIWidth(s.Muted.Render(file.Status), 10)
			add := truncateANSIWidth(s.Add.Render(fmt.Sprintf("+%d", file.Additions)), 6)
			del := truncateANSIWidth(s.Delete.Render(fmt.Sprintf("-%d", file.Deletions)), 6)
			metaW := lipgloss.Width(stripANSI(status)) + lipgloss.Width(stripANSI(add)) + lipgloss.Width(stripANSI(del)) + 3
			pathW := max(1, innerW-metaW)
			fmt.Fprintf(&b, "%s %s %s %s\n",
				truncateANSIWidth(path, pathW),
				status,
				add,
				del,
			)
		}
	}

	// AI verdict (shown only when available).
	if pr.Triage.AI != nil {
		b.WriteByte('\n')
		writeStyledAINotes(&b, m, pr.Triage)
	} else {
		b.WriteByte('\n')
		b.WriteString(s.Muted.Render("press r to run AI review"))
		b.WriteByte('\n')
	}

	return b.String()
}

func (m Model) diffPanel(width int, height int) string {
	if !m.diffEnabled {
		return m.theme.Styles.Muted.Render("press v to load diff")
	}
	if m.diff == "" {
		return m.theme.Styles.Muted.Render("loading diff…")
	}
	_, _, diffInnerW := m.paneInnerWidths()
	vpW := max(1, diffInnerW)
	vpH := max(1, height-paneFrame)
	// Render from the stored viewport so YOffset/XOffset (scroll positions) are
	// preserved and TotalLineCount is accurate for vpCanScroll checks.
	vp := m.diffVP
	vp.Width = max(1, vpW-truncateSafetyMargin)
	vp.Height = vpH
	return vp.View()
}

// truncateANSIWidth clips a string with ANSI escape codes to maxW visible columns.
// No ellipsis. Uses rune widths so wide glyphs stay inside border.
func truncateANSIWidth(s string, maxW int) string {
	maxW = max(0, maxW-truncateSafetyMargin)
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
