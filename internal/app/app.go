package app

import (
	"context"
	"sort"
	"time"

	"github.com/senilio/zappr/internal/ai"
	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/github"
	"github.com/senilio/zappr/internal/model"
)

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

type App struct {
	GitHub         github.Provider
	AI             ai.Provider
	Cache          cache.Store
	TrustedAuthors map[string]bool
}

func New(githubProvider github.Provider, aiProvider ai.Provider, store cache.Store) *App {
	if aiProvider == nil {
		aiProvider = ai.NoopProvider{}
	}
	return &App{GitHub: githubProvider, AI: aiProvider, Cache: store}
}

// AIEnabled reports whether a real AI provider is configured.
func (a *App) AIEnabled() bool {
	if a.AI == nil {
		return false
	}
	_, noop := a.AI.(ai.NoopProvider)
	return !noop
}

func (a *App) Refresh(ctx context.Context, repo string) ([]model.PullRequest, error) {
	prs, err := a.GitHub.ListOpenPRs(ctx, repo)
	if err != nil {
		return nil, err
	}
	for i := range prs {
		// Preserve any existing AI triage from the cache so that a manual
		// "r" analysis is not silently wiped on the next background refresh.
		// AI is only (re-)run by Analyze, which has the full diff available.
		if cached, err := a.Cache.Get(prs[i].Repo, prs[i].Number); err == nil && cached.Triage.AI != nil {
			prs[i].Triage.AI = cached.Triage.AI
		}
		if err := a.Cache.Put(prs[i]); err != nil {
			return nil, err
		}
	}
	openNumbers := make(map[int]bool, len(prs))
	for _, pr := range prs {
		openNumbers[pr.Number] = true
	}
	if err := a.Cache.PruneRepo(repo, openNumbers); err != nil {
		return nil, err
	}
	return a.InboxRepo(repo, SortUpdated), nil
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

func (a *App) Analyze(ctx context.Context, repo string, number int) (model.PullRequest, error) {
	pr, err := a.GitHub.GetPR(ctx, repo, number)
	if err != nil {
		return model.PullRequest{}, err
	}
	diff, err := a.GitHub.GetDiff(ctx, repo, number)
	if err != nil {
		return model.PullRequest{}, err
	}
	pr.Diff = diff
	if aiTriage, err := a.AI.Triage(ctx, pr); err != nil {
		pr.Triage.AI = &model.AITriage{Error: err.Error()}
	} else if aiTriage != nil {
		pr.Triage.AI = aiTriage
	}
	if err := a.Cache.Put(pr); err != nil {
		return model.PullRequest{}, err
	}
	return pr, nil
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
