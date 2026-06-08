package app

import (
	"context"
	"fmt"
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
	if SortUpdated.Next() != SortNumber || SortNumber.Next() != SortUpdated {
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

// TestRefreshPreservesAITriage verifies that Refresh does not wipe an existing
// AI triage result that was written by a prior Analyze call.
func TestRefreshPreservesAITriage(t *testing.T) {
	store := cache.NewMemoryStore()
	aiResult := &model.AITriage{Summary: "looks fine"}
	if err := store.Put(model.PullRequest{
		Repo:   "owner/repo",
		Number: 1,
		Title:  "pr",
		Triage: model.Triage{Risk: model.RiskSafe, AI: aiResult},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	provider := fakeGitHub{prs: []model.PullRequest{{Repo: "owner/repo", Number: 1, Title: "pr"}}}
	zappr := New(provider, ai.NoopProvider{}, store)

	prs, err := zappr.Refresh(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
	if prs[0].Triage.AI == nil {
		t.Fatal("Refresh wiped AI triage; expected it to be preserved")
	}
	if prs[0].Triage.AI.Summary != "looks fine" {
		t.Fatalf("AI summary = %q, want %q", prs[0].Triage.AI.Summary, "looks fine")
	}
}

// TestAnalyzeStoresAIError verifies that when the AI command returns an error,
// Analyze stores it as an AITriage with an Error field rather than silently
// dropping the result so the UI can display the failure.
func TestAnalyzeStoresAIError(t *testing.T) {
	store := cache.NewMemoryStore()
	provider := fakeGitHub{
		pr:   model.PullRequest{Repo: "owner/repo", Number: 1, Title: "pr"},
		diff: "diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go",
	}
	aiErr := fmt.Errorf("model timeout")
	zappr := New(provider, fakeAI{err: aiErr}, store)

	pr, err := zappr.Analyze(context.Background(), "owner/repo", 1)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if pr.Triage.AI == nil {
		t.Fatal("Analyze did not set AI triage on error; expected AITriage with Error field")
	}
	if pr.Triage.AI.Error == "" {
		t.Fatal("AITriage.Error is empty; expected the error message to be stored")
	}
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
	prs  []model.PullRequest
	pr   model.PullRequest
	diff string
}

func (f fakeGitHub) ListOpenPRs(context.Context, string) ([]model.PullRequest, error) {
	return f.prs, nil
}

func (f fakeGitHub) GetPR(context.Context, string, int) (model.PullRequest, error) {
	return f.pr, nil
}

func (f fakeGitHub) GetDiff(context.Context, string, int) (string, error) {
	return f.diff, nil
}

func (f fakeGitHub) Approve(context.Context, string, int, string) error {
	return nil
}

func (f fakeGitHub) Comment(context.Context, string, int, string) error {
	return nil
}

type fakeAI struct {
	result *model.AITriage
	err    error
}

func (f fakeAI) Triage(_ context.Context, _ model.PullRequest) (*model.AITriage, error) {
	return f.result, f.err
}
