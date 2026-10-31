package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func TestReviewsRowLoadingAndNone(t *testing.T) {
	m := modelWithPRs(nil, 0)
	row := stripANSI(m.reviewsRow(model.PullRequest{}, 80))
	if !strings.Contains(row, "reviews") || !strings.Contains(row, "loading…") {
		t.Errorf("unloaded row = %q", row)
	}
	row = stripANSI(m.reviewsRow(model.PullRequest{ReviewersLoaded: true}, 80))
	if !strings.Contains(row, "none") || strings.Contains(row, "loading") {
		t.Errorf("empty row = %q", row)
	}
}

func TestReviewsRowMergeAndOrder(t *testing.T) {
	pr := model.PullRequest{ReviewersLoaded: true, Reviewers: []model.Reviewer{
		{Login: "alice", State: "APPROVED"},
		{Login: "carol", State: "REQUESTED"},
		{Login: "platform", State: "REQUESTED"},
		{Login: "copilot-pull-request-reviewer", State: "APPROVED", Bot: true},
	}}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	row := m.reviewsRow(pr, 120)
	if got, want := stripANSI(row), "  reviews  approved alice · waiting carol, platform · bots 1"; got != want {
		t.Fatalf("row = %q, want %q", got, want)
	}
	s := m.theme.Styles
	for _, want := range []string{s.SuccessCI.Render("alice"), s.Wait.Render("carol"), s.Muted.Render("bots 1")} {
		if !strings.Contains(row, want) {
			t.Errorf("row %q lacks styled segment %q", row, want)
		}
	}
	if strings.Contains(row, "copilot-pull-request-reviewer") {
		t.Error("bot name should be summarized separately")
	}
}

func TestReviewsRowGroupsStableOrder(t *testing.T) {
	pr := model.PullRequest{ReviewersLoaded: true, Reviewers: []model.Reviewer{
		{Login: "unknown", State: "FUTURE_STATE"},
		{Login: "build-bot", State: "APPROVED", Bot: true},
		{Login: "carol", State: "REQUESTED"},
		{Login: "dana", State: "DISMISSED"},
		{Login: "alice", State: "APPROVED"},
		{Login: "bob", State: "CHANGES_REQUESTED"},
		{Login: "erin", State: "COMMENTED"},
		{Login: "frank", State: "APPROVED"},
		{Login: "review-bot", State: "CHANGES_REQUESTED", Bot: true},
	}}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	want := "  reviews  approved alice, frank · changes bob · waiting carol · commented erin · dismissed dana · reviewed unknown · bots 2"
	if got := stripANSI(m.reviewsRow(pr, 180)); got != want {
		t.Fatalf("grouped row = %q, want %q", got, want)
	}
}

func TestReviewsRowTruncatesToWidth(t *testing.T) {
	pr := model.PullRequest{ReviewersLoaded: true, Reviewers: []model.Reviewer{{Login: "a-long-reviewer-name", State: "APPROVED"}, {Login: "another-one", State: "REQUESTED"}}}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	if got := ansi.StringWidth(m.reviewsRow(pr, 20)); got > 20 {
		t.Errorf("row width %d exceeds 20", got)
	}
}

func TestReviewsRowNoColorPreservesStates(t *testing.T) {
	m := modelWithPRs(nil, 0)
	m.theme = uitheme.New(uitheme.DarkANSI(), colorprofile.ASCII, true)
	for _, tt := range []struct {
		name  string
		state string
		bot   bool
		want  string
	}{
		{name: "approved", state: "APPROVED", want: "approved alice"},
		{name: "changes requested", state: "CHANGES_REQUESTED", want: "changes alice"},
		{name: "requested again", state: "REQUESTED", want: "waiting alice"},
		{name: "commented", state: "COMMENTED", want: "commented alice"},
		{name: "dismissed", state: "DISMISSED", want: "dismissed alice"},
		{name: "unknown", state: "FUTURE_STATE", want: "reviewed alice"},
		{name: "bot approval is advisory", state: "APPROVED", bot: true, want: "bots 1"},
		{name: "bot changes are advisory", state: "CHANGES_REQUESTED", bot: true, want: "bots 1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pr := model.PullRequest{ReviewersLoaded: true, Reviewers: []model.Reviewer{{Login: "alice", State: tt.state, Bot: tt.bot}}}
			if got, want := stripANSI(m.reviewsRow(pr, 80)), "  reviews  "+tt.want; got != want {
				t.Fatalf("no-color reviews row = %q, want %q", got, want)
			}
		})
	}
}
