package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/config"
	"github.com/senilio/zappr/internal/model"
	"github.com/senilio/zappr/internal/plan"
)

func planModel(t *testing.T) Model {
	t.Helper()
	body, err := os.ReadFile("../plan/testdata/single-project.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := minimalModel()
	m.cfg = config.Default()
	m.width, m.height = 130, 40
	m.diffEnabled = false
	pr := &m.prs[0]
	pr.HeadRefOID = "head-one"
	pr.DetailsLoaded, pr.ReviewCommentsLoaded, pr.ReviewersLoaded, pr.ConversationCommentsLoaded = true, true, true, true
	pr.Comments = []model.PRComment{{ID: "101", URL: "https://github.com/example/infra/pull/1#issuecomment-101", Author: "deploy-bot", CreatedAt: time.Unix(100, 0), UpdatedAt: time.Unix(100, 0), Body: string(body)}}
	m.allPRs = append([]model.PullRequest(nil), m.prs...)
	m.reconcilePlan()
	return m
}

func TestPlanAvailabilityAndExplicitEntry(t *testing.T) {
	m := planModel(t)
	if m.summaryMode != SummaryModeFiles || !strings.Contains(stripANSI(m.summaryTitle()), "[4]") {
		t.Fatal("availability changed view or missing tab")
	}
	m = pressKey(m, "4")
	if m.summaryMode != SummaryModePlan || m.planNodes()[0].Kind != plan.NodeSummary {
		t.Fatal("not at overall summary")
	}
	if !strings.Contains(stripANSI(m.currentDiffContent()), "source complete") {
		t.Fatal("missing source context")
	}
	for _, disabled := range []bool{false, true} {
		n := minimalModel()
		if disabled {
			n = planModel(t)
			off := false
			n.cfg.Atlantis.Enabled = &off
			n.reconcilePlan()
		}
		n = pressKey(n, "4")
		if n.summaryMode == SummaryModePlan || strings.Contains(stripANSI(n.summaryTitle()), "[4]") {
			t.Fatal("unavailable report opened")
		}
	}
}

func TestPlanEntryWithDiffDisabled(t *testing.T) {
	for _, width := range []int{65, 95, 130} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := planModel(t)
			m.width = width
			m.sideBySide = true
			m.diffMode = DiffModeFile
			m = pressKey(m, "4")
			if !m.largePaneActive() || !strings.Contains(m.currentDiffContent(), "Overall Plan Summary") {
				t.Fatal("plan depends on source diff")
			}
			if strings.Contains(stripANSI(m.diffPanel(70, 20)), "press v") {
				t.Fatal("disabled source diff hides plan")
			}
		})
	}
}

func TestPlanOpenRequestCannotFollowAnotherPR(t *testing.T) {
	m := planModel(t)
	fresh := m.prs[0]
	m.prs[0].Comments = nil
	m.prs[0].ConversationCommentsLoaded = false
	m.allPRs = append([]model.PullRequest(nil), m.prs...)
	m.reconcilePlan()
	m = pressKey(m, "4")
	if m.planState.pendingOpen == nil {
		t.Fatal("missing explicit pending request")
	}
	m.prs = append(m.prs, model.PullRequest{Repo: "org/repo", Number: 2, Title: "second"})
	m.allPRs = append([]model.PullRequest(nil), m.prs...)
	m.selected = 1
	m.resetPRViewState()
	next, _ := m.Update(refreshPRMsg{repo: fresh.Repo, number: fresh.Number, pr: fresh})
	m = next.(Model)
	if pr, _ := m.current(); pr.Number != 2 || m.summaryMode == SummaryModePlan {
		t.Fatal("late request followed another PR")
	}
}

func TestPlanBackgroundFetchDoesNotStealFocus(t *testing.T) {
	m := planModel(t)
	m.planState = planReaderState{}
	next, _ := m.Update(refreshPRMsg{repo: m.prs[0].Repo, number: 1, pr: m.prs[0]})
	m = next.(Model)
	if m.summaryMode != SummaryModeFiles || m.focus != FocusInbox || !m.planAvailable() {
		t.Fatal("background fetch changed reading intent")
	}
}

func TestPlanRefreshPreservesRootAndNode(t *testing.T) {
	m := pressKey(planModel(t), "4")
	report, _ := m.selectedPlanReport()
	m.planState.nodeID = report.Projects[0].Resources[0].ID
	root, node := m.planState.reportID, m.planState.nodeID
	pr := m.prs[0]
	pr.Comments = append([]model.PRComment(nil), pr.Comments...)
	newer := pr.Comments[0]
	newer.ID = "102"
	newer.CreatedAt = newer.CreatedAt.Add(time.Minute)
	newer.UpdatedAt = newer.CreatedAt
	pr.Comments = append(pr.Comments, newer)
	next, _ := m.Update(refreshPRMsg{repo: pr.Repo, number: pr.Number, pr: pr})
	m = next.(Model)
	if m.planState.reportID != root || m.planState.nodeID != node {
		t.Fatal("refresh switched report or resource")
	}
}

func TestPlanParseCacheReusedAcrossViewsAndTicks(t *testing.T) {
	m := pressKey(planModel(t), "4")
	first := m.planState.doc
	for range 2 {
		_ = m.View()
		next, _ := m.Update(uiTickMsg{})
		m = next.(Model)
	}
	if first != m.planState.doc {
		t.Fatal("render/tick reparsed report")
	}
	m.prs[0].Comments = append([]model.PRComment(nil), m.prs[0].Comments...)
	m.prs[0].Comments[0].Body += "\nupdated source\n"
	m.reconcilePlan()
	if first == m.planState.doc {
		t.Fatal("edited content did not invalidate parser")
	}
}

func TestPlanAggregateOnlyHasExplicitMissingDetail(t *testing.T) {
	m := planModel(t)
	body, err := os.ReadFile("../plan/testdata/aggregate-no-changes.txt")
	if err != nil {
		t.Fatal(err)
	}
	m.prs[0].Comments[0].Body = string(body)
	m.reconcilePlan()
	m = pressKey(m, "4")
	report, _ := m.selectedPlanReport()
	m.planState.nodeID = report.Projects[0].ID
	if !strings.Contains(m.currentDiffContent(), "individual project output is unavailable") {
		t.Fatal("aggregate invented project detail")
	}
}

func TestPlanReaderCommandDoesNotShadowAtlantisPlanWorkflow(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["workflow-plan"] = config.ActionConfig{Operation: "comment", Comment: "atlantis plan", Commands: []string{"plan"}}
	registry, err := newActionRegistry(cfg.Actions)
	if err != nil {
		t.Fatal(err)
	}
	if action := registry.commands["plan"]; action.ID != "workflow-plan" {
		t.Fatal("reader shadows user workflow")
	}
	if action := registry.commands["summary-plan"]; action.ID != "summary-plan" || action.Operation != "summary-plan" {
		t.Fatal("reader missing safe command")
	}
}

func TestPlanPendingEntryHonorsLaterNavigation(t *testing.T) {
	for _, action := range []string{"", "3", "tab"} {
		t.Run("after-"+action, func(t *testing.T) {
			m := planModel(t)
			fresh := m.prs[0]
			m.prs[0].Comments = nil
			m.prs[0].ConversationCommentsLoaded = false
			m.allPRs = append([]model.PullRequest(nil), m.prs...)
			m.reconcilePlan()
			m = pressKey(m, "4")
			if action != "" {
				m = pressKey(m, action)
			}
			next, _ := m.Update(refreshPRMsg{repo: fresh.Repo, number: fresh.Number, pr: fresh})
			m = next.(Model)
			if (m.summaryMode == SummaryModePlan) != (action == "") {
				t.Fatal("pending explicit intent not respected")
			}
		})
	}
}

func TestPlanLateHeadResponseCannotRollBackSelection(t *testing.T) {
	m := planModel(t)
	old := m.prs[0]
	m.prs[0].HeadRefOID = "head-two"
	m.allPRs = append([]model.PullRequest(nil), m.prs...)
	next, _ := m.Update(refreshPRMsg{repo: old.Repo, number: old.Number, pr: old, requestHead: old.HeadRefOID})
	m = next.(Model)
	if pr, _ := m.current(); pr.HeadRefOID != "head-two" {
		t.Fatal("late detail response rolled back head")
	}
}

func TestPlanSelectionRequiresConversationCompleteness(t *testing.T) {
	m := planModel(t)
	m.prs[0].ConversationCommentsLoaded = false
	if m.autoDetailsCmd() == nil {
		t.Fatal("legacy conversation metadata not upgraded")
	}
	if m.autoDetailsCmd() != nil {
		t.Fatal("duplicate selected-detail fetch")
	}
	m.prs[0].HeadRefOID = "head-two"
	if m.autoDetailsCmd() == nil {
		t.Fatal("new head reused stale fetch key")
	}
}
