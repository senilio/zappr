package app

import (
	"context"
	"sort"

	"github.com/senilio/zappr/internal/ai"
	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/github"
	"github.com/senilio/zappr/internal/model"
	"github.com/senilio/zappr/internal/risk"
)

type App struct {
	GitHub github.Provider
	AI     ai.Provider
	Cache  cache.Store
}

func New(githubProvider github.Provider, aiProvider ai.Provider, store cache.Store) *App {
	if aiProvider == nil {
		aiProvider = ai.NoopProvider{}
	}
	return &App{GitHub: githubProvider, AI: aiProvider, Cache: store}
}

func (a *App) Refresh(ctx context.Context, repo string) ([]model.PullRequest, error) {
	prs, err := a.GitHub.ListOpenPRs(ctx, repo)
	if err != nil {
		return nil, err
	}
	for i := range prs {
		prs[i].Triage = risk.Classify(prs[i])
		if aiTriage, err := a.AI.Triage(ctx, prs[i]); err == nil && aiTriage != nil {
			prs[i].Triage.AI = aiTriage
		}
		if err := a.Cache.Put(prs[i]); err != nil {
			return nil, err
		}
	}
	return a.Inbox(), nil
}

func (a *App) Inbox() []model.PullRequest {
	prs, err := a.Cache.List()
	if err != nil {
		return nil
	}
	sort.Slice(prs, func(i, j int) bool {
		if prs[i].Skipped != prs[j].Skipped {
			return !prs[i].Skipped
		}
		if riskWeight(prs[i].Triage.Risk) != riskWeight(prs[j].Triage.Risk) {
			return riskWeight(prs[i].Triage.Risk) > riskWeight(prs[j].Triage.Risk)
		}
		return prs[i].Number < prs[j].Number
	})
	return prs
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
	pr.Triage = risk.Classify(pr)
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
