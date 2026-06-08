package cache

import (
	"strconv"

	"github.com/senilio/zappr/internal/model"
)

type MemoryStore struct {
	prs map[string]model.PullRequest
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{prs: map[string]model.PullRequest{}}
}

func (s *MemoryStore) Put(pr model.PullRequest) error {
	key := cacheKey(pr.Repo, pr.Number)
	if old, ok := s.prs[key]; ok {
		pr.Skipped = old.Skipped
	}
	s.prs[key] = pr
	return nil
}

func (s *MemoryStore) List() ([]model.PullRequest, error) {
	out := make([]model.PullRequest, 0, len(s.prs))
	for _, pr := range s.prs {
		out = append(out, pr)
	}
	return out, nil
}

func (s *MemoryStore) Skip(repo string, number int) error {
	key := cacheKey(repo, number)
	pr, ok := s.prs[key]
	if !ok {
		return nil
	}
	pr.Skipped = true
	s.prs[key] = pr
	return nil
}

func (s *MemoryStore) Clear() error {
	s.prs = map[string]model.PullRequest{}
	return nil
}

func (s *MemoryStore) Path() string {
	return "memory"
}

func cacheKey(repo string, number int) string {
	return repo + "#" + strconv.Itoa(number)
}
