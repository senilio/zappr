package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/model"
)

const refreshRepo = "owner/repo"

type refreshProvider struct {
	fakeGitHub
	list func() ([]model.PullRequest, bool, error)
	get  func() (model.PullRequest, error)
}

func (p refreshProvider) ListPRs(context.Context, string) ([]model.PullRequest, bool, error) {
	return p.list()
}

func (p refreshProvider) GetPR(context.Context, string, int) (model.PullRequest, error) {
	return p.get()
}

type refreshResult[T any] struct {
	data T
	err  error
}

func runRefresh[T any](fn func() (T, error)) <-chan refreshResult[T] {
	result := make(chan refreshResult[T], 1)
	go func() {
		data, err := fn()
		result <- refreshResult[T]{data, err}
	}()
	return result
}

func pauseRefresh(t *testing.T) (wait func(), started <-chan struct{}, release func()) {
	t.Helper()
	entered, resume := make(chan struct{}), make(chan struct{})
	return func() {
		close(entered)
		select {
		case <-resume:
		case <-t.Context().Done():
		}
	}, entered, func() { close(resume) }
}

func refreshPR(number int, head string) model.PullRequest {
	return model.PullRequest{Repo: refreshRepo, Number: number, State: "OPEN", HeadRefOID: head, UpdatedAt: time.Unix(10, 0)}
}

func TestRefreshSupersededDoesNotMutateCache(t *testing.T) {
	for _, failNew := range []bool{false, true} {
		t.Run(map[bool]string{false: "newer succeeds", true: "newer fails"}[failNew], func(t *testing.T) {
			store := cache.NewMemoryStore()
			wait, started, release := pauseRefresh(t)
			calls := 0
			listErr := errors.New("list rate limited")
			a := New(refreshProvider{list: func() ([]model.PullRequest, bool, error) {
				calls++
				if calls == 1 {
					wait()
					return []model.PullRequest{refreshPR(1, "old")}, true, nil
				}
				if failNew {
					return nil, false, listErr
				}
				return []model.PullRequest{refreshPR(1, "new"), refreshPR(2, "new")}, true, nil
			}}, store)
			first := runRefresh(func() ([]model.PullRequest, error) { return a.Refresh(t.Context(), refreshRepo, true) })
			<-started
			_, secondErr := a.Refresh(t.Context(), refreshRepo, true)
			release()
			old := <-first
			if failNew {
				if !errors.Is(secondErr, listErr) {
					t.Fatalf("second refresh error = %v, want %v", secondErr, listErr)
				}
				if prs, _ := store.ListRepo(refreshRepo); len(prs) != 0 {
					t.Fatalf("obsolete response wrote to cache after a newer failed request: %+v", prs)
				}
			} else {
				if secondErr != nil {
					t.Fatal(secondErr)
				}
				assertCachedHead(t, store, 1, "new")
				assertCachedHead(t, store, 2, "new")
			}
			if !errors.Is(old.err, ErrRefreshSuperseded) {
				t.Fatalf("obsolete response error = %v, want ErrRefreshSuperseded", old.err)
			}
		})
	}
}

func TestRefreshInboxAndPRShareFreshness(t *testing.T) {
	for _, omit := range []bool{false, true} {
		t.Run(map[bool]string{false: "old head", true: "omitted"}[omit], func(t *testing.T) {
			store := cache.NewMemoryStore()
			wait, started, release := pauseRefresh(t)
			a := New(refreshProvider{
				list: func() ([]model.PullRequest, bool, error) {
					wait()
					if omit {
						return nil, true, nil
					}
					return []model.PullRequest{refreshPR(1, "old")}, true, nil
				},
				get: func() (model.PullRequest, error) { return refreshPR(1, "new"), nil },
			}, store)
			first := runRefresh(func() ([]model.PullRequest, error) { return a.Refresh(t.Context(), refreshRepo, true) })
			<-started
			_, err := a.RefreshPR(t.Context(), refreshRepo, 1)
			release()
			inbox := <-first
			if err != nil || inbox.err != nil {
				t.Fatalf("refresh errors: PR %v; Inbox %v", err, inbox.err)
			}
			assertCachedHead(t, store, 1, "new")
			if len(inbox.data) != 1 || inbox.data[0].HeadRefOID != "new" {
				t.Fatalf("Inbox snapshot lost targeted result: %+v", inbox.data)
			}
		})
	}

	for _, inboxFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "targeted before list request", true: "targeted during list request"}[inboxFirst], func(t *testing.T) {
			store := cache.NewMemoryStore()
			if err := store.Put(refreshPR(1, "old")); err != nil {
				t.Fatal(err)
			}
			waitList, listStarted, releaseList := pauseRefresh(t)
			waitPR, prStarted, releasePR := pauseRefresh(t)
			a := New(refreshProvider{
				list: func() ([]model.PullRequest, bool, error) { waitList(); return nil, true, nil },
				get:  func() (model.PullRequest, error) { waitPR(); return refreshPR(1, "old"), nil },
			}, store)
			startList := func() <-chan refreshResult[[]model.PullRequest] {
				result := runRefresh(func() ([]model.PullRequest, error) { return a.Refresh(t.Context(), refreshRepo, true) })
				<-listStarted
				return result
			}
			var list <-chan refreshResult[[]model.PullRequest]
			if inboxFirst {
				list = startList()
			}
			targeted := runRefresh(func() (model.PullRequest, error) { return a.RefreshPR(t.Context(), refreshRepo, 1) })
			<-prStarted
			if !inboxFirst {
				list = startList()
			}
			releaseList()
			result := <-list
			releasePR()
			late := <-targeted
			if result.err != nil || len(result.data) != 0 {
				t.Fatalf("refresh with complete empty list = %+v", result)
			}
			if _, err := store.Get(refreshRepo, 1); err == nil {
				t.Fatal("late targeted result reinserted a closed PR")
			}
			if late.err == nil {
				t.Fatal("late open response remained valid after closure")
			}
		})
	}
}

func TestRefreshCloseBarrier(t *testing.T) {
	t.Run("local close", func(t *testing.T) {
		store := cache.NewMemoryStore()
		wait, started, release := pauseRefresh(t)
		a := New(refreshProvider{get: func() (model.PullRequest, error) {
			wait()
			return refreshPR(1, "old"), nil
		}}, store)
		targeted := runRefresh(func() (model.PullRequest, error) { return a.RefreshPR(t.Context(), refreshRepo, 1) })
		<-started
		key := PRKey{Repo: refreshRepo, Number: 1}
		closed, err := a.ClosePRUpdate(t.Context(), key, "")
		release()
		late := <-targeted
		if err != nil {
			t.Fatal(err)
		}
		if closed.Key != key || !closed.Closed || closed.ClosedRevision == 0 || !errors.Is(late.err, ErrRefreshSuperseded) {
			t.Fatalf("close = %+v; late result = %+v", closed, late)
		}
		if _, err := store.Get(refreshRepo, 1); err == nil {
			t.Fatal("open result from before a successful local close stayed cached")
		}
		a.GitHub = fakeGitHub{pr: refreshPR(1, "old")}
		reopened, err := a.RefreshPRUpdate(t.Context(), key)
		if err != nil || reopened.Closed || reopened.ClosedRevision != closed.ClosedRevision || reopened.Revision <= closed.Revision {
			t.Fatalf("reopened observation = %+v, %v", reopened, err)
		}
	})
	t.Run("explicit closure even with newer cached open", func(t *testing.T) {
		store := cache.NewMemoryStore()
		open := refreshPR(1, "old")
		if err := store.Put(open); err != nil {
			t.Fatal(err)
		}
		closed := open
		closed.State = "CLOSED"
		closed.UpdatedAt = open.UpdatedAt.Add(-time.Second)
		a := New(fakeGitHub{pr: closed}, store)
		if _, err := a.RefreshPR(t.Context(), refreshRepo, 1); !errors.Is(err, ErrPRNotOpen) {
			t.Fatalf("explicit closure masked by cache: %v", err)
		}
		if _, err := store.Get(refreshRepo, 1); err == nil {
			t.Fatal("closed PR still in cache")
		}
	})
}

func TestRefreshClosureUsesUnfilteredCompleteMembership(t *testing.T) {
	store := cache.NewMemoryStore()
	other := model.PullRequest{Repo: "other/repo", Number: 1}
	draft := refreshPR(1, "draft")
	draft.Draft = true
	for _, pr := range []model.PullRequest{other, draft, refreshPR(2, "open")} {
		if err := store.Put(pr); err != nil {
			t.Fatal(err)
		}
	}
	complete := false
	a := New(refreshProvider{list: func() ([]model.PullRequest, bool, error) {
		return []model.PullRequest{draft}, complete, nil
	}}, store)
	if prs, err := a.Refresh(t.Context(), refreshRepo, false); err != nil || len(prs) != 1 || prs[0].Number != 2 {
		t.Fatalf("incomplete list, hidden draft = %v, %v", prs, err)
	}
	assertCachedHead(t, store, 1, "draft")
	complete = true
	if prs, err := a.Refresh(t.Context(), refreshRepo, false); err != nil || len(prs) != 0 {
		t.Fatalf("complete list, hidden draft = %v, %v", prs, err)
	}
	if _, err := store.Get(refreshRepo, 2); err == nil {
		t.Fatal("complete list did not remove missing PR")
	}
	assertCachedHead(t, store, 1, "draft")
	if _, err := store.Get(other.Repo, other.Number); err != nil {
		t.Fatalf("refresh removed another repo's PR: %v", err)
	}
}

var errCacheRead = errors.New("cache read failed")

type refreshReadFailure struct {
	*cache.MemoryStore
}

func (refreshReadFailure) ListRepo(string) ([]model.PullRequest, error) {
	return nil, errCacheRead
}

type recoveringCache struct {
	*cache.MemoryStore
	failList bool
}

func (s *recoveringCache) ListRepo(repo string) ([]model.PullRequest, error) {
	if s.failList {
		return nil, errCacheRead
	}
	return s.MemoryStore.ListRepo(repo)
}

func TestRefreshRecoveryKeepsNewerCachedHead(t *testing.T) {
	for _, startup := range []bool{true, false} {
		t.Run(map[bool]string{true: "with startup read", false: "without startup read"}[startup], func(t *testing.T) {
			store := &recoveringCache{MemoryStore: cache.NewMemoryStore()}
			old, cached := refreshPR(1, "old"), refreshPR(1, "new")
			cached.UpdatedAt = old.UpdatedAt.Add(time.Second)
			if err := store.Put(cached); err != nil {
				t.Fatal(err)
			}
			a := New(fakeGitHub{prs: []model.PullRequest{old}}, store)
			if startup && len(a.InboxRepo(refreshRepo, SortUpdated)) != 1 {
				t.Fatal("cache row missing at startup")
			}
			store.failList = true
			if _, err := a.Refresh(t.Context(), refreshRepo, true); !errors.Is(err, errCacheRead) {
				t.Fatalf("failed refresh error = %v", err)
			}
			if startup && a.prUpdates[PRKey{Repo: refreshRepo, Number: 1}].PR.HeadRefOID != "new" {
				t.Fatal("failed refresh discarded a head already read from cache")
			}
			store.failList = false
			prs, err := a.Refresh(t.Context(), refreshRepo, true)
			if err != nil || len(prs) != 1 || prs[0].HeadRefOID != "new" {
				t.Fatalf("recovered snapshot rolled back the cached head: %+v, %v", prs, err)
			}
			assertCachedHead(t, store, 1, "new")
		})
	}
}

func TestRefreshPropagatesListRepoFailure(t *testing.T) {
	store := refreshReadFailure{cache.NewMemoryStore()}
	if err := store.Put(refreshPR(1, "cached")); err != nil {
		t.Fatal(err)
	}
	a := New(fakeGitHub{prs: []model.PullRequest{refreshPR(2, "listed")}}, store)
	if _, err := a.Refresh(t.Context(), refreshRepo, true); !errors.Is(err, errCacheRead) {
		t.Errorf("Refresh error = %v, want %v", err, errCacheRead)
	}
	assertCachedHead(t, store, 1, "cached")
	if _, err := store.Get(refreshRepo, 2); err == nil {
		t.Fatal("cache was written despite a failed snapshot read")
	}
	key := PRKey{Repo: refreshRepo, Number: 1}
	a.GitHub = fakeGitHub{pr: refreshPR(1, "cached")}
	if _, err := a.RefreshPRUpdate(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	updates, err := a.RefreshInbox(t.Context(), refreshRepo, a.BeginInboxRefresh(refreshRepo))
	found := false
	for _, update := range updates {
		found = found || update.Key == key && update.Closed
	}
	if !errors.Is(err, errCacheRead) || !found {
		t.Fatalf("cache read failure should retain closure evidence, got %+v, %v", updates, err)
	}
	assertCachedHead(t, store, 1, "cached")
}

func assertCachedHead(t *testing.T, store cache.Store, number int, want string) {
	t.Helper()
	pr, err := store.Get(refreshRepo, number)
	if err != nil || pr.HeadRefOID != want {
		t.Fatalf("cached #%d head = %q, %v, want %q", number, pr.HeadRefOID, err, want)
	}
}
