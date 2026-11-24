package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/senilio/zappr/internal/model"
	"github.com/senilio/zappr/internal/plan"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func (m Model) atlantisEnabled() bool { return m.cfg.AtlantisEnabled() }

func (m Model) isAtlantisRepo() bool {
	if !m.atlantisEnabled() {
		return false
	}
	if pr, ok := m.current(); ok && m.planState.prKey == readerPRKey(pr) && m.planAvailable() {
		return true
	}
	for _, pr := range m.prs {
		if model.DeriveAtlantis(pr.Checks, pr.Comments...).HasAtlantis {
			return true
		}
	}
	return false
}

func (m Model) hasAtlantisActivity(pr model.PullRequest) bool {
	if !m.atlantisEnabled() {
		return false
	}
	if m.planState.prKey == readerPRKey(pr) && m.planAvailable() {
		return true
	}
	return model.DeriveAtlantis(pr.Checks, pr.Comments...).HasAtlantis
}

// Check phases are execution facts, never evidence about plan contents.
func atlantisProgress(a model.AtlantisSummary) (string, uitheme.PillState, string) {
	switch {
	case a.Apply == model.ApplyFailed:
		return "APPLY FAILED", uitheme.PillFailed, ""
	case a.Plan == model.PlanFailed:
		note := a.PlanProject
		if a.PlanAffectedCount > 1 {
			note += fmt.Sprintf(" +%d", a.PlanAffectedCount-1)
		}
		return "PLAN FAILED", uitheme.PillFailed, note
	case a.Policy == model.PolicyFailed:
		return "POLICY FAILED", uitheme.PillFailed, ""
	case a.Apply == model.ApplyRunning:
		return "APPLYING", uitheme.PillWait, ""
	case a.Plan == model.PlanRunning || a.Policy == model.PolicyRunning:
		return "PLANNING", uitheme.PillWait, ""
	case a.Apply == model.ApplyDone:
		return "APPLIED", uitheme.PillPassed, ""
	case a.Apply == model.ApplyPartial:
		return "PARTLY APPLIED", uitheme.PillWait, ""
	case a.Plan == model.PlanPassed:
		return "PLAN OK", uitheme.PillPassed, ""
	case a.Plan == model.PlanDiscarded:
		return "PLAN DISCARDED", uitheme.PillWait, ""
	}
	return "", uitheme.PillNone, ""
}

func (m Model) atlantisRow(pr model.PullRequest, innerW int) string {
	a := model.DeriveAtlantis(pr.Checks, pr.Comments...)
	label, state, _ := atlantisProgress(a)
	report, found := m.selectedPlanReport()
	if found && m.planState.prKey == readerPRKey(pr) {
		if label == "" {
			switch {
			case report.Outcome == plan.OutcomeFailed:
				label, state = "PLAN FAILED", uitheme.PillFailed
			case report.SourceComplete && (report.Outcome == plan.OutcomeChanges || report.Outcome == plan.OutcomeNoChanges):
				label, state = "PLAN OK", uitheme.PillPassed
			}
		}
	}
	if label == "" {
		label = "plan unknown"
	}
	row := m.theme.Pill(strings.ToLower(label), state, m.pillShape(), m.cfg.EffectivePillOpacity())
	return truncatePaneLineWidth(row, innerW)
}

type atlantisCommand int

const (
	atlantisCommandNone atlantisCommand = iota
	atlantisCommandPlan
	atlantisCommandApply
	atlantisCommandCancel
	atlantisCommandUnlock
)

func (c atlantisCommand) label() string {
	switch c {
	case atlantisCommandPlan:
		return "plan"
	case atlantisCommandApply:
		return "apply"
	case atlantisCommandCancel:
		return "cancel"
	case atlantisCommandUnlock:
		return "unlock"
	default:
		return "refresh"
	}
}

func (c atlantisCommand) guarded() bool {
	return c == atlantisCommandPlan || c == atlantisCommandApply
}

const atlantisRecentPlanWindow = 2 * time.Minute

type recentPlanKey struct {
	repo   string
	number int
	head   string
}

func classifyAtlantisComment(body string) atlantisCommand {
	words := strings.Fields(body)
	if len(words) < 2 || !strings.EqualFold(words[0], "atlantis") {
		return atlantisCommandNone
	}
	switch strings.ToLower(words[1]) {
	case "plan":
		return atlantisCommandPlan
	case "apply":
		return atlantisCommandApply
	case "cancel":
		return atlantisCommandCancel
	case "unlock":
		return atlantisCommandUnlock
	}
	return atlantisCommandNone
}

type atlantisFinding string

const (
	atlantisStillRunning   atlantisFinding = "an Atlantis plan or apply is still running"
	atlantisJustRequested  atlantisFinding = "a plan was just requested and has not reported yet"
	atlantisDiscarded      atlantisFinding = "the Atlantis plan was discarded by an unlock"
	atlantisNoPlan         atlantisFinding = "no Atlantis plan exists for the latest commit"
	atlantisPlanFailed     atlantisFinding = "the Atlantis plan failed"
	atlantisPolicyFailed   atlantisFinding = "the Atlantis policy check failed"
	atlantisEvidenceFailed atlantisFinding = "could not verify Atlantis status"
)

type guardReason struct {
	finding atlantisFinding
	detail  string
}

func (r guardReason) text() string {
	if r.detail != "" {
		return string(r.finding) + ": " + r.detail
	}
	return string(r.finding)
}

// guardReasons returns affirmative reasons to block and, only with complete
// evidence, a missing-plan reason. Empty reasons alone do not prove it is safe.
// postedAt is when zappr last posted `atlantis plan` for this PR and head commit.
func guardReasons(cmd atlantisCommand, pr model.PullRequest, postedAt, now time.Time, evidenceComplete bool) []guardReason {
	if !cmd.guarded() {
		return nil
	}
	a := model.DeriveAtlantis(pr.Checks, pr.Comments...)
	applySucceeded := a.Apply == model.ApplyDone || a.Apply == model.ApplyNothing

	var reasons []guardReason
	add := func(finding atlantisFinding) {
		reasons = append(reasons, guardReason{finding: finding})
	}
	if atlantisAnyPending(pr.Checks) {
		add(atlantisStillRunning)
	}
	if atlantisPlanInFlight(pr.Checks, postedAt, now) {
		add(atlantisJustRequested)
	}
	if cmd == atlantisCommandApply {
		if a.Plan == model.PlanDiscarded {
			add(atlantisDiscarded)
		}
		// A successful aggregate apply only waives a missing plan: Atlantis
		// leaves it untouched after later plans on the same commit.
		if evidenceComplete && a.Plan == model.PlanNone && !applySucceeded {
			add(atlantisNoPlan)
		}
		if a.Plan == model.PlanFailed {
			add(atlantisPlanFailed)
		}
		if a.Policy == model.PolicyFailed {
			add(atlantisPolicyFailed)
		}
	}
	return reasons
}

func atlantisAnyPending(checks []model.Check) bool {
	for _, c := range checks {
		if strings.HasPrefix(c.Context, "atlantis/") && (c.State == "PENDING" || c.State == "EXPECTED") {
			return true
		}
	}
	return false
}

func atlantisPlanInFlight(checks []model.Check, postedAt, now time.Time) bool {
	if postedAt.IsZero() || now.Sub(postedAt) >= atlantisRecentPlanWindow {
		return false
	}
	for _, c := range checks {
		isPlan := c.Context == "atlantis/plan" || strings.HasPrefix(c.Context, "atlantis/plan: ")
		if isPlan && c.StartedAt.After(postedAt) {
			return false
		}
	}
	return true
}

func (m *Model) recordRecentPlan(pr model.PullRequest) {
	if m.recentPlans == nil {
		m.recentPlans = map[recentPlanKey]time.Time{}
	}
	m.recentPlans[recentPlanKey{repo: pr.Repo, number: pr.Number, head: pr.HeadRefOID}] = timeNow()
}
