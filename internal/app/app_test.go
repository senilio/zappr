package app

import (
	"context"
	"errors"
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

func TestParseSortModeRoundTrips(t *testing.T) {
	if ParseSortMode(SortUpdated.String()) != SortUpdated {
		t.Fatalf("ParseSortMode(%q) != SortUpdated", SortUpdated.String())
	}
	if ParseSortMode(SortNumber.String()) != SortNumber {
		t.Fatalf("ParseSortMode(%q) != SortNumber", SortNumber.String())
	}
	if ParseSortMode("") != SortUpdated {
		t.Fatalf("ParseSortMode(\"\") should default to SortUpdated")
	}
	if ParseSortMode("garbage") != SortUpdated {
		t.Fatalf("ParseSortMode(garbage) should default to SortUpdated")
	}
}

func TestRefreshPrunesClosedPRs(t *testing.T) {
	store := cache.NewMemoryStore()
	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 1, Title: "closed", Triage: model.Triage{Risk: model.RiskSafe}}); err != nil {
		t.Fatalf("Put closed: %v", err)
	}
	provider := fakeGitHub{prs: []model.PullRequest{{Repo: "owner/repo", Number: 2, Title: "open", Triage: model.Triage{Risk: model.RiskSafe}}}}
	zappr := New(provider, ai.NoopProvider{}, store)

	prs, err := zappr.Refresh(context.Background(), "owner/repo", false)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	assertOrder(t, prs, []int{2})
}

func TestRefreshFiltersDraftsLocally(t *testing.T) {
	store := cache.NewMemoryStore()
	provider := fakeGitHub{prs: []model.PullRequest{
		{Repo: "owner/repo", Number: 1, Draft: true},
		{Repo: "owner/repo", Number: 2},
	}}
	zappr := New(provider, ai.NoopProvider{}, store)

	prs, err := zappr.Refresh(context.Background(), "owner/repo", false)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	assertOrder(t, prs, []int{2})
}

func TestRefreshPreservesHiddenDraftCacheState(t *testing.T) {
	store := cache.NewMemoryStore()
	if err := store.Put(model.PullRequest{
		Repo:    "owner/repo",
		Number:  1,
		Draft:   true,
		Skipped: true,
		Triage:  model.Triage{AI: &model.AITriage{Summary: "cached"}},
	}); err != nil {
		t.Fatalf("Put draft: %v", err)
	}
	provider := fakeGitHub{prs: []model.PullRequest{{Repo: "owner/repo", Number: 1, Title: "fresh", Draft: true}}}
	zappr := New(provider, ai.NoopProvider{}, store)

	prs, err := zappr.Refresh(context.Background(), "owner/repo", false)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(prs) != 0 {
		t.Fatalf("visible PRs = %#v, want no drafts", prs)
	}
	cached, err := store.Get("owner/repo", 1)
	if err != nil {
		t.Fatalf("Get draft: %v", err)
	}
	if cached.Title != "fresh" || !cached.Skipped || cached.Triage.AI == nil || cached.Triage.AI.Summary != "cached" {
		t.Fatalf("cached draft state not preserved: %#v", cached)
	}
}

func TestRefreshDoesNotPruneIncompleteList(t *testing.T) {
	store := cache.NewMemoryStore()
	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 1}); err != nil {
		t.Fatalf("Put cached PR: %v", err)
	}
	provider := fakeGitHub{
		prs:        []model.PullRequest{{Repo: "owner/repo", Number: 2}},
		incomplete: true,
	}
	zappr := New(provider, ai.NoopProvider{}, store)

	prs, err := zappr.Refresh(context.Background(), "owner/repo", false)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	assertOrder(t, prs, []int{2, 1})
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

	prs, err := zappr.Refresh(context.Background(), "owner/repo", false)
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

func TestRefreshPRPreservesAITriage(t *testing.T) {
	store := cache.NewMemoryStore()
	aiResult := &model.AITriage{Summary: "looks fine"}
	if err := store.Put(model.PullRequest{
		Repo:   "owner/repo",
		Number: 1,
		Title:  "cached",
		Triage: model.Triage{Risk: model.RiskSafe, AI: aiResult},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	provider := fakeGitHub{pr: model.PullRequest{Repo: "owner/repo", Number: 1, Title: "fresh"}}
	zappr := New(provider, ai.NoopProvider{}, store)

	pr, err := zappr.RefreshPR(context.Background(), "owner/repo", 1)
	if err != nil {
		t.Fatalf("RefreshPR: %v", err)
	}

	if pr.Title != "fresh" {
		t.Fatalf("Title = %q, want fresh", pr.Title)
	}
	if pr.Triage.AI == nil || pr.Triage.AI.Summary != "looks fine" {
		t.Fatalf("AI triage = %#v, want preserved summary", pr.Triage.AI)
	}
	cached, err := store.Get("owner/repo", 1)
	if err != nil {
		t.Fatalf("Get cached: %v", err)
	}
	if cached.Title != "fresh" {
		t.Fatalf("cached title = %q, want fresh", cached.Title)
	}
}

func TestRefreshPRPrunesClosedPR(t *testing.T) {
	store := cache.NewMemoryStore()
	for _, pr := range []model.PullRequest{
		{Repo: "owner/repo", Number: 1, Title: "closed"},
		{Repo: "owner/repo", Number: 2, Title: "open"},
	} {
		if err := store.Put(pr); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	provider := fakeGitHub{pr: model.PullRequest{Repo: "owner/repo", Number: 1, Title: "closed", State: "CLOSED"}}
	zappr := New(provider, ai.NoopProvider{}, store)

	_, err := zappr.RefreshPR(context.Background(), "owner/repo", 1)
	if !errors.Is(err, ErrPRNotOpen) {
		t.Fatalf("RefreshPR err = %v, want ErrPRNotOpen", err)
	}
	if _, err := store.Get("owner/repo", 1); err == nil {
		t.Fatal("closed PR remains cached")
	}
	if _, err := store.Get("owner/repo", 2); err != nil {
		t.Fatalf("open cached PR missing: %v", err)
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
	prs        []model.PullRequest
	pr         model.PullRequest
	diff       string
	incomplete bool
}

func (f fakeGitHub) ListPRs(context.Context, string) ([]model.PullRequest, bool, error) {
	return f.prs, !f.incomplete, nil
}

func (f fakeGitHub) GetPR(context.Context, string, int) (model.PullRequest, error) {
	return f.pr, nil
}

func (f fakeGitHub) GetDiff(context.Context, string, int) (string, error) {
	return f.diff, nil
}

func (f fakeGitHub) GetCommitDiff(context.Context, string, string) (string, error) {
	return "", nil
}

func (f fakeGitHub) GetFileContent(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (f fakeGitHub) Approve(context.Context, string, int, string) error {
	return nil
}

func (f fakeGitHub) Comment(context.Context, string, int, string) error {
	return nil
}

func (f fakeGitHub) ToggleDraft(context.Context, string, int, bool) error {
	return nil
}

func (f fakeGitHub) ClosePR(context.Context, string, int, string) error {
	return nil
}

func (f fakeGitHub) UpdateBranch(context.Context, string, int) error {
	return nil
}

func (f fakeGitHub) RequestChanges(context.Context, string, int, string) error {
	return nil
}

func (f fakeGitHub) MergePR(context.Context, string, int) error {
	return nil
}

type fakeAI struct {
	result *model.AITriage
	err    error
}

func (f fakeAI) Triage(_ context.Context, _ model.PullRequest) (*model.AITriage, error) {
	return f.result, f.err
}
