package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/config"
	"github.com/senilio/zappr/internal/plan"
)

func TestExtraReaderActionsRemoved(t *testing.T) {
	m := planModel(t)
	ids := []string{
		"reader-search", "reader-next-match", "reader-previous-match", "reader-source", "reader-return", "show-full-title",
		"comments-filter-author", "comments-clear-filters", "plan-next-resource", "plan-previous-resource",
		"plan-next-project", "plan-previous-project", "plan-toggle-project", "plan-toggle-value", "plan-expand-values",
		"plan-summary", "plan-next-report", "plan-previous-report",
	}
	for _, id := range ids {
		if _, ok := m.actions.actionByID(id); ok {
			t.Errorf("removed action %s still registered", id)
		}
		if _, ok := config.Default().Actions[id]; ok {
			t.Errorf("removed action %s still configured by default", id)
		}
		if _, ok := m.actions.commands[id]; ok {
			t.Errorf("removed command %s still registered", id)
		}
	}
	for _, key := range []string{"/", "n", "N", "]", "[", "}", "{", "e", "z", "Z", "b", "backspace", "t", "u", "U"} {
		if action, ok := m.actions.actionForKey(actionContextNormal, key); ok {
			t.Errorf("key %q still occupied by %s", key, action.ID)
		}
	}
	if help := stripANSI(m.helpPanel()); strings.Contains(help, "Plan reader and comments") || strings.Contains(help, ":plan-") {
		t.Fatal("help still advertises removed functions")
	}
}

func TestPlanHeaderSparseAndWarningsInContent(t *testing.T) {
	m := pressKey(planModel(t), "4")
	if got := m.readerTitle(); got != "Plan" {
		t.Errorf("title = %q, want Plan", got)
	}
	for _, node := range m.planNodes() {
		m.planState.nodeID = node.ID
		if !strings.Contains(stripANSI(m.currentDiffContent()), "source complete") {
			t.Errorf("node %s lost source context", node.ID)
		}
	}
}

func TestPlanFirstLastNavigationUpdatesSelection(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		t.Run(fmt.Sprint(wrap), func(t *testing.T) {
			m := planModel(t)
			m.cfg.Wrap.Summary = wrap
			m = pressKey(m, "4")
			m.summaryVP.SetHeight(2)
			m = pressKey(m, "G")
			if got, want := m.planNodeIndex(), len(m.planNodes())-1; got != want || got == 0 {
				t.Fatalf("G selected %d, want %d", got, want)
			}
			if !strings.Contains(m.currentDiffContent(), "Ran Plan") {
				t.Fatal("G did not update the detail to the source comment")
			}
			m = pressKey(pressKey(m, "g"), "g")
			if m.planNodeIndex() != 0 || !strings.Contains(m.currentDiffContent(), "Overall Plan Summary") || m.summaryVP.YOffset() != 0 {
				t.Fatal("gg did not restore overall selection and detail")
			}
			m = pressSpecial(m, tea.KeyDown)
			if m.planNodeIndex() != 1 {
				t.Fatal("down no longer moves a plan item")
			}
			m = pressSpecial(m, tea.KeyUp)
			if m.planNodeIndex() != 0 {
				t.Fatal("up no longer moves a plan item")
			}
		})
	}
}

func TestPlanResourceReachableWithoutExtraActions(t *testing.T) {
	m := pressKey(planModel(t), "4")
	for range len(m.planNodes()) {
		if node, _ := m.selectedPlanNode(); node.Kind == plan.NodeResource {
			if !strings.Contains(m.currentDiffContent(), `name = "alpha"`) {
				t.Fatal("resource has no output")
			}
			return
		}
		m = pressSpecial(m, tea.KeyDown)
	}
	t.Fatal("resources unreachable using basic navigation")
}

func TestPlanPageNavigationUpdatesSelection(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		m := pressKey(planModel(t), "4")
		m.cfg.Wrap.Summary = wrap
		m.summaryVP.SetHeight(2)
		m = pressSpecial(m, tea.KeyPgDown)
		if m.planNodeIndex() == 0 {
			t.Fatalf("wrap=%v: page down did not change selected item", wrap)
		}
		m = pressSpecial(m, tea.KeyPgUp)
		if m.planNodeIndex() != 0 || !strings.Contains(m.currentDiffContent(), "Overall Plan Summary") {
			t.Fatalf("wrap=%v: page up did not restore summary", wrap)
		}
	}
}

func TestPlanNavigationWithNoNodesIsSafe(t *testing.T) {
	m := pressKey(planModel(t), "4")
	m.planState.doc = nil
	for _, key := range []string{"g", "G", "down", "up", "pgup", "pgdown"} {
		m = pressKey(m, key)
	}
}

func TestPlanValuesVisibleWithoutFoldControls(t *testing.T) {
	m := planModel(t)
	value := "+ content = <<-EOT\n" + strings.Repeat("        changed value\n", 40) + "        FINAL-VALUE\n        EOT"
	m.prs[0].Comments[0].Body = strings.Replace(m.prs[0].Comments[0].Body, `+ name = "alpha"`, value, 1)
	m.reconcilePlan()
	m = pressKey(m, "4")
	m = pressSpecial(pressSpecial(m, tea.KeyDown), tea.KeyDown)
	if node, _ := m.selectedPlanNode(); node.Kind != plan.NodeResource || !strings.Contains(m.currentDiffContent(), "FINAL-VALUE") || strings.Contains(m.currentDiffContent(), "lines folded") {
		t.Fatalf("changed value hidden without a way to expand it: %s", m.currentDiffContent())
	}
}

func TestPlanReportPreviewNeedsExplicitAdoption(t *testing.T) {
	m := planModel(t)
	oldID := m.planState.reportID
	other := m.prs[0].Comments[0]
	other.ID, other.CreatedAt, other.UpdatedAt = "202", time.Unix(200, 0), time.Unix(200, 0)
	m.prs[0].Comments = append(m.prs[0].Comments, other)
	m.reconcilePlan()
	m = pressKey(m, "4")
	for range len(m.planNodes()) {
		if node, _ := m.selectedPlanNode(); node.Kind == plan.NodeReport {
			break
		}
		m = pressSpecial(m, tea.KeyDown)
	}
	if m.planState.reportID != oldID || !strings.Contains(m.currentDiffContent(), "Report preview") {
		t.Fatal("highlighting a different report silently adopted it")
	}
	m = pressSpecial(m, tea.KeyEnter)
	if m.planState.reportID == oldID || m.planState.nodeID != m.planState.reportID+"/summary" {
		t.Fatal("Enter did not explicitly adopt report at Overall")
	}
}
