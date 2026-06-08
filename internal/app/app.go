package app

import (
	"context"
	"sort"
	"time"

	"github.com/senilio/zappr/internal/ai"
	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/github"
	"github.com/senilio/zappr/internal/model"
	"github.com/senilio/zappr/internal/risk"
)

type SortMode int

const (
	SortUpdated SortMode = iota
	SortNumber
	SortRisk
)

func (m SortMode) String() string {
	switch m {
	case SortNumber:
		return "number"
	case SortRisk:
		return "risk"
	default:
		return "updated"
	}
}

func (m SortMode) Next() SortMode {
	switch m {
	case SortUpdated:
		return SortNumber
	case SortNumber:
		return SortRisk
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
		prs[i].Triage = risk.ClassifyWithTrust(prs[i], a.TrustedAuthors)
		if aiTriage, err := a.AI.Triage(ctx, prs[i]); err == nil && aiTriage != nil {
			prs[i].Triage.AI = aiTriage
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
	return a.Inbox(SortUpdated), nil
}

func (a *App) Inbox(sortMode SortMode) []model.PullRequest {
	prs, err := a.Cache.List()
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
		case SortRisk:
			if riskWeight(prs[i].Triage.Risk) != riskWeight(prs[j].Triage.Risk) {
				return riskWeight(prs[i].Triage.Risk) > riskWeight(prs[j].Triage.Risk)
			}
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
	pr.Triage = risk.ClassifyWithTrust(pr, a.TrustedAuthors)
	if aiTriage, err := a.AI.Triage(ctx, pr); err == nil && aiTriage != nil {
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

func (a *App) Skip(repo string, number int) error {
	return a.Cache.Skip(repo, number)
}

func riskWeight(r model.Risk) int {
	switch r {
	case model.RiskBlock:
		return 3
	case model.RiskReview:
		return 2
	case model.RiskSafe:
		return 1
	default:
		return 0
	}
}
