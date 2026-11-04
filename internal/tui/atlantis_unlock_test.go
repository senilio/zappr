package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/model"
)

func TestAtlantisUnlockInvalidatesInboxStatus(t *testing.T) {
	at := time.Date(2026, 11, 4, 11, 0, 0, 0, time.UTC)
	confirmation := model.PRComment{
		Author: "trustly-atlantis-bot[bot]", CreatedAt: at.Add(time.Minute),
		Body: "All Atlantis locks for this PR have been unlocked and plans discarded",
	}
	for _, tc := range []struct {
		name    string
		comment model.PRComment
		checks  []model.Check
		want    string
	}{
		{name: "unlock", comment: confirmation, want: "plan discarded"},
		{name: "unlock without checks", comment: confirmation, checks: []model.Check{}, want: "plan discarded"},
		{name: "stale apply and policy", comment: confirmation, checks: []model.Check{
			{Context: "atlantis/plan", State: "SUCCESS", StartedAt: at},
			{Context: "atlantis/apply", State: "SUCCESS", StartedAt: at},
			{Context: "atlantis/apply: a", State: "SUCCESS", StartedAt: at},
			{Context: "atlantis/policy_check", State: "FAILURE", StartedAt: at},
		}, want: "plan discarded"},
		{name: "command is not confirmation", comment: model.PRComment{Body: "atlantis unlock", CreatedAt: confirmation.CreatedAt}, want: "plan ok"},
		{name: "review is not confirmation", comment: model.PRComment{Body: confirmation.Body, Kind: "review", CreatedAt: confirmation.CreatedAt}, want: "plan ok"},
		{name: "older unlock", comment: model.PRComment{Body: confirmation.Body, CreatedAt: at.Add(-time.Minute)}, want: "plan ok"},
		{name: "new plan drops stale project failures", comment: confirmation, checks: []model.Check{
			{Context: "atlantis/plan", State: "SUCCESS", StartedAt: at.Add(2 * time.Minute)},
			{Context: "atlantis/plan: old", State: "FAILURE", StartedAt: at},
		}, want: "plan ok"},
		{name: "new plan running", comment: confirmation, checks: []model.Check{
			{Context: "atlantis/plan", State: "PENDING", StartedAt: at.Add(2 * time.Minute)},
		}, want: "planning"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := planModel(t) // Retain an old parsed report to exercise the fallback too.
			m.prs[0].Checks = tc.checks
			if tc.checks == nil {
				m.prs[0].Checks = atlantisChecks("SUCCESS", at)
			}
			m.prs[0].Comments = append(m.prs[0].Comments, tc.comment)
			m.reconcilePlan()
			if got := stripANSI(m.atlantisRow(m.prs[0], 100)); !strings.Contains(got, tc.want) {
				t.Fatalf("row = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInboxLayoutInvalidatesAfterAtlantisUnlock(t *testing.T) {
	m := scrollModel(t, 1)
	at := time.Date(2026, 11, 4, 11, 0, 0, 0, time.UTC)
	m.prs[0].Checks = atlantisChecks("SUCCESS", at)
	before := m.inboxPane(100)
	if !strings.Contains(stripANSI(before.content), "plan ok") {
		t.Fatal("missing initial plan status")
	}
	m.prs[0].Comments = []model.PRComment{{
		Author: "trustly-atlantis-bot[bot]", CreatedAt: at.Add(time.Minute),
		Body: "All Atlantis locks for this PR have been unlocked and plans discarded",
	}}
	after := m.inboxPane(100)
	if after == before || !strings.Contains(stripANSI(after.content), "plan discarded") {
		t.Fatalf("stale Inbox after unlock: %q", stripANSI(after.content))
	}
}

func TestAtlantisApplyGuardAfterUnlock(t *testing.T) {
	at := time.Date(2026, 11, 4, 11, 0, 0, 0, time.UTC)
	pr := model.PullRequest{Checks: atlantisChecks("SUCCESS", at), Comments: []model.PRComment{{
		Body: "All Atlantis locks for this PR have been unlocked and plans discarded", CreatedAt: at.Add(time.Minute),
	}}}
	if reasons := guardReasons(atlantisCommandApply, pr, time.Time{}, at.Add(2*time.Minute)); len(reasons) == 0 {
		t.Fatal("apply guard accepted a discarded plan")
	}
}
