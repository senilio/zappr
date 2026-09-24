package github

import (
	"context"

	"github.com/senilio/zappr/internal/model"
)

type Provider interface {
	ListPRs(ctx context.Context, repo string) ([]model.PullRequest, bool, error)
	GetPR(ctx context.Context, repo string, number int) (model.PullRequest, error)
	GetDiff(ctx context.Context, repo string, number int) (string, error)
	GetCommitDiff(ctx context.Context, repo string, sha string) (string, error)
	GetFileContent(ctx context.Context, repo string, path string, ref string) (string, error)
	Approve(ctx context.Context, repo string, number int, body string) error
	Comment(ctx context.Context, repo string, number int, body string) error
	ToggleDraft(ctx context.Context, repo string, number int, isDraft bool) error
	ClosePR(ctx context.Context, repo string, number int, comment string) error
	UpdateBranch(ctx context.Context, repo string, number int) error
	RequestChanges(ctx context.Context, repo string, number int, body string) error
	MergePR(ctx context.Context, repo string, number int) error
}
