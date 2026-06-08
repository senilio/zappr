package ai

import (
	"context"

	"github.com/senilio/zappr/internal/model"
)

type Provider interface {
	Triage(ctx context.Context, pr model.PullRequest) (*model.AITriage, error)
}

type NoopProvider struct{}

func (NoopProvider) Triage(context.Context, model.PullRequest) (*model.AITriage, error) {
	return nil, nil
}
