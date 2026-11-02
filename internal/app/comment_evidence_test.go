package app

import (
	"context"
	"errors"
	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/model"
	"testing"
	"time"
)

func TestRefreshRetainsLegacyCommentEvidenceForUpgrade(t *testing.T) {
	testEvidenceRefresh(t, false)
}

func TestRefreshInvalidatesMarkersButRetainsOldCommentEvidence(t *testing.T) {
	testEvidenceRefresh(t, true)
}

func testEvidenceRefresh(t *testing.T, changed bool) {
	t.Helper()
	store := cache.NewMemoryStore()
	old := model.PullRequest{Repo: "example/infra", Number: 7, UpdatedAt: time.Unix(100, 0), DetailsLoaded: true, ReviewCommentsLoaded: true, ConversationCommentsLoaded: changed, Comments: []model.PRComment{{Body: "old evidence"}}, Commits: []model.PRCommit{{OID: "abc"}}}
	if err := store.Put(old); err != nil {
		t.Fatal(err)
	}
	list := old
	list.Comments = nil
	list.Commits = nil
	list.DetailsLoaded = false
	list.ReviewCommentsLoaded = false
	list.ConversationCommentsLoaded = false
	if changed {
		list.UpdatedAt = list.UpdatedAt.Add(time.Second)
	}
	prs, err := New(fakeGitHub{prs: []model.PullRequest{list}}, store).Refresh(context.Background(), old.Repo, true)
	if err != nil {
		t.Fatal(err)
	}
	got := prs[0]
	if len(got.Comments) != 1 || len(got.Commits) != 1 || got.DetailsLoaded || got.ReviewCommentsLoaded || got.ConversationCommentsLoaded {
		t.Fatal(got)
	}
}

type conversationFailure struct{ fakeGitHub }

func (conversationFailure) GetPR(context.Context, string, int) (model.PullRequest, error) {
	return model.PullRequest{}, errors.New("conversation rate limit")
}

func TestRefreshPRConversationFailurePreservesCache(t *testing.T) {
	store := cache.NewMemoryStore()
	old := model.PullRequest{Repo: "example/infra", Number: 7, Comments: []model.PRComment{{Body: "keep"}}}
	if err := store.Put(old); err != nil {
		t.Fatal(err)
	}
	if _, err := New(conversationFailure{}, store).RefreshPR(context.Background(), old.Repo, 7); err == nil {
		t.Fatal("missing error")
	}
	got, err := store.Get(old.Repo, 7)
	if err != nil || len(got.Comments) != 1 || got.Comments[0].Body != "keep" {
		t.Fatal(got, err)
	}
}
