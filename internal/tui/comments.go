package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/model"
)

func commentKind(comment model.PRComment) string {
	if comment.Kind == "" {
		return "comment"
	}
	return comment.Kind
}

func (m *Model) reconcileComments() {
	pr, ok := m.current()
	if !ok {
		return
	}
	key := readerPRKey(pr)
	if m.commentState.prKey != "" && m.commentState.prKey != key {
		m.commentState, m.commentIndex = commentViewState{}, 0
	}
	m.commentState.prKey = key
	comments := m.activityEntries(pr)
	for i, comment := range comments {
		if comment.Key == m.commentState.selectedKey {
			m.commentIndex = i
			return
		}
	}
	if len(comments) == 0 {
		m.commentIndex, m.commentState.selectedKey = 0, ""
		return
	}
	m.commentIndex = max(0, min(m.commentIndex, len(comments)-1))
	m.commentState.selectedKey = comments[m.commentIndex].Key
}

func (m Model) commentSummaryLayout(pr model.PullRequest, width int) (string, []rowSpan) {
	comments := m.activityEntries(pr)
	if len(comments) == 0 {
		return truncateANSIWidth(m.theme.Styles.Muted.Render("no activity"), width) + "\n", nil
	}
	var rows []string
	var spans []rowSpan
	now := time.Now()
	for i, entry := range comments {
		start := len(rows)
		selected := entry.Key == m.commentState.selectedKey || m.commentState.selectedKey == "" && i == m.commentIndex
		line := m.activityLine(entry, now, width, selected, m.cfg.Wrap.Summary)
		if m.cfg.Wrap.Summary {
			line = wrapStructured(line, max(1, width))
		}
		for _, row := range strings.Split(line, "\n") {
			rows = append(rows, truncateANSIWidth(row, width))
		}
		spans = append(spans, rowSpan{start, len(rows)})
	}
	return strings.Join(rows, "\n") + "\n", spans
}

func (m Model) commentsContext(pr model.PullRequest) string {
	n := len(m.activityEntries(pr))
	return fmt.Sprintf("%d/%d", n, n)
}

func (m Model) activityLine(entry activityEntry, now time.Time, width int, selected, wrap bool) string {
	s := m.theme.Styles
	stamp := summaryTimestamp(entry.At, now)
	prefix := ""
	if stamp != "" {
		prefix = stamp + " - "
	}
	author := m.activityAuthor(entry.Author)
	if !wrap {
		suffixWidth := 0
		if entry.possessive {
			suffixWidth = 2
		}
		room := width - ansi.StringWidth(prefix) - ansi.StringWidth(entry.Action) - suffixWidth - 1
		if room < 1 {
			if clock := strings.LastIndexByte(stamp, ' '); clock >= 0 {
				short := stamp[clock+1:] + " - "
				room += ansi.StringWidth(prefix) - ansi.StringWidth(short)
				prefix = short
			}
		}
		if room < 1 {
			room += ansi.StringWidth(prefix)
			prefix = ""
		}
		if room > 0 {
			author = ansi.Truncate(author, room, "…")
		} else {
			author = ""
		}
	}
	if author != "" {
		if entry.possessive {
			author += "’s"
		}
		author += " "
	}
	nameStyle := s.Muted
	if selected {
		nameStyle = s.TitleStrong
	}
	actionStyle := nameStyle
	switch entry.tone {
	case activityApproved:
		actionStyle = s.SuccessCI
	case activityChanges:
		actionStyle = s.Error
	case activityStale:
		actionStyle = s.Wait
	}
	return s.Muted.Bold(selected).Render(prefix) + nameStyle.Render(author) + actionStyle.Bold(selected).Render(entry.Action)
}
