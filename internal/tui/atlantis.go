package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/model"
)

func (m Model) atlantisEnabled() bool { return m.cfg.AtlantisEnabled() }

func (m Model) isAtlantisRepo() bool {
	if !m.atlantisEnabled() {
		return false
	}
	for _, pr := range m.prs {
		for _, c := range pr.Checks {
			if strings.HasPrefix(c.Context, "atlantis/") {
				return true
			}
		}
	}
	return false
}

func (m Model) atlantisRow(pr model.PullRequest, innerW int) string {
	s := m.theme.Styles
	a := model.DeriveAtlantis(pr.Checks)
	dot := s.Muted.Render(" · ")

	plan := s.Muted.Render("no plan")
	switch a.Plan {
	case model.PlanRunning:
		plan = s.Pending.Render("plan running…") + atlantisProjects(a)
	case model.PlanPassed:
		plan = s.SuccessCI.Render("plan ✓") + atlantisProjects(a)
	case model.PlanFailed:
		plan = s.Error.Render("plan ✗") + atlantisProjects(a)
	}
	if !a.PlanAt.IsZero() {
		age := timeNow().Sub(a.PlanAt)
		if age < 0 {
			age = 0
		}
		plan += s.Muted.Render(" " + shortDuration(age) + " ago")
	}

	parts := []string{s.Muted.Render("atlantis"), plan}
	switch a.Policy {
	case model.PolicyRunning:
		parts = append(parts, s.Pending.Render("policy running…"))
	case model.PolicyPassed:
		parts = append(parts, s.SuccessCI.Render("policy ✓"))
	case model.PolicyFailed:
		parts = append(parts, s.Error.Render("policy ✗"))
	}

	apply := s.Muted.Render("apply –")
	switch a.Apply {
	case model.ApplyRunning:
		apply = s.Pending.Render("apply running…")
	case model.ApplyFailed:
		apply = s.Error.Render("apply ✗")
	case model.ApplyDone:
		apply = s.SuccessCI.Render("apply ✓")
	case model.ApplyNothing:
		apply = s.Muted.Render("nothing to apply")
	}
	parts = append(parts, apply)
	return truncatePaneLineWidth(strings.Join(parts, dot), innerW)
}

func atlantisProjects(a model.AtlantisSummary) string {
	switch {
	case a.PlanAffectedCount > 1:
		return fmt.Sprintf(" %s +%d", a.PlanProject, a.PlanAffectedCount-1)
	case a.PlanProject != "":
		return " " + a.PlanProject
	case a.Plan == model.PlanPassed && a.PlanProjectCount > 1:
		return fmt.Sprintf(" %d projects", a.PlanProjectCount)
	}
	return ""
}

type atlantisCommand int

const (
	atlantisCommandNone atlantisCommand = iota
	atlantisCommandPlan
	atlantisCommandApply
)

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
	}
	return atlantisCommandNone
}

// guardReasons returns why cmd should be blocked for pr; empty means allowed.
// postedAt is when zappr last posted `atlantis plan` for this PR and head commit.
func guardReasons(cmd atlantisCommand, pr model.PullRequest, postedAt, now time.Time) []string {
	if cmd == atlantisCommandNone {
		return nil
	}
	a := model.DeriveAtlantis(pr.Checks)
	applySucceeded := a.Apply == model.ApplyDone || a.Apply == model.ApplyNothing

	var reasons []string
	if atlantisAnyPending(pr.Checks) {
		reasons = append(reasons, "an Atlantis plan or apply is still running")
	}
	if atlantisPlanInFlight(pr.Checks, postedAt, now) {
		reasons = append(reasons, "a plan was just requested and has not reported yet")
	}
	if cmd == atlantisCommandApply {
		// A successful aggregate apply only waives a missing plan: Atlantis
		// leaves it untouched after later plans on the same commit.
		if a.Plan == model.PlanNone && !applySucceeded {
			reasons = append(reasons, "no Atlantis plan exists for the latest commit")
		}
		if a.Plan == model.PlanFailed {
			reasons = append(reasons, "the Atlantis plan failed")
		}
		if a.Policy == model.PolicyFailed {
			reasons = append(reasons, "the Atlantis policy check failed")
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

// guardComment is the single entry point for posting a comment that may be an
// Atlantis command. Non-Atlantis comments, or any comment when the guard is
// disabled, post exactly as before; Atlantis commands first refresh the PR.
func (m Model) guardComment(pr model.PullRequest, body string, action tuiAction, needsConfirm bool) (tea.Model, tea.Cmd) {
	cmd := classifyAtlantisComment(body)
	if !m.atlantisEnabled() || cmd == atlantisCommandNone {
		m.loading = true
		return m, tea.Batch(m.startStatus("commenting…"), commentCmd(m.app, pr, body))
	}
	if m.atlantisCheckInFlight() {
		return m, m.finishStatus("Atlantis check already in progress", true)
	}
	// Close the palette or comment editor now; the result lands where the
	// unguarded path would have ended up.
	switch m.mode {
	case ModeCommand, ModeConfirmCommand:
		m.mode = m.prevMode
		m.commandInput = ""
	case ModeComment:
		m.mode = ModeDetail
	}
	m.atlantisPending = atlantisGuard{pr: pr, body: body, cmd: cmd, confirm: needsConfirm, action: action, returnMode: m.mode}
	m.loading = true
	return m, tea.Batch(m.startStatus("checking Atlantis status…"), atlantisCheckCmd(m.app, pr))
}

func (m Model) handleAtlantisCheck(msg atlantisCheckMsg) (tea.Model, tea.Cmd) {
	g := m.atlantisPending
	if !m.atlantisCheckInFlight() || g.pr.Repo != msg.repo || g.pr.Number != msg.number {
		return m, nil
	}
	m.loading = false
	m.atlantisPending = atlantisGuard{}
	m.statusActive, m.statusText, m.statusErr = false, "", false

	if errors.Is(msg.err, app.ErrPRNotOpen) {
		return m, m.handlePRClosed(msg.repo, msg.number)
	}
	if msg.err == nil {
		// Keep the cache current even if the result is discarded below.
		m.updateCachedPR(msg.pr)
		m.applyPRSettings()
	}
	// The UI stays interactive during the check. If the user moved on (another
	// modal, mode, or PR), taking over would hijack their next keypress.
	cur, ok := m.current()
	if m.mode != g.returnMode || !ok || cur.Repo != g.pr.Repo || cur.Number != g.pr.Number {
		return m, m.finishStatus("Atlantis check discarded: view changed", false)
	}

	target := g.pr
	if msg.err != nil {
		g.reasons = []string{"could not verify Atlantis status: " + msg.err.Error()}
	} else {
		if msg.pr.HeadRefOID != "" {
			target.HeadRefOID = msg.pr.HeadRefOID
		}
		key := recentPlanKey{repo: msg.repo, number: msg.number, head: target.HeadRefOID}
		g.reasons = guardReasons(g.cmd, msg.pr, m.recentPlans[key], timeNow())
	}
	g.pr = target

	if len(g.reasons) > 0 {
		m.atlantisPending = g
		m.mode = ModeConfirmAtlantis
		return m, nil
	}
	if g.confirm {
		m.pendingAction = g.action
		m.prevMode = g.returnMode
		m.mode = ModeConfirmCommand
		return m, nil
	}
	return m.postGuardedComment(g, target)
}

// atlantisCheckInFlight reports whether a guard is waiting for its status
// check. A guard that already has reasons is (or was) shown as a dialog and
// may be replaced, so a dialog dismissed by a refresh cannot block new guards.
func (m Model) atlantisCheckInFlight() bool {
	return m.atlantisPending.cmd != atlantisCommandNone && len(m.atlantisPending.reasons) == 0
}

// postGuardedComment posts an Atlantis comment that passed or overrode the guard.
func (m Model) postGuardedComment(g atlantisGuard, pr model.PullRequest) (tea.Model, tea.Cmd) {
	if g.cmd == atlantisCommandPlan {
		m.recordRecentPlan(pr)
	}
	if g.action.ID == "" {
		// Free-text submissions set loading, matching the unguarded path.
		m.loading = true
	}
	return m, tea.Batch(m.startStatus("commenting…"), commentCmd(m.app, pr, g.body))
}

func (m *Model) recordRecentPlan(pr model.PullRequest) {
	if m.recentPlans == nil {
		m.recentPlans = map[recentPlanKey]time.Time{}
	}
	m.recentPlans[recentPlanKey{repo: pr.Repo, number: pr.Number, head: pr.HeadRefOID}] = timeNow()
}
