package app

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/github"
	"github.com/senilio/zappr/internal/model"
)

var ErrPRNotOpen = errors.New("pull request is not open")

type SortMode int

const (
	SortUpdated SortMode = iota
	SortNumber
)

func (m SortMode) String() string {
	switch m {
	case SortNumber:
		return "number"
	default:
		return "updated"
	}
}

func (m SortMode) Next() SortMode {
	switch m {
	case SortUpdated:
		return SortNumber
	default:
		return SortUpdated
	}
}

// ParseSortMode converts a persisted string (as produced by SortMode.String)
// back into a SortMode. Unknown or empty values default to SortUpdated.
func ParseSortMode(s string) SortMode {
	switch s {
	case "number":
		return SortNumber
	default:
		return SortUpdated
	}
}

type App struct {
	GitHub         github.Provider
	Cache          cache.Store
	cacheRefreshMu sync.Mutex // protects observations and their cache reads/writes
	revision       uint64
	inboxRequests  map[string]uint64
	prUpdates      map[PRKey]PRUpdate
}

func New(githubProvider github.Provider, store cache.Store) *App {
	return &App{GitHub: githubProvider, Cache: store}
}

func (a *App) Refresh(ctx context.Context, repo string, includeDrafts bool) ([]model.PullRequest, error) {
	updates, err := a.RefreshInbox(ctx, repo, a.BeginInboxRefresh(repo))
	if err != nil {
		return nil, err
	}
	prs := make([]model.PullRequest, 0, len(updates))
	for _, update := range updates {
		if !update.Closed && (includeDrafts || !update.PR.Draft) {
			prs = append(prs, update.PR)
		}
	}
	SortPullRequests(prs, SortUpdated)
	return prs, nil
}

// RefreshPR returns display data, which may come from a newer observation
// than this request. Use RefreshPRUpdate to distinguish superseded reads.
func (a *App) RefreshPR(ctx context.Context, repo string, number int) (model.PullRequest, error) {
	update, err := a.RefreshPRUpdate(ctx, PRKey{Repo: repo, Number: number})
	if errors.Is(err, ErrRefreshSuperseded) && !update.Closed && update.PR.Number != 0 {
		return update.PR, nil
	}
	if err != nil {
		return model.PullRequest{}, err
	}
	return update.PR, nil
}

func (a *App) Inbox(sortMode SortMode) []model.PullRequest {
	a.cacheRefreshMu.Lock()
	defer a.cacheRefreshMu.Unlock()
	prs, err := a.Cache.List()
	if err != nil {
		return nil
	}
	SortPullRequests(prs, sortMode)
	return prs
}

// InboxRepo returns cached PRs scoped to a single repository. The TUI is always
// bound to one active repo, so it must not surface stale PRs left in the shared
// cache by other repositories (e.g. a repo with zero open PRs would otherwise
// keep showing a previously-viewed PR from another repo).
func (a *App) InboxRepo(repo string, sortMode SortMode) []model.PullRequest {
	a.cacheRefreshMu.Lock()
	defer a.cacheRefreshMu.Unlock()
	prs, err := a.Cache.ListRepo(repo)
	if err != nil {
		return nil
	}
	for _, pr := range prs {
		a.rememberCachedPRLocked(pr)
	}
	SortPullRequests(prs, sortMode)
	return prs
}

func SortPullRequests(prs []model.PullRequest, sortMode SortMode) {
	sort.Slice(prs, func(i, j int) bool {
		if prs[i].Skipped != prs[j].Skipped {
			return !prs[i].Skipped
		}
		switch sortMode {
		case SortNumber:
			return prs[i].Number > prs[j].Number
		default:
			left := prs[i].UpdatedAt
			right := prs[j].UpdatedAt
			if left.IsZero() {
				left = fallbackPRTime(prs[i])
			}
			if right.IsZero() {
				right = fallbackPRTime(prs[j])
			}
			if !left.Equal(right) {
				return left.After(right)
			}
		}
		return prs[i].Number > prs[j].Number
	})
}

func fallbackPRTime(pr model.PullRequest) time.Time {
	return time.Unix(int64(pr.Number), 0)
}

func (a *App) Approve(ctx context.Context, repo string, number int, body string) error {
	return a.GitHub.Approve(ctx, repo, number, body)
}

func (a *App) Comment(ctx context.Context, repo string, number int, body string) error {
	return a.GitHub.Comment(ctx, repo, number, body)
}

func (a *App) ToggleDraft(ctx context.Context, repo string, number int, isDraft bool) error {
	return a.GitHub.ToggleDraft(ctx, repo, number, isDraft)
}

func (a *App) ToggleAutoMerge(ctx context.Context, repo string, number int, enabled bool) error {
	return a.GitHub.ToggleAutoMerge(ctx, repo, number, enabled)
}

func (a *App) ClosePR(ctx context.Context, repo string, number int, comment string) error {
	_, err := a.ClosePRUpdate(ctx, PRKey{Repo: repo, Number: number}, comment)
	return err
}

func (a *App) UpdateBranch(ctx context.Context, repo string, number int) error {
	return a.GitHub.UpdateBranch(ctx, repo, number)
}

func (a *App) RequestChanges(ctx context.Context, repo string, number int, body string) error {
	return a.GitHub.RequestChanges(ctx, repo, number, body)
}

func (a *App) MergePR(ctx context.Context, repo string, number int) error {
	return a.GitHub.MergePR(ctx, repo, number)
}

func (a *App) SetSkip(repo string, number int, skipped bool) error {
	a.cacheRefreshMu.Lock()
	defer a.cacheRefreshMu.Unlock()
	if err := a.Cache.SetSkip(repo, number, skipped); err != nil {
		return err
	}
	key := PRKey{Repo: repo, Number: number}
	if update := a.prUpdates[key]; update.PR.Number != 0 {
		update.PR.Skipped = skipped
		a.prUpdates[key] = update
	}
	return nil
}
