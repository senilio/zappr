package tui

import (
	"strings"
	"testing"

	"github.com/senilio/zappr/internal/config"
)

func TestSummaryActionsOnlyIncludeStandardModes(t *testing.T) {
	m := minimalModel()
	want := map[string]bool{"summary-files": true, "summary-commits": true, "summary-comments": true}
	for _, action := range m.actions.actions {
		if strings.HasPrefix(action.ID, "summary-") {
			if !want[action.ID] {
				t.Errorf("unexpected built-in summary action %q", action.ID)
			}
			delete(want, action.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing summary actions: %v", want)
	}
}

func TestSummaryTitleOnlyIncludesThreeTabs(t *testing.T) {
	for _, tc := range []struct {
		mode SummaryMode
		want string
	}{
		{SummaryModeFiles, "Summary [1 files] [2] [3]"},
		{SummaryModeCommits, "Summary [1] [2 commits] [3]"},
		{SummaryModeComments, "Summary [1] [2] [3 comments]"},
	} {
		t.Run(tc.mode.String(), func(t *testing.T) {
			m := minimalModel()
			m.summaryMode = tc.mode
			if got := stripANSI(m.summaryTitle()); got != tc.want {
				t.Fatalf("summary title = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSummaryHelpOnlyIncludesThreeShortcuts(t *testing.T) {
	m := minimalModel()
	for name, view := range map[string]string{
		"help":   m.helpPanel(),
		"inbox":  m.inboxView(),
		"detail": m.detailView(),
	} {
		t.Run(name, func(t *testing.T) {
			view = stripANSI(view)
			if !strings.Contains(view, "1/2/3") || strings.Contains(view, "1/2/3/4") {
				t.Fatalf("summary shortcuts should only show 1/2/3: %q", view)
			}
		})
	}
}

func TestUnboundFourthKeyDoesNotChangeSummaryMode(t *testing.T) {
	m := minimalModel()
	m.summaryMode = SummaryModeComments
	m = pressKey(m, "4")
	if m.summaryMode != SummaryModeComments {
		t.Fatalf("summary mode after unbound 4 = %v, want comments", m.summaryMode)
	}
}

func TestFourthKeyAvailableForCustomComment(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["workflow-plan"] = config.ActionConfig{Operation: "comment", Keys: []string{"4"}, Comment: "workflow plan"}
	registry, err := newActionRegistry(cfg.Actions)
	if err != nil {
		t.Fatalf("newActionRegistry: %v", err)
	}
	action, ok := registry.actionForKey(actionContextNormal, "4")
	if !ok || action.ID != "workflow-plan" || action.Operation != "comment" || action.Comment != "workflow plan" {
		t.Fatalf("action for 4 = %#v, found = %v", action, ok)
	}
}
