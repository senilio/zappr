package tui

import "strings"

type commentViewState struct {
	selectedKey string
	prKey       string
}

func (m Model) readerContentActive() bool {
	return m.summaryMode == SummaryModePlan || m.summaryMode == SummaryModeComments
}

func readerInput(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(commentPlainText(text), "\r", ""), "\n", " ")
}

func (m Model) readerTitle() string {
	if m.summaryMode == SummaryModePlan {
		return "Plan"
	}
	return "Comment"
}
