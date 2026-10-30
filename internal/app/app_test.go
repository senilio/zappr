package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

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
	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 1, Title: "closed"}); err != nil {
		t.Fatalf("Put closed: %v", err)
	}
	provider := fakeGitHub{prs: []model.PullRequest{{Repo: "owner/repo", Number: 2, Title: "open"}}}
	zappr := New(provider, store)

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
	zappr := New(provider, store)

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
	}); err != nil {
		t.Fatalf("Put draft: %v", err)
	}
	provider := fakeGitHub{prs: []model.PullRequest{{Repo: "owner/repo", Number: 1, Title: "fresh", Draft: true}}}
	zappr := New(provider, store)

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
	if cached.Title != "fresh" || !cached.Skipped {
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
	zappr := New(provider, store)

	prs, err := zappr.Refresh(context.Background(), "owner/repo", false)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	assertOrder(t, prs, []int{2, 1})
}

func TestRefreshKeepsCachedDetailsWhenPRUnchanged(t *testing.T) {
	store := cache.NewMemoryStore()
	updated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := store.Put(model.PullRequest{
		Repo:                 "owner/repo",
		Number:               1,
		UpdatedAt:            updated,
		Commits:              []model.PRCommit{{OID: "abc"}},
		Comments:             []model.PRComment{{Body: "hi"}},
		DetailsLoaded:        true,
		ReviewCommentsLoaded: true,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	provider := fakeGitHub{prs: []model.PullRequest{{Repo: "owner/repo", Number: 1, Title: "fresh", UpdatedAt: updated}}}
	zappr := New(provider, store)

	prs, err := zappr.Refresh(context.Background(), "owner/repo", false)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(prs) != 1 || prs[0].Title != "fresh" || !prs[0].DetailsLoaded || !prs[0].ReviewCommentsLoaded || len(prs[0].Commits) != 1 || len(prs[0].Comments) != 1 {
		t.Fatalf("cached details not preserved: %#v", prs)
	}
}

func TestRefreshDropsCachedDetailsWhenPRUpdated(t *testing.T) {
	store := cache.NewMemoryStore()
	updated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := store.Put(model.PullRequest{
		Repo:                 "owner/repo",
		Number:               1,
		UpdatedAt:            updated,
		Commits:              []model.PRCommit{{OID: "abc"}},
		DetailsLoaded:        true,
		ReviewCommentsLoaded: true,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	provider := fakeGitHub{prs: []model.PullRequest{{Repo: "owner/repo", Number: 1, UpdatedAt: updated.Add(time.Minute)}}}
	zappr := New(provider, store)

	prs, err := zappr.Refresh(context.Background(), "owner/repo", false)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(prs) != 1 || prs[0].DetailsLoaded || len(prs[0].Commits) != 0 {
		t.Fatalf("stale details kept: %#v", prs)
	}
}

func TestRefreshDropsLegacyDetailsWithoutReviewComments(t *testing.T) {
	store := cache.NewMemoryStore()
	updated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := store.Put(model.PullRequest{
		Repo: "owner/repo", Number: 1, UpdatedAt: updated, DetailsLoaded: true,
		Comments: []model.PRComment{{Body: "conversation only"}},
	}); err != nil {
		t.Fatal(err)
	}
	provider := fakeGitHub{prs: []model.PullRequest{{Repo: "owner/repo", Number: 1, UpdatedAt: updated}}}
	prs, err := New(provider, store).Refresh(context.Background(), "owner/repo", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].DetailsLoaded {
		t.Fatalf("legacy cached details must be reloaded: %#v", prs)
	}
}

func TestRefreshPRUpdatesCachedDetails(t *testing.T) {
	store := cache.NewMemoryStore()
	if err := store.Put(model.PullRequest{
		Repo: "owner/repo", Number: 1, Title: "cached",
		Comments: []model.PRComment{{Body: "old comment"}},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	provider := fakeGitHub{pr: model.PullRequest{
		Repo: "owner/repo", Number: 1, Title: "fresh", DetailsLoaded: true,
		Comments: []model.PRComment{{Body: "new comment"}},
	}}
	zappr := New(provider, store)

	pr, err := zappr.RefreshPR(context.Background(), "owner/repo", 1)
	if err != nil {
		t.Fatalf("RefreshPR: %v", err)
	}

	if pr.Title != "fresh" {
		t.Fatalf("Title = %q, want fresh", pr.Title)
	}
	if !pr.DetailsLoaded || len(pr.Comments) != 1 || pr.Comments[0].Body != "new comment" {
		t.Fatalf("PR details not refreshed: %#v", pr)
	}
	cached, err := store.Get("owner/repo", 1)
	if err != nil {
		t.Fatalf("Get cached: %v", err)
	}
	if cached.Title != "fresh" || !cached.DetailsLoaded || len(cached.Comments) != 1 || cached.Comments[0].Body != "new comment" {
		t.Fatalf("cached details not refreshed: %#v", cached)
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
	zappr := New(provider, store)

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
	incomplete bool
}

func (f fakeGitHub) ListPRs(context.Context, string) ([]model.PullRequest, bool, error) {
	return f.prs, !f.incomplete, nil
}

func (f fakeGitHub) GetPR(context.Context, string, int) (model.PullRequest, error) {
	return f.pr, nil
}

func (f fakeGitHub) GetDiff(context.Context, string, int) (string, error) {
	return "", nil
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

func (f fakeGitHub) ToggleAutoMerge(context.Context, string, int, bool) error { return nil }

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

func refreshOne(t *testing.T, cached, listed model.PullRequest) model.PullRequest {
	t.Helper()
	store := cache.NewMemoryStore()
	if err := store.Put(cached); err != nil {
		t.Fatalf("Put: %v", err)
	}
	prs, err := New(fakeGitHub{prs: []model.PullRequest{listed}}, store).Refresh(context.Background(), "owner/repo", false)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("prs = %#v", prs)
	}
	return prs[0]
}

func TestRefreshKeepsCachedReviewersWhenPRUnchanged(t *testing.T) {
	updated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	reviewers := []model.Reviewer{{Login: "alice", State: "APPROVED"}}
	got := refreshOne(t,
		model.PullRequest{Repo: "owner/repo", Number: 1, UpdatedAt: updated, Reviewers: reviewers, ReviewersLoaded: true},
		model.PullRequest{Repo: "owner/repo", Number: 1, UpdatedAt: updated})
	if !got.ReviewersLoaded || !reflect.DeepEqual(got.Reviewers, reviewers) {
		t.Fatalf("reviewers not preserved: %#v", got)
	}
}

func TestRefreshDropsCachedReviewersWhenPRUpdated(t *testing.T) {
	updated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := refreshOne(t,
		model.PullRequest{Repo: "owner/repo", Number: 1, UpdatedAt: updated, Reviewers: []model.Reviewer{{Login: "alice", State: "APPROVED"}}, ReviewersLoaded: true},
		model.PullRequest{Repo: "owner/repo", Number: 1, UpdatedAt: updated.Add(time.Minute)})
	if got.ReviewersLoaded || got.Reviewers != nil {
		t.Fatalf("stale reviewers kept: %#v", got)
	}
}

func TestRefreshKeepsCachedMergeStateWhenListUnknown(t *testing.T) {
	updated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range []struct{ cached, listed, want string }{
		{"BEHIND", "UNKNOWN", "BEHIND"},
		{"BEHIND", "CLEAN", "CLEAN"},
		{"BEHIND", "", "BEHIND"},
	} {
		got := refreshOne(t,
			model.PullRequest{Repo: "owner/repo", Number: 1, UpdatedAt: updated, MergeState: tc.cached},
			model.PullRequest{Repo: "owner/repo", Number: 1, UpdatedAt: updated.Add(time.Minute), MergeState: tc.listed})
		if got.MergeState != tc.want {
			t.Errorf("cached %q listed %q: MergeState = %q, want %q", tc.cached, tc.listed, got.MergeState, tc.want)
		}
	}
}
