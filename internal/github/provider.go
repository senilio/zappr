package github

import (
	"context"

	"github.com/senilio/zappr/internal/model"
)

type Provider interface {
	ListOpenPRs(ctx context.Context, repo string) ([]model.PullRequest, error)
	GetPR(ctx context.Context, repo string, number int) (model.PullRequest, error)
	GetDiff(ctx context.Context, repo string, number int) (string, error)
	Approve(ctx context.Context, repo string, number int, body string) error
	Comment(ctx context.Context, repo string, number int, body string) error
}
