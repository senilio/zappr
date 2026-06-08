package cache

import "github.com/senilio/zappr/internal/model"

type Store interface {
	Put(model.PullRequest) error
	List() ([]model.PullRequest, error)
	Skip(repo string, number int) error
	Clear() error
	Path() string
}
