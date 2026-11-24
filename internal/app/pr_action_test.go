package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/github"
	"github.com/senilio/zappr/internal/model"
)

type actionProvider struct {
	fakeGitHub
	check    github.PRActionCheck
	verify   func() github.PRActionCheck
	comments int
}

func (p *actionProvider) CheckPRAction(context.Context, string, int, bool) (github.PRActionCheck, error) {
	if p.verify != nil {
		return p.verify(), nil
	}
	return p.check, nil
}

func (p *actionProvider) Comment(context.Context, string, int, string) error {
	p.comments++
	return nil
}

var errSaveObservation = errors.New("cache write failed")

type actionStore struct {
	*cache.MemoryStore
	failPut bool
}

func (s *actionStore) Put(pr model.PullRequest) error {
	if s.failPut {
		return errSaveObservation
	}
	return s.MemoryStore.Put(pr)
}

func TestRejectedVerificationCannotBorrowAnotherRevision(t *testing.T) {
	key := PRKey{Repo: refreshRepo, Number: 1}
	pr := refreshPR(1, "same-head")
	newer := pr
	newer.Checks = []model.Check{{Context: "atlantis/plan", State: "PENDING"}}
	wait, started, release := pauseRefresh(t)
	provider := &actionProvider{fakeGitHub: fakeGitHub{pr: newer}, verify: func() github.PRActionCheck {
		wait()
		return github.PRActionCheck{PR: pr, ChecksKnown: true}
	}}
	a := New(provider, cache.NewMemoryStore())
	inFlight := runRefresh(func() (PRVerification, error) { return a.VerifyPRAction(t.Context(), key, true) })
	<-started
	_, acceptErr := a.RefreshPRUpdate(t.Context(), key)
	release()
	rejected := <-inFlight
	if acceptErr != nil || !errors.Is(rejected.err, ErrRefreshSuperseded) {
		t.Fatalf("new observation error = %v; verification error = %v", acceptErr, rejected.err)
	}
	if err := a.PostPRAction(t.Context(), key, "atlantis plan", rejected.data, 0); err == nil || provider.comments != 0 {
		t.Fatalf("rejected verification borrowed the newer revision and posted: %v, comments=%d", err, provider.comments)
	}
}

func TestPostPRActionRejectsSupersededProof(t *testing.T) {
	key := PRKey{Repo: refreshRepo, Number: 1}
	for _, scenario := range []string{"newer cache", "newer head", "newer head with cache error", "closed", "closed and reopened", "expired", "current"} {
		t.Run(scenario, func(t *testing.T) {
			pr := refreshPR(1, "old")
			provider := &actionProvider{check: github.PRActionCheck{PR: pr}}
			store := &actionStore{MemoryStore: cache.NewMemoryStore()}
			a := New(provider, store)
			if scenario == "newer cache" {
				newer := refreshPR(1, "new")
				newer.UpdatedAt = pr.UpdatedAt.Add(time.Second)
				if err := store.Put(newer); err != nil {
					t.Fatal(err)
				}
				proof, err := a.VerifyPRAction(t.Context(), key, false)
				if !errors.Is(err, ErrRefreshSuperseded) || proof.Check.PR.HeadRefOID == "new" {
					t.Fatalf("verification must not substitute cached target: %+v, %v", proof, err)
				}
				if err := a.PostPRAction(t.Context(), key, "atlantis plan", proof, 0); err == nil || provider.comments != 0 {
					t.Fatalf("unaccepted proof posted: %v, comments=%d", err, provider.comments)
				}
				return
			}
			proof, err := a.VerifyPRAction(t.Context(), key, false)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "newer head", "newer head with cache error":
				provider.pr = refreshPR(1, "new")
				store.failPut = scenario == "newer head with cache error"
				_, err := a.RefreshPRUpdate(t.Context(), key)
				if (store.failPut && !errors.Is(err, errSaveObservation)) || (!store.failPut && err != nil) {
					t.Fatalf("new observation error = %v", err)
				}
			case "closed", "closed and reopened":
				if _, err := a.ClosePRUpdate(t.Context(), key, ""); err != nil {
					t.Fatal(err)
				}
				if scenario == "closed and reopened" {
					proof, err = a.VerifyPRAction(t.Context(), key, false)
					if err != nil || proof.Update.ClosedRevision == 0 {
						t.Fatalf("reopen: %+v, %v", proof, err)
					}
				}
			case "expired":
				proof.CheckedAt = proof.CheckedAt.Add(-time.Minute)
			}
			err = a.PostPRAction(t.Context(), key, "atlantis plan", proof, 0)
			if scenario == "current" {
				if err != nil || provider.comments != 1 {
					t.Fatalf("verified action not dispatched: %v, comments=%d", err, provider.comments)
				}
			} else if err == nil || provider.comments != 0 {
				t.Fatalf("stale action dispatched: %v, comments=%d", err, provider.comments)
			}
		})
	}
}

func TestCheckPRActionKeepsFreshConversationWithoutClaimingFullDetails(t *testing.T) {
	pr := refreshPR(1, "old")
	pr.Comments = []model.PRComment{{ID: "fresh"}}
	pr.ConversationCommentsLoaded = true
	provider := &actionProvider{check: github.PRActionCheck{PR: pr, ChecksKnown: true}}
	store := cache.NewMemoryStore()
	prior := pr
	prior.Comments = []model.PRComment{{ID: "stale"}, {ID: "inline", Kind: "inline"}}
	if err := store.Put(prior); err != nil {
		t.Fatal(err)
	}
	a := New(provider, store)
	proof, err := a.VerifyPRAction(t.Context(), PRKey{Repo: refreshRepo, Number: 1}, true)
	got := proof.Update.PR
	if err != nil || !got.ConversationCommentsLoaded || got.DetailsLoaded || len(got.Comments) != 2 || got.Comments[0].ID != "fresh" || got.Comments[1].ID != "inline" {
		t.Fatalf("partial verification = %+v, %v", got, err)
	}
}
