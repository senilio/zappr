package tui

import "strings"

type commentViewState struct {
	selectedKey string
	prKey       string
}

// descriptionShown reports whether the large pane shows the PR description.
// Narrow layouts have no large pane next to the Inbox.
func (m Model) descriptionShown() bool {
	return m.descriptionActive && m.width >= minMediumWidth
}

func (m Model) readerContentActive() bool {
	return m.descriptionShown() || m.summaryMode == SummaryModePlan || m.summaryMode == SummaryModeComments
}

func readerInput(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(commentPlainText(text), "\r", ""), "\n", " ")
}

func (m Model) readerTitle() string {
	if m.descriptionShown() {
		return "Description"
	}
	if m.summaryMode == SummaryModePlan {
		return "Plan"
	}
	return "Comment"
}
