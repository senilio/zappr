package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/model"
)

func atlantisChecks(planState string, planAt time.Time) []model.Check {
	return []model.Check{
		{Context: "atlantis/plan", State: planState, StartedAt: planAt},
		{Context: "atlantis/plan: sso-sso", State: planState, StartedAt: planAt},
		{Context: "atlantis/policy_check", State: "SUCCESS"},
	}
}

func TestAtlantisRowContent(t *testing.T) {
	pr := model.PullRequest{Number: 1, Checks: atlantisChecks("SUCCESS", time.Now().Add(-4*time.Minute))}
	s := model.DeriveAtlantis(pr.Checks)
	if s.Plan != model.PlanPassed || s.PlanProject != "sso-sso" || s.Policy != model.PolicyPassed || s.Apply != model.ApplyNone {
		t.Fatalf("unexpected summary: %+v", s)
	}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	row := stripANSI(m.atlantisRow(pr, 80))
	for _, want := range []string{"atlantis", "plan", "sso-sso", "4m", "policy", "apply"} {
		if !strings.Contains(row, want) {
			t.Errorf("row %q missing %q", row, want)
		}
	}
}

func TestAtlantisRowNoPlan(t *testing.T) {
	pr := model.PullRequest{Number: 1}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	row := stripANSI(m.atlantisRow(pr, 80))
	if !strings.Contains(row, "no plan") || !strings.Contains(row, "apply –") {
		t.Errorf("row %q missing no plan / apply –", row)
	}
	if strings.Contains(row, "ago") || strings.Contains(row, "policy") {
		t.Errorf("row %q must not contain age or policy", row)
	}
}

func TestAtlantisRowMultipleProjects(t *testing.T) {
	at := time.Now().Add(-time.Minute)
	tests := []struct {
		name   string
		states []string
		want   string
	}{
		{"failed names first affected", []string{"SUCCESS", "FAILURE", "ERROR", "SUCCESS", "SUCCESS"}, "plan ✗ p1 +1"},
		{"running names first affected", []string{"PENDING", "PENDING", "PENDING", "SUCCESS"}, "plan running… p0 +2 1m ago"},
		{"single failed is named", []string{"SUCCESS", "FAILURE"}, "plan ✗ p1 "},
		{"passed shows count", []string{"SUCCESS", "SUCCESS", "SUCCESS"}, "plan ✓ 3 projects"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggState := "SUCCESS"
			checks := []model.Check{}
			for i, st := range tt.states {
				checks = append(checks, model.Check{Context: fmt.Sprintf("atlantis/plan: p%d", i), State: st, StartedAt: at})
				if st != "SUCCESS" {
					aggState = st
				}
			}
			checks = append(checks, model.Check{Context: "atlantis/plan", State: aggState, StartedAt: at})
			pr := model.PullRequest{Number: 1, Checks: checks}
			m := modelWithPRs([]model.PullRequest{pr}, 0)
			row := stripANSI(m.atlantisRow(pr, 120))
			if !strings.Contains(row, tt.want) {
				t.Errorf("row %q missing %q", row, tt.want)
			}
		})
	}
}

func TestAtlantisRowFuturePlanTimeClampsToZero(t *testing.T) {
	pr := model.PullRequest{Number: 1, Checks: atlantisChecks("SUCCESS", time.Now().Add(time.Hour))}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	row := stripANSI(m.atlantisRow(pr, 80))
	if !strings.Contains(row, " 0s ago") || strings.Contains(row, " -") {
		t.Errorf("row %q should show 0s ago without a negative age", row)
	}
}

func TestAtlantisRowTruncatesToWidth(t *testing.T) {
	pr := model.PullRequest{Number: 1, Checks: atlantisChecks("SUCCESS", time.Now().Add(-time.Minute))}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	if got := len([]rune(stripANSI(m.atlantisRow(pr, 12)))); got > 12 {
		t.Errorf("row width %d exceeds 12", got)
	}
}

func TestIsAtlantisRepoRequiresEnabledAndStatus(t *testing.T) {
	atlantis := model.PullRequest{Number: 1, Checks: atlantisChecks("SUCCESS", time.Now())}
	plain := model.PullRequest{Number: 2}
	m := modelWithPRs([]model.PullRequest{plain, atlantis}, 0)
	if !m.isAtlantisRepo() {
		t.Error("repo with atlantis status should be an atlantis repo")
	}
	disabled := false
	m.cfg.Atlantis.Enabled = &disabled
	if m.isAtlantisRepo() {
		t.Error("disabled config must not be an atlantis repo")
	}
	m = modelWithPRs([]model.PullRequest{plain}, 0)
	if m.isAtlantisRepo() {
		t.Error("repo without atlantis statuses must not be an atlantis repo")
	}
}

func TestClassifyAtlantisComment(t *testing.T) {
	tests := []struct {
		body string
		want atlantisCommand
	}{
		{"atlantis plan", atlantisCommandPlan},
		{"  Atlantis   PLAN -p sso", atlantisCommandPlan},
		{"atlantis apply -d foo", atlantisCommandApply},
		{"atlantis unlock", atlantisCommandNone},
		{"please atlantis plan", atlantisCommandNone},
		{"", atlantisCommandNone},
		{"atlantisplan", atlantisCommandNone},
	}
	for _, tt := range tests {
		if got := classifyAtlantisComment(tt.body); got != tt.want {
			t.Errorf("classifyAtlantisComment(%q) = %v, want %v", tt.body, got, tt.want)
		}
	}
}

func TestGuardReasons(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	passed := func(policy string) []model.Check {
		return []model.Check{
			{Context: "atlantis/plan", State: "SUCCESS", StartedAt: now.Add(-10 * time.Minute)},
			{Context: "atlantis/plan: sso", State: "SUCCESS", StartedAt: now.Add(-10 * time.Minute)},
			{Context: "atlantis/policy_check", State: policy},
		}
	}
	with := func(checks []model.Check, extra ...model.Check) []model.Check {
		return append(append([]model.Check{}, checks...), extra...)
	}
	tests := []struct {
		name     string
		cmd      atlantisCommand
		checks   []model.Check
		postedAt time.Time
		want     string
	}{
		{"plan pending", atlantisCommandPlan, []model.Check{{Context: "atlantis/plan", State: "PENDING"}}, time.Time{}, "running"},
		{"plan expected counts as pending", atlantisCommandPlan, []model.Check{{Context: "atlantis/plan: sso", State: "EXPECTED"}}, time.Time{}, "running"},
		{"plan just requested", atlantisCommandPlan, passed("SUCCESS"), now.Add(-time.Minute), "just requested"},
		{"plan outside window", atlantisCommandPlan, nil, now.Add(-3 * time.Minute), ""},
		{"plan reported after post", atlantisCommandPlan, []model.Check{{Context: "atlantis/plan", State: "SUCCESS", StartedAt: now.Add(-30 * time.Second)}}, now.Add(-time.Minute), ""},
		{"plan idle", atlantisCommandPlan, passed("SUCCESS"), time.Time{}, ""},
		{"apply no plan", atlantisCommandApply, nil, time.Time{}, "no Atlantis plan"},
		{"apply pending", atlantisCommandApply, with(passed("SUCCESS"), model.Check{Context: "atlantis/apply: sso", State: "PENDING"}), time.Time{}, "running"},
		{"apply project plan failed", atlantisCommandApply, []model.Check{
			{Context: "atlantis/plan", State: "SUCCESS"},
			{Context: "atlantis/plan: sso", State: "FAILURE"},
		}, time.Time{}, "failed"},
		{"apply policy failed", atlantisCommandApply, passed("FAILURE"), time.Time{}, "policy"},
		{"apply plan in flight", atlantisCommandApply, passed("SUCCESS"), now.Add(-time.Minute), "just requested"},
		{"apply already done", atlantisCommandApply, with(passed("SUCCESS"), model.Check{Context: "atlantis/apply", State: "SUCCESS"}), time.Time{}, ""},
		{"apply success with running plan", atlantisCommandApply, []model.Check{
			{Context: "atlantis/apply", State: "SUCCESS"},
			{Context: "atlantis/plan", State: "PENDING"},
		}, time.Time{}, "running"},
		{"apply success with plan in flight", atlantisCommandApply, []model.Check{
			{Context: "atlantis/apply", State: "SUCCESS"},
			{Context: "atlantis/plan", State: "SUCCESS", StartedAt: now.Add(-10 * time.Minute)},
		}, now.Add(-time.Minute), "just requested"},
		{"apply success with failed plan", atlantisCommandApply, []model.Check{
			{Context: "atlantis/apply", State: "SUCCESS"},
			{Context: "atlantis/plan", State: "FAILURE"},
		}, time.Time{}, "failed"},
		{"apply success without plan", atlantisCommandApply, []model.Check{{Context: "atlantis/apply", State: "SUCCESS"}}, time.Time{}, ""},
		{"apply ready", atlantisCommandApply, passed("SUCCESS"), time.Time{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := model.PullRequest{Number: 1, Checks: tt.checks}
			reasons := guardReasons(tt.cmd, pr, tt.postedAt, now)
			if tt.want == "" {
				if len(reasons) != 0 {
					t.Fatalf("expected allowed, got %v", reasons)
				}
				return
			}
			if len(reasons) == 0 {
				t.Fatalf("expected block containing %q, got allowed", tt.want)
			}
			if !strings.Contains(strings.Join(reasons, "; "), tt.want) {
				t.Errorf("reasons %v missing %q", reasons, tt.want)
			}
		})
	}
}
