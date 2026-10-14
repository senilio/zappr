package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/senilio/zappr/internal/model"
)

func TestViewsIgnoreLegacyAutomatedReview(t *testing.T) {
	var pr model.PullRequest
	data := `{"Repo":"owner/repo","Number":123,"Title":"Manual review","Triage":{"risk":"safe","AI":{"summary":"stale recommendation","risk_notes":["stale risk"],"suggested_focus":["stale focus"]}}}`
	if err := json.Unmarshal([]byte(data), &pr); err != nil {
		t.Fatalf("decode legacy PR: %v", err)
	}
	m := minimalModel()
	m.prs = []model.PullRequest{pr}
	views := map[string]string{
		"details":        m.detailView(),
		"approval":       m.approveView(),
		"approval modal": m.approvePanel(),
	}
	for _, mode := range []SummaryMode{SummaryModeFiles, SummaryModeCommits, SummaryModeComments} {
		m.summaryMode = mode
		views[mode.String()] = m.summaryContent(pr, 80)
	}
	for name, view := range views {
		t.Run(name, func(t *testing.T) {
			for _, removed := range []string{"AI review", "stale recommendation", "stale risk", "stale focus"} {
				if strings.Contains(stripANSI(view), removed) {
					t.Fatalf("view still contains %q: %q", removed, view)
				}
			}
		})
	}
}
