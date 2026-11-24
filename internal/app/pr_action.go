package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/senilio/zappr/internal/github"
)

const prVerificationMaxAge = 30 * time.Second

// CheckPRIntent rejects a write from an open period that App knows has ended.
// It does not fetch GitHub state or require the UI to have received the removal.
func (a *App) CheckPRIntent(key PRKey, closedRevision uint64) error {
	a.cacheRefreshMu.Lock()
	current := a.prUpdates[key]
	a.cacheRefreshMu.Unlock()
	if current.Closed || current.ClosedRevision != closedRevision {
		return fmt.Errorf("PR closed or reopened before sending: %w", ErrPRNotOpen)
	}
	return nil
}

type PRVerification struct {
	Check     github.PRActionCheck
	Update    PRUpdate
	CheckedAt time.Time
}

// VerifyPRAction never substitutes cached display data for the live check.
func (a *App) VerifyPRAction(ctx context.Context, key PRKey, guarded bool) (PRVerification, error) {
	id := a.beginPRRead(key)
	check, err := a.GitHub.CheckPRAction(ctx, key.Repo, key.Number, guarded)
	if err != nil {
		return PRVerification{}, err
	}
	proof := PRVerification{Check: check, CheckedAt: time.Now()}
	switch check.PR.State {
	case "CLOSED", "MERGED":
	case "OPEN":
		if strings.TrimSpace(check.PR.HeadRefOID) == "" {
			return proof, fmt.Errorf("missing PR head ref")
		}
	default:
		return proof, fmt.Errorf("unknown pull request state %q", check.PR.State)
	}
	proof.Update, err = a.savePRObservation(key, id, check.PR, true)
	if err != nil && !proof.Update.Closed {
		// A rejected read can carry a newer row for display. Its revision
		// belongs to another observation, not to the check in this proof.
		proof.Update = PRUpdate{}
	}
	return proof, err
}

// PostPRAction checks facts known to zappr at dispatch. GitHub's comment
// endpoint has no compare-and-swap for the PR head or state.
func (a *App) PostPRAction(ctx context.Context, key PRKey, body string, proof PRVerification, expectedClosedRevision uint64) error {
	pr := proof.Check.PR
	a.cacheRefreshMu.Lock()
	current := a.prUpdates[key]
	age := time.Since(proof.CheckedAt)
	valid := proof.Update.Key == key && pr.Repo == key.Repo && pr.Number == key.Number &&
		pr.State == "OPEN" && pr.HeadRefOID != "" && current.PR.State == "OPEN" && current.PR.HeadRefOID == pr.HeadRefOID &&
		current.Revision != 0 && current.Revision == proof.Update.Revision && !current.Closed && !proof.Update.Closed &&
		current.ClosedRevision == expectedClosedRevision && proof.Update.ClosedRevision == expectedClosedRevision &&
		age >= 0 && age <= prVerificationMaxAge
	a.cacheRefreshMu.Unlock()
	if !valid {
		return fmt.Errorf("PR changed or verification expired before sending: %w", ErrRefreshSuperseded)
	}
	return a.Comment(ctx, key.Repo, key.Number, body)
}
