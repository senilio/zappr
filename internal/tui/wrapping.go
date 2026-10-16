package tui

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/senilio/zappr/internal/config"
	"github.com/senilio/zappr/internal/model"
)

// Settings commands may execute out of order. Serialize writes and discard
// superseded snapshots, including snapshots from other preference controls.
type settingsSaver struct {
	mu       sync.Mutex
	revision atomic.Uint64
}

var hangingPrefix = regexp.MustCompile(`^\s*(?:(?:•|[0-9]+\.)\s+(?:\[[ x]\]\s+)?)?`)

// Pane fitting resets each row's ANSI state. Close and reopen active styles
// at wrap boundaries so continuation rows render correctly on their own.
func isolateStyledRows(content string) string {
	var b strings.Builder
	pen := cellbuf.NewPenWriter(&b)
	_, _ = pen.Write([]byte(content))
	_ = pen.Close()
	return b.String()
}

// Wrap individual logical lines, keeping list markers and code indentation in
// place. Code is hard-wrapped to preserve whitespace rather than reflowed.
func wrapStructured(content string, width int) string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		line = truncateANSIWidth(line, max(1, len(line)*tabStopWidth))
		plain := stripANSI(line)
		prefix := hangingPrefix.FindString(plain)
		prefixW := min(ansi.StringWidth(prefix), max(0, width-1))
		body := ansi.Cut(line, prefixW, ansi.StringWidth(line))
		var wrapped string
		if strings.HasPrefix(plain, "    ") {
			wrapped = ansi.Hardwrap(body, max(1, width-prefixW), true)
		} else {
			wrapped = wrapLines(body, max(1, width-prefixW))
		}
		for j, row := range strings.Split(isolateStyledRows(wrapped), "\n") {
			indent := strings.Repeat(" ", prefixW)
			if j == 0 {
				indent = ansi.Cut(line, 0, prefixW)
			}
			out = append(out, indent+row)
		}
	}
	return strings.Join(out, "\n")
}

func (m Model) summaryLayout(pr model.PullRequest, width int) (string, []rowSpan) {
	// The file list stays one row per file even when summary wrapping is on.
	if !m.cfg.Wrap.Summary || m.summaryMode == SummaryModeFiles {
		content := m.unwrappedSummaryContent(pr, width)
		var spans []rowSpan
		switch m.summaryMode {
		case SummaryModeFiles:
			for i, line := range fileSummaryLines(pr.Files) {
				start := line
				if i == 0 || splitDir(pr.Files[i-1].Path) != splitDir(pr.Files[i].Path) {
					start--
				}
				spans = append(spans, rowSpan{start, line + 1})
			}
		case SummaryModeCommits:
			for i := range pr.Commits {
				spans = append(spans, rowSpan{i * 2, i*2 + 2})
			}
		case SummaryModeComments:
			row := 0
			for _, comment := range newestComments(pr.Comments) {
				var b strings.Builder
				m.writeCommentSummary(&b, model.PullRequest{Comments: []model.PRComment{comment}}, width)
				end := row + strings.Count(b.String(), "\n")
				spans = append(spans, rowSpan{row, end})
				row = end + 1
			}
		}
		return content, spans
	}
	var blocks []string
	var spans []rowSpan
	row := 0
	appendBlock := func(block string) {
		block = strings.TrimSuffix(wrapStructured(block, max(1, width)), "\n")
		end := row + len(strings.Split(block, "\n"))
		spans = append(spans, rowSpan{row, end})
		blocks = append(blocks, block)
		row = end
	}
	switch m.summaryMode {
	case SummaryModeCommits:
		for i, commit := range newestCommits(pr.Commits) {
			mv := m
			mv.commitIndex = -1
			if i == m.commitIndex {
				mv.commitIndex = 0
			}
			var b strings.Builder
			mv.writeCommitSummary(&b, model.PullRequest{Commits: []model.PRCommit{commit}}, width)
			appendBlock(b.String())
		}
	case SummaryModeComments:
		for i, comment := range newestComments(pr.Comments) {
			if i > 0 {
				blocks = append(blocks, "")
				row++
			}
			var b strings.Builder
			// The existing writer must receive the full content width here;
			// clipping before wrapping would irreversibly lose text.
			m.writeCommentSummary(&b, model.PullRequest{Comments: []model.PRComment{comment}}, len(comment.Body)+len(comment.Path)+len(comment.Author)+256)
			appendBlock(b.String())
		}
	}
	if len(blocks) == 0 {
		return m.unwrappedSummaryContent(pr, width), nil
	}
	return strings.Join(blocks, "\n") + "\n", spans
}

func splitDir(path string) string { dir, _ := splitFileDir(path); return dir }

func spanAt(spans []rowSpan, row int) int {
	for i, span := range spans {
		if row < span.end {
			return i
		}
	}
	return max(0, len(spans)-1)
}

var diffGutter = regexp.MustCompile(`^(?:[ +\-]?[ 0-9]+⋮[ 0-9]+│ ?|[ +\-]?[ 0-9]+│ ?|[+\-])`)

func wrapDiffLine(line string, width int) []string {
	line = truncateANSIWidth(line, max(1, len(line)*tabStopWidth))
	prefix := diffGutter.FindString(stripANSI(line))
	prefixW := min(ansi.StringWidth(prefix), max(0, width-1))
	body := ansi.Cut(line, prefixW, ansi.StringWidth(line))
	wrapped := ansi.Hardwrap(body, max(1, width-prefixW), true)
	rows := strings.Split(isolateStyledRows(wrapped), "\n")
	for i := range rows {
		gutter := ansi.Cut(line, 0, prefixW)
		if i > 0 {
			gutter = continuationGutter(gutter)
		}
		rows[i] = gutter + rows[i]
	}
	return rows
}

func continuationGutter(gutter string) string {
	var b strings.Builder
	var state byte
	for len(gutter) > 0 {
		seq, width, n, next := ansi.DecodeSequence(gutter, state, nil)
		if width > 0 {
			seq = strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return ' '
				}
				return r
			}, seq)
		}
		b.WriteString(seq)
		gutter, state = gutter[n:], next
	}
	return b.String()
}

// Every visual diff row records its original logical row. Both the stored
// viewport and the renderer use this layout, so scrolling and reflow agree.
func (m Model) diffLayout(width int) (string, []int) {
	content := m.currentDiffContent()
	var rows []string
	var sources []int
	source := -1
	for _, line := range strings.Split(content, "\n") {
		nativeSide := m.sideBySide && strings.HasPrefix(stripANSI(line), "│") && strings.Count(stripANSI(line), "│") >= 4
		if !nativeSide || sideLineNumber.MatchString(stripANSI(line)) {
			source++
		}
		visual := []string{line}
		if m.cfg.Wrap.Diff && !nativeSide {
			visual = wrapDiffLine(line, width)
		}
		for _, row := range visual {
			rows = append(rows, row)
			sources = append(sources, source)
		}
	}
	return strings.Join(rows, "\n"), sources
}

var sideLineNumber = regexp.MustCompile(`│\s*[0-9]+\s*│`)

type sideLayoutMsg struct {
	key, content string
	anchor       int
	offset       int
	err          error
}

func (m Model) sideLayoutRaw() string {
	if !m.diffEnabled || !m.sideBySide || m.diffMode == DiffModeFile {
		return ""
	}
	if m.summaryMode == SummaryModeCommits && m.rawCommitDiff != "" {
		return m.rawCommitDiff
	}
	if raw := m.selectedRawDiffFile(); raw != "" {
		return raw
	}
	return m.rawDiff
}

func (m Model) sideLayoutKey() string {
	raw := m.sideLayoutRaw()
	if raw == "" {
		return ""
	}
	return fmt.Sprintf("%x:%d:%t", sha256.Sum256([]byte(raw)), m.diffVP.Width, m.cfg.Wrap.Diff)
}

// delta must lay out each side independently at the actual pane width. Reuse
// cached raw diffs; changing width or wrapping never makes a GitHub request.
func (m *Model) sideLayoutCmd(anchor int) tea.Cmd {
	key := m.sideLayoutKey()
	if key != m.sideDiffFailed {
		m.sideDiffFailed = ""
	}
	if key == "" || key == m.sideDiffKey || key == m.sideDiffRequested || key == m.sideDiffFailed {
		return nil
	}
	m.sideDiffRequested = key
	raw, width, wrap := m.sideLayoutRaw(), m.diffVP.Width, m.cfg.Wrap.Diff
	offset := m.diffVP.YOffset
	if !wrap {
		width = diffRenderWidth(width)
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		content, err := renderDiff(ctx, raw, max(1, width), true, wrap)
		return sideLayoutMsg{key: key, content: content, anchor: anchor, offset: offset, err: err}
	}
}

func visualRow(sources []int, logical int) int {
	for i, source := range sources {
		if source >= logical {
			return i
		}
	}
	return max(0, len(sources)-1)
}

func (m Model) diffAnchor() int {
	_, sources := m.diffLayout(max(1, m.diffVP.Width))
	if len(sources) == 0 {
		return 0
	}
	return sources[min(m.diffVP.YOffset, len(sources)-1)]
}

func (m *Model) restoreDiffAnchor(logical int) {
	_, sources := m.diffLayout(max(1, m.diffVP.Width))
	m.diffVP.SetYOffset(visualRow(sources, logical))
}

func (m Model) summaryAnchor() int {
	pr, ok := m.current()
	if !ok {
		return 0
	}
	_, spans := m.summaryLayout(pr, max(1, m.summaryVP.Width))
	return spanAt(spans, m.summaryVP.YOffset)
}

func (m *Model) restoreSummaryAnchor(item int) {
	pr, ok := m.current()
	if !ok {
		return
	}
	_, spans := m.summaryLayout(pr, max(1, m.summaryVP.Width))
	if item < len(spans) {
		m.summaryVP.SetYOffset(spans[item].start)
	}
}

type settingsSavedMsg struct{ err error }

func (m *Model) saveSettings() tea.Cmd {
	if m.settingsSaver == nil {
		m.settingsSaver = &settingsSaver{}
	}
	saver, cfg := m.settingsSaver, m.cfg
	revision := saver.revision.Add(1)
	return func() tea.Msg {
		saver.mu.Lock()
		defer saver.mu.Unlock()
		if revision != saver.revision.Load() {
			return settingsSavedMsg{}
		}
		return settingsSavedMsg{err: config.Save(cfg)}
	}
}

type rowSpan struct{ start, end int } // end excludes separators

func keepSpanVisible(offset, height int, span rowSpan) int {
	if span.start < offset {
		return span.start
	}
	// A tall item starts at its title; users can then page through it.
	if span.end-span.start > height {
		return span.start
	}
	if span.end > offset+height {
		return span.end - height
	}
	return offset
}

func (m Model) inboxContent(innerW int) (string, []rowSpan) {
	s := m.theme.Styles
	var b strings.Builder
	spans := make([]rowSpan, len(m.prs))
	row := 0
	for i, pr := range m.prs {
		spans[i].start = row
		selected := i == m.selected
		var badges []string
		if pr.AutoMerge {
			badges = append(badges, "[auto]")
		}
		if pr.Draft {
			badges = append(badges, "[draft]")
		}
		badge := truncateANSIWidth(strings.Join(badges, " "), innerW)
		titleWidth := innerW
		if badge != "" {
			titleWidth = max(0, innerW-ansi.StringWidth(badge)-1)
		}
		title := truncateANSIWidth(pr.Title, titleWidth)
		if m.cfg.Wrap.Inbox {
			title = wrapLines(pr.Title, max(1, titleWidth))
			if titleWidth == 0 {
				title = wrapLines(pr.Title, innerW) + "\n" + badge
				badge = ""
			}
		}
		titleStyle, badgeStyle := s.Muted, s.Muted
		if selected {
			titleStyle, badgeStyle = s.TitleStrong, s.Title
		}
		for j, line := range strings.Split(title, "\n") {
			titleRow := titleStyle.Render(line)
			if j == 0 && badge != "" {
				titleRow += strings.Repeat(" ", max(0, innerW-ansi.StringWidth(line)-ansi.StringWidth(badge))) + badgeStyle.Render(badge)
			}
			b.WriteString(truncatePaneLineWidth(titleRow, innerW) + "\n")
			row++
		}
		dot := s.Muted.Render(" · ")
		meta := fmt.Sprintf("#%d · %s", pr.Number, pr.Author)
		ci, review := m.theme.CIStatus(pr.CIStatus), m.theme.ReviewStatus(pr.ReviewDecision)
		if selected {
			meta = s.TitleStrong.Render(meta) + dot + ci
			if review != "" {
				meta += dot + review
			}
		} else {
			meta += " · " + stripANSI(ci)
			if review != "" {
				meta += " · " + stripANSI(review)
			}
			meta = s.Muted.Render(meta)
		}
		b.WriteString(truncatePaneLineWidth(meta, innerW) + "\n")
		row++
		spans[i].end = row
		if i < len(m.prs)-1 {
			b.WriteString(s.Muted.Render(strings.Repeat("─", innerW)) + "\n")
			row++
		}
	}
	return strings.TrimSuffix(b.String(), "\n"), spans
}

func (m Model) inboxOffset(innerW, height int) int {
	_, spans := m.inboxContent(innerW)
	offset := m.inboxVP.YOffset
	if !m.inboxManualScroll && m.selected >= 0 && m.selected < len(spans) {
		offset = keepSpanVisible(offset, height, spans[m.selected])
	}
	total := 0
	if len(spans) > 0 {
		total = spans[len(spans)-1].end
	}
	return min(max(0, offset), max(0, total-height))
}

func (m *Model) syncInboxVP() {
	w, _, _ := m.paneInnerWidths()
	content, _ := m.inboxContent(w)
	offset := m.inboxOffset(w, m.inboxVP.Height)
	m.inboxVP.Width = w
	m.inboxVP.SetContent(content)
	m.inboxVP.SetYOffset(offset)
}

func (m Model) inboxAnchor() int {
	_, spans := m.inboxContent(max(1, m.inboxVP.Width))
	return spanAt(spans, m.inboxVP.YOffset)
}

func (m *Model) restoreInboxAnchor(item int) {
	if !m.inboxManualScroll {
		return
	}
	_, spans := m.inboxContent(max(1, m.inboxVP.Width))
	if item < len(spans) {
		m.inboxVP.SetYOffset(spans[item].start)
	}
}
