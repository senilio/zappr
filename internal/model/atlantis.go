package model

import (
	"strings"
	"time"
)

type PlanState int

const (
	PlanNone PlanState = iota
	PlanRunning
	PlanPassed
	PlanFailed
)

type PolicyState int

const (
	PolicyNone PolicyState = iota
	PolicyRunning
	PolicyPassed
	PolicyFailed
)

type ApplyState int

const (
	ApplyNone ApplyState = iota
	ApplyRunning
	ApplyFailed
	ApplyDone
	ApplyNothing
)

// AtlantisSummary is comparable so the inbox layout cache can compare it with ==.
type AtlantisSummary struct {
	HasAtlantis bool
	Plan        PlanState
	// PlanProject names the single planned project when passed, or the first
	// affected project when running or failed.
	PlanProject      string
	PlanProjectCount int
	// PlanAffectedCount is the number of running or failed projects.
	PlanAffectedCount int
	PlanAt            time.Time
	Policy            PolicyState
	Apply             ApplyState
}

const (
	atlantisPrefix      = "atlantis/"
	atlantisPlan        = "atlantis/plan"
	atlantisPolicyCheck = "atlantis/policy_check"
	atlantisApply       = "atlantis/apply"
	atlantisProjectSep  = ": "
)

type atlantisStatus struct {
	project string
	state   string
	at      time.Time
}

// DeriveAtlantis summarizes Atlantis commit statuses found in checks.
func DeriveAtlantis(checks []Check) AtlantisSummary {
	var s AtlantisSummary
	var planAgg, policyAgg, applyAgg *atlantisStatus
	var planProjects, applyProjects []atlantisStatus

	for _, c := range checks {
		if !strings.HasPrefix(c.Context, atlantisPrefix) {
			continue
		}
		s.HasAtlantis = true
		st := atlantisStatus{state: c.State, at: c.StartedAt}
		switch {
		case c.Context == atlantisPlan:
			planAgg = &st
		case c.Context == atlantisPolicyCheck:
			policyAgg = &st
		case c.Context == atlantisApply:
			applyAgg = &st
		case strings.HasPrefix(c.Context, atlantisPlan+atlantisProjectSep):
			st.project = strings.TrimPrefix(c.Context, atlantisPlan+atlantisProjectSep)
			planProjects = append(planProjects, st)
		case strings.HasPrefix(c.Context, atlantisApply+atlantisProjectSep):
			st.project = strings.TrimPrefix(c.Context, atlantisApply+atlantisProjectSep)
			applyProjects = append(applyProjects, st)
		}
	}
	if !s.HasAtlantis {
		return s
	}

	s.deriveApply(applyAgg, applyProjects)
	s.derivePolicy(policyAgg)
	s.derivePlan(planAgg, planProjects)
	return s
}

func isPending(state string) bool { return state == "PENDING" || state == "EXPECTED" }

func isFailed(state string) bool { return state == "FAILURE" || state == "ERROR" }

func (s *AtlantisSummary) derivePlan(agg *atlantisStatus, projects []atlantisStatus) {
	var pending, failed []string
	for _, p := range projects {
		switch {
		case isPending(p.state):
			pending = append(pending, p.project)
		case isFailed(p.state):
			failed = append(failed, p.project)
		}
	}
	anyPending := len(pending) > 0 || (agg != nil && isPending(agg.state))
	anyFailed := len(failed) > 0 || (agg != nil && isFailed(agg.state))

	switch {
	case anyPending:
		s.Plan = PlanRunning
	case anyFailed:
		s.Plan = PlanFailed
	case agg != nil && agg.state == "SUCCESS":
		s.Plan = PlanPassed
	default:
		return
	}

	if agg != nil {
		s.PlanAt = agg.at
	}
	s.PlanProjectCount = len(projects)
	var affected []string
	switch s.Plan {
	case PlanRunning:
		affected = pending
	case PlanFailed:
		affected = failed
	case PlanPassed:
		if len(projects) == 1 {
			s.PlanProject = projects[0].project
		}
	}
	if len(affected) > 0 {
		s.PlanProject = affected[0]
		s.PlanAffectedCount = len(affected)
	}
}

func (s *AtlantisSummary) derivePolicy(agg *atlantisStatus) {
	if agg == nil {
		return
	}
	switch {
	case isPending(agg.state):
		s.Policy = PolicyRunning
	case isFailed(agg.state):
		s.Policy = PolicyFailed
	case agg.state == "SUCCESS":
		s.Policy = PolicyPassed
	}
}

func (s *AtlantisSummary) deriveApply(agg *atlantisStatus, projects []atlantisStatus) {
	if agg == nil {
		return
	}
	pending, failed := isPending(agg.state), isFailed(agg.state)
	for _, p := range projects {
		pending = pending || isPending(p.state)
		failed = failed || isFailed(p.state)
	}
	switch {
	case pending:
		s.Apply = ApplyRunning
	case failed:
		s.Apply = ApplyFailed
	case agg.state == "SUCCESS" && len(projects) > 0:
		s.Apply = ApplyDone
	case agg.state == "SUCCESS":
		s.Apply = ApplyNothing
	}
}
