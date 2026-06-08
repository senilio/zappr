package app

import (
	"context"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/ai"
	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/model"
)

func TestSortPullRequestsDefaultsToUpdatedDesc(t *testing.T) {
	oldest := model.PullRequest{Repo: "owner/repo", Number: 1, UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	newest := model.PullRequest{Repo: "owner/repo", Number: 2, UpdatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)}
	middle := model.PullRequest{Repo: "owner/repo", Number: 3, UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)}
	prs := []model.PullRequest{oldest, newest, middle}

	SortPullRequests(prs, SortUpdated)

	assertOrder(t, prs, []int{2, 3, 1})
}

func TestSortPullRequestsCyclesModes(t *testing.T) {
	if SortUpdated.Next() != SortNumber || SortNumber.Next() != SortRisk || SortRisk.Next() != SortUpdated {
		t.Fatalf("unexpected sort cycle")
	}
}

func TestRefreshPrunesClosedPRs(t *testing.T) {
	store := cache.NewMemoryStore()
	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 1, Title: "closed", Triage: model.Triage{Risk: model.RiskSafe}}); err != nil {
		t.Fatalf("Put closed: %v", err)
	}
	provider := fakeGitHub{prs: []model.PullRequest{{Repo: "owner/repo", Number: 2, Title: "open", Triage: model.Triage{Risk: model.RiskSafe}}}}
	zappr := New(provider, ai.NoopProvider{}, store)

	prs, err := zappr.Refresh(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	assertOrder(t, prs, []int{2})
}

func assertOrder(t *testing.T, prs []model.PullRequest, want []int) {
	t.Helper()
	if len(prs) != len(want) {
		t.Fatalf("len = %d, want %d", len(prs), len(want))
	}
	for i, pr := range prs {
		if pr.Number != want[i] {
			t.Fatalf("order[%d] = %d, want %d", i, pr.Number, want[i])
		}
	}
}

type fakeGitHub struct {
	prs []model.PullRequest
}

func (f fakeGitHub) ListOpenPRs(context.Context, string) ([]model.PullRequest, error) {
	return f.prs, nil
}

func (f fakeGitHub) GetPR(context.Context, string, int) (model.PullRequest, error) {
	return model.PullRequest{}, nil
}

func (f fakeGitHub) GetDiff(context.Context, string, int) (string, error) {
	return "", nil
}

func (f fakeGitHub) Approve(context.Context, string, int, string) error {
	return nil
}

func (f fakeGitHub) Comment(context.Context, string, int, string) error {
	return nil
}
