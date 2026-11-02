package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/senilio/zappr/internal/model"
)

func commentKind(comment model.PRComment) string {
	if comment.Kind == "" {
		return "comment"
	}
	return comment.Kind
}

func (m Model) visibleComments(pr model.PullRequest) []model.PRComment {
	return newestComments(pr.Comments)
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
	comments := m.visibleComments(pr)
	for i, comment := range comments {
		if comment.Key() == m.commentState.selectedKey {
			m.commentIndex = i
			return
		}
	}
	if len(comments) == 0 {
		m.commentIndex, m.commentState.selectedKey = 0, ""
		return
	}
	m.commentIndex = max(0, min(m.commentIndex, len(comments)-1))
	m.commentState.selectedKey = comments[m.commentIndex].Key()
}

func (m Model) commentSummaryLayout(pr model.PullRequest, width int) (string, []rowSpan) {
	comments := m.visibleComments(pr)
	if len(comments) == 0 {
		message := "no comments"
		if len(pr.Comments) != 0 {
			message = "no matching comments"
		}
		return truncateANSIWidth(m.theme.Styles.Muted.Render(message), width) + "\n", nil
	}
	var rows []string
	var spans []rowSpan
	for i, comment := range comments {
		start := len(rows)
		style := m.theme.Styles.Muted
		selected := comment.Key() == m.commentState.selectedKey || m.commentState.selectedKey == "" && i == m.commentIndex
		if selected {
			style = m.theme.Styles.TitleStrong
		}
		meta := comment.Author + " · " + commentKind(comment)
		if stamp := summaryTimestamp(comment.CreatedAt, time.Now()); stamp != "" {
			meta = stamp + " · " + meta
		}
		line := readerInput(meta)
		if m.cfg.Wrap.Summary {
			line = wrapStructured(line, max(1, width))
		}
		for _, row := range strings.Split(line, "\n") {
			rows = append(rows, truncateANSIWidth(style.Render(row), width))
		}
		spans = append(spans, rowSpan{start, len(rows)})
	}
	return strings.Join(rows, "\n") + "\n", spans
}

func (m Model) commentsContext(pr model.PullRequest) string {
	return fmt.Sprintf("%d/%d", len(m.visibleComments(pr)), len(pr.Comments))
}
