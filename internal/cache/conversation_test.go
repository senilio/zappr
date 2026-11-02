package cache

import (
	"encoding/json"
	"github.com/senilio/zappr/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStorePersistsConversationProvenance(t *testing.T) {
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	pr := model.PullRequest{Repo: "example/infra", Number: 7, ConversationCommentsLoaded: true, Comments: []model.PRComment{{ID: "101", URL: "https://github.com/example/infra/issues/7#issuecomment-101", UpdatedAt: time.Unix(100, 0).UTC()}}}
	if err := s.Put(pr); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(pr.Repo, pr.Number)
	if err != nil || !got.ConversationCommentsLoaded || got.Comments[0].ID != "101" || !got.Comments[0].UpdatedAt.Equal(pr.Comments[0].UpdatedAt) {
		t.Fatal(got, err)
	}
}

func TestSQLiteLegacyCommentsRequestProvenanceUpgrade(t *testing.T) {
	var pr model.PullRequest
	if err := json.Unmarshal([]byte(`{"Repo":"example/infra","Number":7,"details_loaded":true,"Comments":[{"Body":"legacy"}]}`), &pr); err != nil {
		t.Fatal(err)
	}
	if pr.ConversationCommentsLoaded || len(pr.Comments) != 1 || pr.Comments[0].Key() == "" {
		t.Fatal(pr)
	}
}
