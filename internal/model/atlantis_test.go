package model

import (
	"testing"
	"time"
)

func TestDeriveAtlantis(t *testing.T) {
	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	status := func(context, state string) Check {
		return Check{Context: context, State: state, StartedAt: at}
	}

	tests := []struct {
		name   string
		checks []Check
		want   AtlantisSummary
	}{
		{name: "empty", checks: nil, want: AtlantisSummary{}},
		{
			name: "only non-atlantis check runs",
			checks: []Check{
				{Name: "build", WorkflowName: "CI", State: "SUCCESS"},
				{Name: "atlantis/plan", State: "SUCCESS"},
			},
			want: AtlantisSummary{},
		},
		{
			name: "aggregate plan with unknown state is no plan and has no time",
			checks: []Check{
				status("atlantis/plan", ""),
			},
			want: AtlantisSummary{HasAtlantis: true, Plan: PlanNone},
		},
		{
			name: "plan and policy passed, no apply",
			checks: []Check{
				status("atlantis/plan", "SUCCESS"),
				{Context: "atlantis/plan: sso-sso", State: "SUCCESS", StartedAt: at.Add(time.Minute)},
				status("atlantis/policy_check", "SUCCESS"),
			},
			want: AtlantisSummary{
				HasAtlantis:      true,
				Plan:             PlanPassed,
				PlanProject:      "sso-sso",
				PlanProjectCount: 1,
				PlanAt:           at,
				Policy:           PolicyPassed,
				Apply:            ApplyNone,
			},
		},
		{
			name:   "aggregate plan pending",
			checks: []Check{status("atlantis/plan", "PENDING")},
			want:   AtlantisSummary{HasAtlantis: true, Plan: PlanRunning, PlanAt: at},
		},
		{
			name:   "aggregate plan expected is running",
			checks: []Check{status("atlantis/plan", "EXPECTED")},
			want:   AtlantisSummary{HasAtlantis: true, Plan: PlanRunning, PlanAt: at},
		},
		{
			name: "single pending project plan is named",
			checks: []Check{
				status("atlantis/plan", "PENDING"),
				status("atlantis/plan: a", "SUCCESS"),
				status("atlantis/plan: b", "PENDING"),
			},
			want: AtlantisSummary{
				HasAtlantis:       true,
				Plan:              PlanRunning,
				PlanProject:       "b",
				PlanProjectCount:  2,
				PlanAffectedCount: 1,
				PlanAt:            at,
			},
		},
		{
			name: "multiple pending project plans name the first",
			checks: []Check{
				status("atlantis/plan", "PENDING"),
				status("atlantis/plan: a", "PENDING"),
				status("atlantis/plan: b", "PENDING"),
				status("atlantis/plan: c", "SUCCESS"),
			},
			want: AtlantisSummary{
				HasAtlantis:       true,
				Plan:              PlanRunning,
				PlanProject:       "a",
				PlanProjectCount:  3,
				PlanAffectedCount: 2,
				PlanAt:            at,
			},
		},
		{
			name: "multiple failed project plans name the first",
			checks: []Check{
				status("atlantis/plan", "FAILURE"),
				status("atlantis/plan: a", "SUCCESS"),
				status("atlantis/plan: b", "FAILURE"),
				status("atlantis/plan: c", "ERROR"),
				status("atlantis/plan: d", "SUCCESS"),
				status("atlantis/plan: e", "SUCCESS"),
			},
			want: AtlantisSummary{
				HasAtlantis:       true,
				Plan:              PlanFailed,
				PlanProject:       "b",
				PlanProjectCount:  5,
				PlanAffectedCount: 2,
				PlanAt:            at,
			},
		},
		{
			name: "failing project wins over successful aggregate",
			checks: []Check{
				status("atlantis/plan", "SUCCESS"),
				status("atlantis/plan: a", "FAILURE"),
				status("atlantis/plan: b", "SUCCESS"),
			},
			want: AtlantisSummary{
				HasAtlantis:       true,
				Plan:              PlanFailed,
				PlanProject:       "a",
				PlanProjectCount:  2,
				PlanAffectedCount: 1,
				PlanAt:            at,
			},
		},
		{
			name: "single errored project",
			checks: []Check{
				status("atlantis/plan", "ERROR"),
				status("atlantis/plan: a", "ERROR"),
			},
			want: AtlantisSummary{
				HasAtlantis:       true,
				Plan:              PlanFailed,
				PlanProject:       "a",
				PlanProjectCount:  1,
				PlanAffectedCount: 1,
				PlanAt:            at,
			},
		},
		{
			name: "two successful projects are unnamed",
			checks: []Check{
				status("atlantis/plan", "SUCCESS"),
				status("atlantis/plan: a", "SUCCESS"),
				status("atlantis/plan: b", "SUCCESS"),
			},
			want: AtlantisSummary{
				HasAtlantis:      true,
				Plan:             PlanPassed,
				PlanProjectCount: 2,
				PlanAt:           at,
			},
		},
		{
			name: "policy running and failed",
			checks: []Check{
				status("atlantis/plan", "SUCCESS"),
				status("atlantis/policy_check", "PENDING"),
			},
			want: AtlantisSummary{HasAtlantis: true, Plan: PlanPassed, PlanAt: at, Policy: PolicyRunning},
		},
		{
			name: "policy failed",
			checks: []Check{
				status("atlantis/plan", "SUCCESS"),
				status("atlantis/policy_check", "FAILURE"),
			},
			want: AtlantisSummary{HasAtlantis: true, Plan: PlanPassed, PlanAt: at, Policy: PolicyFailed},
		},
		{
			name:   "aggregate apply pending",
			checks: []Check{status("atlantis/apply", "PENDING")},
			want:   AtlantisSummary{HasAtlantis: true, Apply: ApplyRunning},
		},
		{
			name:   "aggregate apply failure",
			checks: []Check{status("atlantis/apply", "FAILURE")},
			want:   AtlantisSummary{HasAtlantis: true, Apply: ApplyFailed},
		},
		{
			name: "per-project apply failure",
			checks: []Check{
				status("atlantis/apply", "SUCCESS"),
				status("atlantis/apply: sso-sso", "ERROR"),
			},
			want: AtlantisSummary{HasAtlantis: true, Apply: ApplyFailed},
		},
		{
			name: "apply done with project apply",
			checks: []Check{
				status("atlantis/apply", "SUCCESS"),
				status("atlantis/apply: sso-sso", "SUCCESS"),
			},
			want: AtlantisSummary{HasAtlantis: true, Apply: ApplyDone},
		},
		{
			name:   "apply success without project apply is nothing to apply",
			checks: []Check{status("atlantis/apply", "SUCCESS")},
			want:   AtlantisSummary{HasAtlantis: true, Apply: ApplyNothing},
		},
		{
			name:   "per-project apply without aggregate is none",
			checks: []Check{status("atlantis/apply: sso-sso", "SUCCESS")},
			want:   AtlantisSummary{HasAtlantis: true, Apply: ApplyNone},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveAtlantis(tt.checks); got != tt.want {
				t.Fatalf("DeriveAtlantis() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
