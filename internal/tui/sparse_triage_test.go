package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/model"
)

func TestCommentPaneTitleStaysSparse(t *testing.T) {
	m := minimalModel()
	m.summaryMode = SummaryModeComments
	m.prs[0].Comments = []model.PRComment{{ID: "long-review-id", Author: "reviewer", Kind: "review", Path: "internal/main.go", Line: 42, CreatedAt: time.Now(), Body: "Review text"}}
	if got := stripANSI(m.diffTitle()); got != "Comment" {
		t.Fatalf("comment pane title = %q, want Comment", got)
	}
}

func TestSummaryTitleNamesOnlyActiveTab(t *testing.T) {
	for _, mode := range []SummaryMode{SummaryModeFiles, SummaryModeCommits, SummaryModeComments, SummaryModePlan} {
		m := planModel(t)
		m.width = 240
		m.summaryMode = mode
		title := stripANSI(m.summaryTitle())
		for i, tab := range []SummaryMode{SummaryModeFiles, SummaryModeCommits, SummaryModeComments, SummaryModePlan} {
			label := "[" + string(rune('1'+i))
			if mode == tab {
				label += " " + tab.String()
			}
			if !strings.Contains(title, label+"]") {
				t.Errorf("mode %s title %q lacks %q", mode, title, label+"]")
			}
		}
	}
}

func TestInboxAtlantisActivityIsPerPR(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		m := planModel(t)
		m.cfg.Wrap.Inbox = wrap
		m.prs[0].Checks = []model.Check{{Context: "atlantis/plan", State: "SUCCESS"}}
		m.prs = append(m.prs, model.PullRequest{Repo: m.prs[0].Repo, Number: 2, Title: "Ordinary PR", ReviewersLoaded: true, ConversationCommentsLoaded: true})
		m.selected = 1
		content, spans := m.inboxContent(140)
		if strings.Contains(stripANSI(strings.Split(content, "\n")[spans[1].end-1]), "plan ok") {
			t.Fatalf("wrap=%v: unrelated Atlantis row: %s", wrap, content)
		}
		if rows := spans[1].end - spans[1].start; rows != 2 {
			t.Fatalf("selected item has %d rows, want title and metadata", rows)
		}
		m.prs[1].Checks = []model.Check{{Context: "atlantis/plan", State: "FAILURE"}}
		content, spans = m.inboxContent(140)
		if !strings.Contains(stripANSI(content), "plan failed") || spans[1].end-spans[1].start != 2 {
			t.Fatal("selected PR's Atlantis failure missing", content)
		}
	}
}
