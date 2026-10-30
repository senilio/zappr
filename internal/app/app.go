package app

import (
	"context"
	"errors"
	"sort"
	"strings"
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
	GitHub github.Provider
	Cache  cache.Store
}

func New(githubProvider github.Provider, store cache.Store) *App {
	return &App{GitHub: githubProvider, Cache: store}
}

func (a *App) Refresh(ctx context.Context, repo string, includeDrafts bool) ([]model.PullRequest, error) {
	prs, complete, err := a.GitHub.ListPRs(ctx, repo)
	if err != nil {
		return nil, err
	}
	for i := range prs {
		if cached, err := a.Cache.Get(prs[i].Repo, prs[i].Number); err == nil {
			// List refreshes omit commits and comments; keep the cached ones
			// until the PR changes, after which GetPR must refetch them.
			if cached.DetailsLoaded && cached.ReviewCommentsLoaded && cached.UpdatedAt.Equal(prs[i].UpdatedAt) {
				prs[i].Commits = cached.Commits
				prs[i].Comments = cached.Comments
				prs[i].DetailsLoaded = true
				prs[i].ReviewCommentsLoaded = true
			}
			if cached.ReviewersLoaded && cached.UpdatedAt.Equal(prs[i].UpdatedAt) {
				prs[i].Reviewers = cached.Reviewers
				prs[i].ReviewersLoaded = true
			}
			// The list query often reports UNKNOWN while GitHub recomputes
			// mergeability; keep the last known state instead.
			if (prs[i].MergeState == "" || prs[i].MergeState == "UNKNOWN") && cached.MergeState != "" {
				prs[i].MergeState = cached.MergeState
			}
		}
		if err := a.Cache.Put(prs[i]); err != nil {
			return nil, err
		}
	}
	openNumbers := make(map[int]bool, len(prs))
	for _, pr := range prs {
		openNumbers[pr.Number] = true
	}
	if complete {
		if err := a.Cache.PruneRepo(repo, openNumbers); err != nil {
			return nil, err
		}
	}
	inbox := a.InboxRepo(repo, SortUpdated)
	if includeDrafts {
		return inbox, nil
	}
	visible := inbox[:0]
	for _, pr := range inbox {
		if !pr.Draft {
			visible = append(visible, pr)
		}
	}
	return visible, nil
}

func (a *App) RefreshPR(ctx context.Context, repo string, number int) (model.PullRequest, error) {
	pr, err := a.GitHub.GetPR(ctx, repo, number)
	if err != nil {
		return model.PullRequest{}, err
	}
	if pr.State != "" && !strings.EqualFold(pr.State, "OPEN") {
		if err := a.Cache.Delete(repo, number); err != nil {
			return model.PullRequest{}, err
		}
		return model.PullRequest{}, ErrPRNotOpen
	}
	if err := a.Cache.Put(pr); err != nil {
		return model.PullRequest{}, err
	}
	return pr, nil
}

func (a *App) Inbox(sortMode SortMode) []model.PullRequest {
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
	prs, err := a.Cache.ListRepo(repo)
	if err != nil {
		return nil
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
	return a.GitHub.ClosePR(ctx, repo, number, comment)
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
	return a.Cache.SetSkip(repo, number, skipped)
}
