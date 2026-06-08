package tui

import (
	"testing"

	"github.com/senilio/zappr/internal/model"
)

func TestVisiblePRsExcludesDrafts(t *testing.T) {
	prs := []model.PullRequest{
		{Number: 1, Draft: true},
		{Number: 2},
		{Number: 3, Draft: true},
	}

	got := visiblePRs(prs)

	if len(got) != 1 || got[0].Number != 2 {
		t.Fatalf("visiblePRs = %#v, want only PR #2", got)
	}
}
