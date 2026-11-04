package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/model"
)

func noChangeReport(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("../plan/testdata/aggregate-no-changes.txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestInboxRemovesTFAndBotCount(t *testing.T) {
	m := planModel(t)
	m.prs[0].Checks = []model.Check{{Context: "atlantis/plan", State: "SUCCESS"}}
	m.prs[0].Reviewers = []model.Reviewer{{Login: "sam", State: "APPROVED"}, {Login: "review-bot", State: "APPROVED", Bot: true}}
	plain := stripANSI(strings.Join(inboxLines(m, 140), "\n"))
	for _, forbidden := range []string{" TF ", "PLAN OK", "bots 1", "sam", "review-bot", "reviews"} {
		if strings.Contains(plain, forbidden) {
			t.Fatal(forbidden, plain)
		}
	}
}

func TestAtlantisRowSeparatesProgressFromContents(t *testing.T) {
	m := planModel(t)
	m.prs[0].Checks = []model.Check{{Context: "atlantis/plan", State: "SUCCESS"}, {Context: "atlantis/apply", State: "SUCCESS"}}
	m.prs[0].Comments, m.prs[0].ConversationCommentsLoaded = nil, false
	m.reconcilePlan()
	row := stripANSI(m.atlantisRow(m.prs[0], 160))
	if strings.Contains(row, "changes") || !strings.Contains(row, "plan ok") {
		t.Fatal("status-only inference", row)
	}
	m.prs[0].ConversationCommentsLoaded = true
	m.reconcilePlan()
	if row := stripANSI(m.atlantisRow(m.prs[0], 160)); !strings.Contains(row, "plan ok") || strings.Contains(row, "changes") {
		t.Fatal(row)
	}
	for _, phase := range []struct{ context, state, want string }{
		{"atlantis/plan", "PENDING", "planning"}, {"atlantis/apply", "PENDING", "applying"}, {"atlantis/apply", "FAILURE", "apply failed"},
	} {
		m.prs[0].Checks = []model.Check{{Context: phase.context, State: phase.state}}
		if row := stripANSI(m.atlantisRow(m.prs[0], 160)); !strings.Contains(row, phase.want) || strings.Contains(row, "changes") {
			t.Fatal(phase.want, row)
		}
	}
	m = planModel(t)
	m.prs[0].Comments[0].Body = noChangeReport(t)
	m.reconcilePlan()
	row = stripANSI(m.atlantisRow(m.prs[0], 160))
	if strings.Contains(row, "no changes") || !strings.Contains(row, "plan ok") {
		t.Fatal("want only the status pill", row)
	}
	for _, width := range []int{40, 48, 60} {
		row = stripANSI(m.atlantisRow(m.prs[0], width))
		if strings.Contains(row, "no changes") || !strings.Contains(row, "plan ok") {
			t.Fatal(width, row)
		}
	}
	// Missing continuation is not promoted to a complete no-change result.
	m.prs[0].ConversationCommentsLoaded = false
	m.reconcilePlan()
	if row = stripANSI(m.atlantisRow(m.prs[0], 160)); strings.Contains(row, "incomplete") || strings.Contains(row, "plan ok") || !strings.Contains(row, "plan unknown") {
		t.Fatal(row)
	}
}

func TestAtlantisRowDoesNotBorrowAnotherPRReport(t *testing.T) {
	m := planModel(t)
	m.prs[0].Comments[0].Body = noChangeReport(t)
	m.reconcilePlan()
	other := model.PullRequest{Repo: m.prs[0].Repo, Number: 2}
	m.prs = append(m.prs, other)
	m.selected = 1 // Even before selected-source reconciliation, don't borrow it.
	if row := stripANSI(m.atlantisRow(other, 140)); strings.Contains(row, "changes") || !strings.Contains(row, "plan unknown") {
		t.Fatal(row)
	}
}

func TestInboxStatusStaysStableOnReportOutcomeEdit(t *testing.T) {
	m := planModel(t)
	m.prs[0].Checks = []model.Check{{Context: "atlantis/plan", State: "SUCCESS"}}
	before, _ := m.inboxContent(140)
	m.prs[0].Comments[0].Body = noChangeReport(t)
	m.prs[0].Comments[0].UpdatedAt = time.Unix(200, 0)
	m.reconcilePlan()
	after, _ := m.inboxContent(140)
	if before != after || strings.Contains(stripANSI(after), "no changes") || !strings.Contains(stripANSI(after), "plan ok") {
		t.Fatal("report outcome changed the status-only Inbox", after)
	}
}
