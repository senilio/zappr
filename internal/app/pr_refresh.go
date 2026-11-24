package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/senilio/zappr/internal/model"
)

var ErrRefreshSuperseded = errors.New("refresh superseded")

type PRKey struct {
	Repo   string
	Number int
}

// PRUpdate is an accepted observation, or (at revision zero) a display-only
// cache entry. ClosedRevision survives reopening to invalidate old intents.
type PRUpdate struct {
	Key                      PRKey
	PR                       model.PullRequest
	Revision, ClosedRevision uint64
	Closed                   bool
}

func (a *App) nextRevisionLocked() uint64 {
	a.revision++
	return a.revision
}

func (a *App) BeginInboxRefresh(repo string) uint64 {
	a.cacheRefreshMu.Lock()
	defer a.cacheRefreshMu.Unlock()
	if a.inboxRequests == nil {
		a.inboxRequests = make(map[string]uint64)
	}
	id := a.nextRevisionLocked()
	a.inboxRequests[repo] = id
	return id
}

// RefreshInbox returns the unfiltered snapshot and versioned removals. On a
// cache error, the removals are still valid; the snapshot is not.
func (a *App) RefreshInbox(ctx context.Context, repo string, id uint64) ([]PRUpdate, error) {
	prs, complete, fetchErr := a.GitHub.ListPRs(ctx, repo)
	a.cacheRefreshMu.Lock()
	defer a.cacheRefreshMu.Unlock()
	if id == 0 || id != a.inboxRequests[repo] {
		return nil, ErrRefreshSuperseded
	}
	if fetchErr != nil {
		return nil, fetchErr
	}
	for _, pr := range prs {
		if pr.Repo != repo || pr.Number <= 0 || (pr.State != "" && pr.State != "OPEN") {
			return nil, fmt.Errorf("unexpected open PR in listing for %s", repo)
		}
	}

	cached, readErr := a.Cache.ListRepo(repo)
	cacheByNumber := make(map[int]model.PullRequest, len(cached))
	for _, pr := range cached {
		cacheByNumber[pr.Number] = pr
		a.rememberCachedPRLocked(pr)
	}
	listed := make(map[int]bool, len(prs))
	var writes []model.PullRequest
	for _, pr := range prs {
		key := PRKey{Repo: repo, Number: pr.Number}
		listed[pr.Number] = true
		previous := a.prUpdates[key]
		if readErr != nil && previous.PR.Number == 0 && !previous.Closed {
			// Without a baseline, this open row could be older than the
			// unreadable cache. Retain membership, but don't install it.
			a.trackPRLocked(key)
			continue
		}
		update, err := a.observePRLocked(key, id, pr, cacheByNumber[pr.Number], true)
		if err == nil {
			writes = append(writes, update.PR)
		}
	}
	if complete {
		for key, previous := range a.prUpdates {
			if key.Repo == repo && !listed[key.Number] && previous.Revision < id {
				a.closePRLocked(key)
			}
		}
	}

	removals := make([]PRUpdate, 0)
	open := make([]PRUpdate, 0, len(cached)+len(prs))
	for key, update := range a.prUpdates {
		if key.Repo != repo {
			continue
		}
		switch {
		case update.Closed:
			removals = append(removals, update)
		case update.Revision != 0:
			open = append(open, update)
		case cacheByNumber[key.Number].Number != 0:
			open = append(open, update)
		}
	}
	if readErr != nil {
		return removals, readErr
	}
	for _, pr := range writes {
		if err := a.Cache.Put(pr); err != nil {
			return removals, err
		}
	}
	for _, update := range removals {
		if cacheByNumber[update.Key.Number].Number != 0 {
			if err := a.Cache.Delete(repo, update.Key.Number); err != nil {
				return removals, err
			}
		}
	}
	return append(open, removals...), nil
}

func (a *App) RefreshPRUpdate(ctx context.Context, key PRKey) (PRUpdate, error) {
	id := a.beginPRRead(key)
	pr, err := a.GitHub.GetPR(ctx, key.Repo, key.Number)
	if err != nil {
		return PRUpdate{}, err
	}
	return a.savePRObservation(key, id, pr, false)
}

func (a *App) beginPRRead(key PRKey) uint64 {
	a.cacheRefreshMu.Lock()
	defer a.cacheRefreshMu.Unlock()
	a.trackPRLocked(key)
	return a.nextRevisionLocked()
}

func (a *App) trackPRLocked(key PRKey) {
	if a.prUpdates == nil {
		a.prUpdates = make(map[PRKey]PRUpdate)
	}
	if _, ok := a.prUpdates[key]; !ok {
		a.prUpdates[key] = PRUpdate{Key: key}
	}
}

func (a *App) rememberCachedPRLocked(pr model.PullRequest) {
	key := PRKey{Repo: pr.Repo, Number: pr.Number}
	a.trackPRLocked(key)
	current := a.prUpdates[key]
	if current.Revision == 0 && !updatedAfter(current.PR, pr) {
		// Keep the display-only baseline if a later cache read fails.
		current.PR = pr
		a.prUpdates[key] = current
	}
}

func (a *App) savePRObservation(key PRKey, id uint64, pr model.PullRequest, partial bool) (PRUpdate, error) {
	if pr.Number != key.Number || pr.Repo != key.Repo {
		return PRUpdate{}, fmt.Errorf("unexpected pull request: wanted %s#%d", key.Repo, key.Number)
	}
	a.cacheRefreshMu.Lock()
	defer a.cacheRefreshMu.Unlock()
	cached, cacheErr := a.Cache.Get(key.Repo, key.Number)
	if cacheErr == nil {
		a.rememberCachedPRLocked(cached)
	}
	update, err := a.observePRLocked(key, id, pr, cached, partial)
	if err != nil {
		return update, err
	}
	if update.Closed {
		return update, errors.Join(ErrPRNotOpen, a.Cache.Delete(key.Repo, key.Number))
	}
	return update, a.Cache.Put(update.PR)
}

func (a *App) observePRLocked(key PRKey, id uint64, pr, cached model.PullRequest, partial bool) (PRUpdate, error) {
	a.trackPRLocked(key)
	previous := a.prUpdates[key]
	if id <= previous.Revision {
		return previous, ErrRefreshSuperseded
	}
	switch pr.State {
	case "CLOSED", "MERGED":
		return a.closePRLocked(key), nil
	case "OPEN":
	case "":
		if previous.ClosedRevision != 0 {
			return previous, ErrRefreshSuperseded
		}
	default:
		return PRUpdate{}, fmt.Errorf("unknown pull request state %q", pr.State)
	}

	prior := previous.PR
	if prior.Number == 0 || updatedAfter(cached, prior) {
		prior = cached
	}
	// Don't compare against cached data from a previous open period.
	if !previous.Closed && updatedAfter(prior, pr) {
		if previous.PR.Number == 0 || updatedAfter(cached, previous.PR) {
			return PRUpdate{Key: key, PR: cached, ClosedRevision: previous.ClosedRevision}, ErrRefreshSuperseded
		}
		return previous, ErrRefreshSuperseded
	}
	if !previous.Closed {
		if partial {
			pr = preservePRDetails(pr, prior)
		}
		pr.Skipped = prior.Skipped
	}
	update := PRUpdate{Key: key, PR: pr, Revision: id, ClosedRevision: previous.ClosedRevision}
	a.prUpdates[key] = update
	return update, nil
}

func updatedAfter(a, b model.PullRequest) bool {
	return !a.UpdatedAt.IsZero() && !b.UpdatedAt.IsZero() && a.UpdatedAt.After(b.UpdatedAt)
}

func (a *App) ClosePRUpdate(ctx context.Context, key PRKey, body string) (PRUpdate, error) {
	if err := a.GitHub.ClosePR(ctx, key.Repo, key.Number, body); err != nil {
		return PRUpdate{}, err
	}
	a.cacheRefreshMu.Lock()
	defer a.cacheRefreshMu.Unlock()
	update := a.closePRLocked(key)
	return update, a.Cache.Delete(key.Repo, key.Number)
}

// Assign the barrier at acceptance, not when the read starts: another read
// could already be in flight even if it started after the closure request.
func (a *App) closePRLocked(key PRKey) PRUpdate {
	a.trackPRLocked(key)
	id := a.nextRevisionLocked()
	update := PRUpdate{Key: key, Revision: id, ClosedRevision: id, Closed: true}
	a.prUpdates[key] = update
	return update
}

func preservePRDetails(pr, prior model.PullRequest) model.PullRequest {
	sameUpdate := prior.UpdatedAt.Equal(pr.UpdatedAt)
	sameVersion := sameUpdate && prior.HeadRefOID == pr.HeadRefOID
	pr.Commits = prior.Commits
	if pr.ConversationCommentsLoaded {
		// A guard check fetches conversation but not reviews or inline
		// comments. Replace only the evidence it actually read.
		comments := append([]model.PRComment(nil), pr.Comments...)
		for _, c := range prior.Comments {
			if c.Kind == "review" || c.Kind == "inline" {
				comments = append(comments, c)
			}
		}
		pr.Comments = comments
	} else {
		pr.Comments = prior.Comments
		pr.ConversationCommentsLoaded = sameVersion && prior.ConversationCommentsLoaded
	}
	pr.Reviews = prior.Reviews
	pr.ReviewDismissals = prior.ReviewDismissals
	pr.ReviewActivityLoaded = sameVersion && prior.ReviewActivityLoaded
	if sameVersion && prior.DetailsLoaded && prior.ReviewCommentsLoaded && pr.ConversationCommentsLoaded {
		pr.DetailsLoaded = true
		pr.ReviewCommentsLoaded = true
	}
	if sameUpdate && prior.ReviewersLoaded {
		pr.Reviewers = prior.Reviewers
		pr.ReviewersLoaded = true
	}
	// The list query often reports UNKNOWN while GitHub recomputes mergeability.
	if (pr.MergeState == "" || pr.MergeState == "UNKNOWN") && prior.MergeState != "" {
		pr.MergeState = prior.MergeState
	}
	return pr
}
