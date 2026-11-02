package tui

import (
	"strings"
	"testing"

	"github.com/senilio/zappr/internal/config"
)

func TestSummaryActionsIncludeOptionalPlanMode(t *testing.T) {
	m := minimalModel()
	want := map[string]bool{"summary-files": true, "summary-commits": true, "summary-comments": true, "summary-plan": true}
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

func TestSummaryTitleNamesActiveDestination(t *testing.T) {
	for _, tc := range []struct {
		mode SummaryMode
		want string
	}{
		{SummaryModeFiles, "Summary [1 files] [2] [3]"},
		{SummaryModeCommits, "Summary [1] [2 commits] [3]"},
		{SummaryModeComments, "Summary 0/0 [1] [2] [3 comments]"},
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

func TestSummaryHelpIncludesOptionalPlanShortcut(t *testing.T) {
	m := minimalModel()
	if view := stripANSI(m.helpPanel()); !strings.Contains(view, "1/2/3/4") {
		t.Fatal("missing optional Plan help")
	}
}

func TestUnavailableFourthKeyDoesNotChangeSummaryMode(t *testing.T) {
	m := minimalModel()
	m.summaryMode = SummaryModeComments
	m = pressKey(m, "4")
	if m.summaryMode != SummaryModeComments {
		t.Fatalf("summary mode after unbound 4 = %v, want comments", m.summaryMode)
	}
}

func TestPlanKeyCollisionRejectsCustomWorkflow(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["workflow-plan"] = config.ActionConfig{Operation: "comment", Keys: []string{"4"}, Comment: "workflow plan"}
	_, err := newActionRegistry(cfg.Actions)
	if err == nil || !strings.Contains(err.Error(), "4") || !strings.Contains(err.Error(), "summary-plan") {
		t.Fatalf("collision error = %v", err)
	}
	if _, err := New(nil, "org/repo", 0, "", cfg); err == nil {
		t.Fatal("New accepted collision")
	}
}
