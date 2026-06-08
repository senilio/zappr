package cache

import "github.com/senilio/zappr/internal/model"

type Store interface {
	Put(model.PullRequest) error
	Get(repo string, number int) (model.PullRequest, error)
	List() ([]model.PullRequest, error)
	PruneRepo(repo string, openNumbers map[int]bool) error
	SetSkip(repo string, number int, skipped bool) error
	Clear() error
	Path() string
}
