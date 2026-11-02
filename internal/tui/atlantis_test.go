package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func atlantisChecks(planState string, planAt time.Time) []model.Check {
	return []model.Check{
		{Context: "atlantis/plan", State: planState, StartedAt: planAt},
		{Context: "atlantis/plan: sso-sso", State: planState, StartedAt: planAt},
		{Context: "atlantis/policy_check", State: "SUCCESS"},
	}
}

func TestAtlantisProgress(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	planAt := now.Add(-6 * time.Minute)
	passed := model.AtlantisSummary{HasAtlantis: true, Plan: model.PlanPassed, PlanAt: planAt}
	with := func(a model.AtlantisSummary, f func(*model.AtlantisSummary)) model.AtlantisSummary {
		f(&a)
		return a
	}
	cases := []struct {
		name  string
		a     model.AtlantisSummary
		label string
		state uitheme.PillState
		note  string
	}{
		{"apply failed", with(passed, func(a *model.AtlantisSummary) { a.Apply = model.ApplyFailed }), "APPLY FAILED", uitheme.PillFailed, ""},
		{"plan failed", model.AtlantisSummary{Plan: model.PlanFailed, PlanProject: "a", PlanAffectedCount: 2, PlanAt: planAt}, "PLAN FAILED", uitheme.PillFailed, "a +1"},
		{"policy failed", with(passed, func(a *model.AtlantisSummary) { a.Policy = model.PolicyFailed }), "POLICY FAILED", uitheme.PillFailed, ""},
		{"apply running", with(passed, func(a *model.AtlantisSummary) { a.Apply = model.ApplyRunning }), "APPLYING", uitheme.PillWait, ""},
		{"plan running", model.AtlantisSummary{Plan: model.PlanRunning, PlanAt: planAt}, "PLANNING", uitheme.PillWait, ""},
		{"policy running", with(passed, func(a *model.AtlantisSummary) { a.Policy = model.PolicyRunning }), "PLANNING", uitheme.PillWait, ""},
		{"passed 12 projects", with(passed, func(a *model.AtlantisSummary) { a.PlanProjectCount = 12 }), "PLAN OK", uitheme.PillPassed, ""},
		{"passed 1 project", with(passed, func(a *model.AtlantisSummary) { a.PlanProjectCount, a.PlanProject = 1, "sso-sso" }), "PLAN OK", uitheme.PillPassed, ""},
		{"applied", with(passed, func(a *model.AtlantisSummary) { a.Apply = model.ApplyDone }), "APPLIED", uitheme.PillPassed, ""},
		{"nothing to apply is not content evidence", with(passed, func(a *model.AtlantisSummary) { a.Apply = model.ApplyNothing }), "PLAN OK", uitheme.PillPassed, ""},
		{"plan failed beats apply done", model.AtlantisSummary{Plan: model.PlanFailed, Apply: model.ApplyDone}, "PLAN FAILED", uitheme.PillFailed, ""},
		{"apply failure beats plan failure", model.AtlantisSummary{Plan: model.PlanFailed, Apply: model.ApplyFailed}, "APPLY FAILED", uitheme.PillFailed, ""},
		{"plan failure beats apply running", model.AtlantisSummary{Plan: model.PlanFailed, Apply: model.ApplyRunning}, "PLAN FAILED", uitheme.PillFailed, ""},
		{"policy failure beats plan running", model.AtlantisSummary{Plan: model.PlanRunning, Policy: model.PolicyFailed}, "POLICY FAILED", uitheme.PillFailed, ""},
		{"none", model.AtlantisSummary{}, "", uitheme.PillNone, ""},
		{"future plan", with(passed, func(a *model.AtlantisSummary) { a.PlanAt = now.Add(time.Hour) }), "PLAN OK", uitheme.PillPassed, ""},
	}
	for _, tc := range cases {
		label, state, note := atlantisProgress(tc.a)
		if label != tc.label || state != tc.state || note != tc.note {
			t.Errorf("%s: (%q, %v, %q), want (%q, %v, %q)", tc.name, label, state, note, tc.label, tc.state, tc.note)
		}
	}
}

func manyProjectChecks(n int, planAt time.Time) []model.Check {
	checks := []model.Check{{Context: "atlantis/plan", State: "SUCCESS", StartedAt: planAt}}
	for i := 0; i < n; i++ {
		checks = append(checks, model.Check{Context: fmt.Sprintf("atlantis/plan: p%d", i), State: "SUCCESS", StartedAt: planAt})
	}
	return checks
}

func TestAtlantisRowContent(t *testing.T) {
	pr := model.PullRequest{Number: 1, Checks: manyProjectChecks(12, time.Now().Add(-6*time.Minute))}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	m.cfg.Pills = "square"
	if got, want := stripANSI(m.atlantisRow(pr, 120)), "  atlantis  plan ok "; got != want {
		t.Errorf("row = %q, want %q", got, want)
	}
}

func TestAtlantisRowTruncatesToWidth(t *testing.T) {
	pr := model.PullRequest{Number: 1, Checks: manyProjectChecks(12, time.Now().Add(-time.Minute))}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	if got := ansi.StringWidth(m.atlantisRow(pr, 20)); got > 20 {
		t.Errorf("row width %d exceeds 20", got)
	}
}

func TestAtlantisRowPrioritizesOutcome(t *testing.T) {
	m := planModel(t)
	m.prs[0].Comments[0].Body = noChangeReport(t)
	m.prs[0].Comments[0].UpdatedAt = time.Now().Add(-6 * time.Minute)
	m.reconcilePlan()
	pr := m.prs[0]
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	for _, tt := range []struct {
		width      int
		wantPrefix string
	}{
		{160, "  atlantis"},
		{80, "  atlantis"},
		{40, "  "},
		{32, "  "},
	} {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			row := m.atlantisRow(pr, tt.width)
			plain := stripANSI(row)
			if !strings.HasPrefix(plain, tt.wantPrefix) || !strings.Contains(plain, "plan ok") || !strings.Contains(plain, "no changes") {
				t.Fatalf("outcome lost at width %d: %q", tt.width, plain)
			}
			if strings.Contains(plain, "project") || strings.Contains(plain, "ago") || strings.Contains(plain, "unverified") {
				t.Errorf("unexpected metadata: %q", plain)
			}
			if ansi.StringWidth(row) > tt.width {
				t.Errorf("row exceeds width %d: %q", tt.width, plain)
			}
		})
	}
}

func TestAtlantisRowNarrowWidthsKeepVerdict(t *testing.T) {
	for _, noColor := range []bool{false, true} {
		for _, shape := range []string{"rounded", "square"} {
			for _, state := range []string{"SUCCESS", "PENDING", "FAILURE"} {
				pr := model.PullRequest{Checks: atlantisChecks(state, time.Now().Add(-time.Minute))}
				m := modelWithPRs([]model.PullRequest{pr}, 0)
				m.cfg.Pills = shape
				m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, noColor)
				label, _, _ := atlantisProgress(model.DeriveAtlantis(pr.Checks))
				label = strings.ToLower(label)
				if label == "" {
					label = "changes unknown"
				}
				for width := 20; width <= 80; width++ {
					row := m.atlantisRow(pr, width)
					if ansi.StringWidth(row) > width || strings.Contains(row, "\n") || !strings.Contains(stripANSI(row), label) {
						t.Errorf("noColor=%v shape=%s state=%s width=%d: %q", noColor, shape, state, width, row)
					}
				}
			}
		}
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
