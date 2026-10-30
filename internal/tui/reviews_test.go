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
	if got, want := stripANSI(row), "  reviews  alice  carol  platform  copilot-pull-request-reviewer"; got != want {
		t.Fatalf("row = %q, want %q", got, want)
	}
	s := m.theme.Styles
	for _, want := range []string{s.SuccessCI.Render("alice"), s.Wait.Render("carol"), s.Muted.Render("copilot-pull-request-reviewer")} {
		if !strings.Contains(row, want) {
			t.Errorf("row %q lacks styled segment %q", row, want)
		}
	}
	if strings.Contains(row, s.SuccessCI.Render("copilot-pull-request-reviewer")) {
		t.Error("bot rendered as approval")
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
		{name: "approved", state: "APPROVED", want: "alice✓"},
		{name: "changes requested", state: "CHANGES_REQUESTED", want: "alice✗"},
		{name: "requested again", state: "REQUESTED", want: "alice●"},
		{name: "commented", state: "COMMENTED", want: "alice (commented)"},
		{name: "dismissed", state: "DISMISSED", want: "alice (dismissed)"},
		{name: "bot approval is advisory", state: "APPROVED", bot: true, want: "alice (bot)"},
		{name: "bot changes are advisory", state: "CHANGES_REQUESTED", bot: true, want: "alice (bot)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pr := model.PullRequest{ReviewersLoaded: true, Reviewers: []model.Reviewer{{Login: "alice", State: tt.state, Bot: tt.bot}}}
			if got, want := stripANSI(m.reviewsRow(pr, 80)), "  reviews  "+tt.want; got != want {
				t.Fatalf("no-color reviews row = %q, want %q", got, want)
			}
		})
	}
}
