package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/model"
	"github.com/senilio/zappr/internal/plan"
)

type planOpenRequest struct {
	prKey, head string
	token       uint64
}

// Parsed documents are immutable and shared by Model values.
type planReaderState struct {
	prKey, fingerprint string
	doc                *plan.Document
	reportID, nodeID   string
	pendingOpen        *planOpenRequest
}

func readerPRKey(pr model.PullRequest) string { return fmt.Sprintf("%s#%d", pr.Repo, pr.Number) }

// reconcilePlan runs on selected-source changes, never during rendering/ticks.
func (m *Model) reconcilePlan() {
	m.reconcileComments()
	pr, ok := m.current()
	if !ok || !m.atlantisEnabled() {
		m.planState = planReaderState{}
		if m.summaryMode == SummaryModePlan {
			m.summaryMode = SummaryModeFiles
		}
		return
	}
	key := readerPRKey(pr)
	changedPR := m.planState.prKey != key
	if changedPR {
		m.readerIntentToken++
		m.planState = planReaderState{prKey: key}
	}
	fingerprint := plan.Fingerprint(pr.Comments, pr.ConversationCommentsLoaded)
	if m.planState.doc == nil || m.planState.fingerprint != fingerprint {
		doc := plan.Parse(pr.Comments, pr.ConversationCommentsLoaded)
		m.planState.doc, m.planState.fingerprint = &doc, fingerprint
		if m.planState.reportID == "" && len(doc.Reports) > 0 {
			m.planState.reportID = doc.Reports[0].ID
			m.planState.nodeID = doc.Reports[0].ID + "/summary"
		}
		if report, found := m.selectedPlanReport(); found {
			if _, exists := m.selectedPlanNode(); !exists {
				m.planState.nodeID = report.ID + "/summary"
			}
		}
	}
	if changedPR && !m.planAvailable() && m.summaryMode == SummaryModePlan {
		m.summaryMode = SummaryModeFiles
	}
	if request := m.planState.pendingOpen; request != nil {
		if request.prKey != key || request.head != pr.HeadRefOID || request.token != m.readerIntentToken {
			m.planState.pendingOpen = nil
		} else if m.planAvailable() {
			m.planState.pendingOpen = nil
			m.enterPlan()
		} else if pr.ConversationCommentsLoaded {
			m.planState.pendingOpen = nil
			m.message = "no supported plan report found"
		}
	}
}

func (m Model) planAvailable() bool {
	return m.atlantisEnabled() && m.planForCurrentPR() && len(m.planState.doc.Reports) > 0
}

// planForCurrentPR guards against showing a report parsed for a PR that is no
// longer selected, for example after it closed or was filtered out.
func (m Model) planForCurrentPR() bool {
	pr, ok := m.current()
	return ok && m.planState.doc != nil && m.planState.prKey == readerPRKey(pr)
}

func (m Model) selectedPlanReport() (plan.Report, bool) {
	if m.planForCurrentPR() {
		for _, report := range m.planState.doc.Reports {
			if report.ID == m.planState.reportID {
				return report, true
			}
		}
	}
	return plan.Report{}, false
}

func (m Model) planNodes() []plan.Node {
	if m.planState.doc == nil {
		return nil
	}
	expanded := make(map[string]bool)
	if report, ok := m.selectedPlanReport(); ok {
		for _, project := range report.Projects {
			expanded[project.ID] = true
		}
	}
	return plan.Outline(*m.planState.doc, m.planState.reportID, expanded)
}

func (m Model) selectedPlanNode() (plan.Node, bool) {
	for _, node := range m.planNodes() {
		if node.ID == m.planState.nodeID {
			return node, true
		}
	}
	return plan.Node{}, false
}

func (m Model) planNodeIndex() int {
	for i, node := range m.planNodes() {
		if node.ID == m.planState.nodeID {
			return i
		}
	}
	return 0
}

func (m *Model) cancelPlanOpen() {
	m.readerIntentToken++
	m.planState.pendingOpen = nil
}

func (m *Model) enterPlan() {
	if _, ok := m.selectedPlanReport(); !ok && m.planState.doc != nil && len(m.planState.doc.Reports) > 0 {
		m.planState.reportID = m.planState.doc.Reports[0].ID
	}
	m.summaryMode, m.mode, m.focus = SummaryModePlan, ModeDetail, FocusSummary
	m.planState.nodeID = m.planState.reportID + "/summary"
	m.summaryVP.GotoTop()
	m.diffVP.GotoTop()
	m.summaryXOffset, m.diffXOffset = 0, 0
}

func (m *Model) openPlan() tea.Cmd {
	m.reconcilePlan()
	m.cancelPlanOpen()
	pr, ok := m.current()
	if !ok || !m.atlantisEnabled() {
		m.message = "plan reader unavailable"
		return nil
	}
	if m.planAvailable() {
		m.enterPlan()
		return m.autoDetailsCmd()
	}
	if pr.ConversationCommentsLoaded {
		m.message = "no supported plan report found"
		return nil
	}
	m.planState.pendingOpen = &planOpenRequest{readerPRKey(pr), pr.HeadRefOID, m.readerIntentToken}
	m.message = "loading plan comments…"
	return m.autoDetailsCmd()
}

func (m *Model) movePlan(delta int) {
	nodes := m.planNodes()
	next := m.planNodeIndex() + delta
	if len(nodes) == 0 {
		return
	}
	next = max(0, min(next, len(nodes)-1))
	m.planState.nodeID = nodes[next].ID
	m.diffVP.GotoTop()
	m.diffXOffset = 0
	m.syncSummaryVP()
	m.syncSummaryVPScroll()
	m.syncDiffVP()
}

func (m *Model) pagePlan(direction int) {
	pr, ok := m.current()
	if !ok {
		return
	}
	_, spans := m.summaryLayout(pr, m.summaryVP.Width())
	index := m.planNodeIndex()
	if len(spans) == 0 || index >= len(spans) {
		return
	}
	target := spans[index].start + direction*max(1, m.summaryVP.Height())
	next := index
	for next+direction >= 0 && next+direction < len(spans) {
		next += direction
		if direction > 0 && spans[next].start >= target || direction < 0 && spans[next].start <= target {
			break
		}
	}
	m.movePlan(next - index)
}

func (m *Model) adoptPlanReport(id string) {
	if m.planState.doc == nil {
		return
	}
	for _, report := range m.planState.doc.Reports {
		if report.ID == id {
			m.planState.reportID = id
			m.planState.nodeID = id + "/summary"
			m.movePlan(0)
			return
		}
	}
}

func planOutcome(outcome plan.Outcome) string {
	if outcome == plan.OutcomeNoChanges {
		return "no changes"
	}
	if outcome == "" {
		return "unknown"
	}
	return strings.ReplaceAll(string(outcome), "-", " ")
}
