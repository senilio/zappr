package tui

import (
	"slices"
	"strings"
	"time"

	"github.com/senilio/zappr/internal/model"
)

type activityTone int

const (
	activityNeutral activityTone = iota
	activityApproved
	activityChanges
	activityStale
)

type activityEntry struct {
	Key, URL, Author, Action string
	At                       time.Time
	Comment                  model.PRComment
	tone                     activityTone
	possessive               bool
}

func (m Model) activityAuthor(login string) string {
	name := strings.TrimSuffix(login, "[bot]")
	if alias := strings.TrimSpace(readerInput(m.cfg.AuthorAliases[name])); alias != "" {
		return alias
	}
	if name = readerInput(name); name != "" {
		return name
	}
	return "unknown"
}

func (m Model) activityEntries(pr model.PullRequest) []activityEntry {
	reviews := make(map[string]model.PRReview, len(pr.Reviews))
	dismissals := make(map[string]model.PRReviewDismissal, len(pr.ReviewDismissals))
	for _, d := range pr.ReviewDismissals {
		if old, found := dismissals[d.ReviewID]; d.ReviewID != "" && (!found || d.CreatedAt.After(old.CreatedAt)) {
			dismissals[d.ReviewID] = d
		}
	}
	latest := make(map[string]int)
	for i, r := range pr.Reviews {
		if r.ID != "" {
			reviews[r.ID] = r
		}
		switch r.State {
		case "APPROVED", "CHANGES_REQUESTED", "DISMISSED":
			if prev, found := latest[r.Author]; !found || !r.SubmittedAt.Before(pr.Reviews[prev].SubmittedAt) {
				latest[r.Author] = i
			}
		}
	}
	entries := make([]activityEntry, 0, len(pr.Comments)+len(pr.Reviews)+len(pr.ReviewDismissals))
	for _, c := range pr.Comments {
		action := "commented"
		if c.Kind == "inline" {
			action = "commented inline"
		}
		if c.Kind == "review" {
			if _, found := reviews[c.ID]; found {
				continue
			}
			action = "reviewed"
		}
		entries = append(entries, activityEntry{Key: c.Key(), URL: c.URL, Author: c.Author, Action: action, At: c.CreatedAt, Comment: c})
	}
	for i, r := range pr.Reviews {
		if r.State == "PENDING" {
			continue
		}
		c := model.PRComment{ID: r.ID, URL: r.URL, Author: r.Author, Body: r.Body, Kind: "review", CreatedAt: r.SubmittedAt}
		entry := activityEntry{Key: c.Key(), URL: r.URL, Author: r.Author, Action: "reviewed", At: r.SubmittedAt, Comment: c}
		d, dismissed := dismissals[r.ID]
		active := pr.ReviewActivityLoaded && !dismissed && r.Author != "" && latest[r.Author] == i
		switch {
		case dismissed && d.PreviousState == "APPROVED" && d.Stale:
			entry.Action = "approved (now stale)"
		case dismissed && d.PreviousState == "APPROVED":
			entry.Action = "approved (now dismissed)"
		case dismissed && d.PreviousState == "CHANGES_REQUESTED":
			entry.Action = "requested changes (now dismissed)"
		case r.State == "APPROVED":
			entry.Action = "approved"
			if active {
				entry.tone = activityApproved
			}
		case r.State == "CHANGES_REQUESTED":
			entry.Action = "requested changes"
			if active {
				entry.tone = activityChanges
			}
		case r.State == "DISMISSED":
			entry.Action = "reviewed (now dismissed)"
		}
		entries = append(entries, entry)
	}
	for _, d := range pr.ReviewDismissals {
		entry := activityEntry{Key: "dismissal:" + d.ID, URL: d.URL, Author: reviews[d.ReviewID].Author, Action: "review dismissed", At: d.CreatedAt, possessive: true}
		switch d.PreviousState {
		case "APPROVED":
			entry.Action = "approval dismissed"
			if d.Stale {
				entry.Action = "approval became stale"
				entry.tone = activityStale
			}
		case "CHANGES_REQUESTED":
			entry.Action = "change request dismissed"
		}
		entries = append(entries, entry)
	}
	slices.SortStableFunc(entries, func(a, b activityEntry) int { return b.At.Compare(a.At) })
	return entries
}

func (m Model) selectedActivity() (activityEntry, bool) {
	pr, ok := m.current()
	if !ok {
		return activityEntry{}, false
	}
	entries := m.activityEntries(pr)
	if m.commentState.selectedKey != "" {
		for _, entry := range entries {
			if entry.Key == m.commentState.selectedKey {
				return entry, true
			}
		}
		return activityEntry{}, false
	}
	if m.commentIndex >= 0 && m.commentIndex < len(entries) {
		return entries[m.commentIndex], true
	}
	return activityEntry{}, false
}
