package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func TestActivityRows(t *testing.T) {
	m := nativeCommentModel(t)
	at := time.Now()
	pr := &m.prs[0]
	pr.ReviewActivityLoaded = true
	pr.Comments = []model.PRComment{
		{ID: "c", Author: "trustly-atlantis-bot[bot]", CreatedAt: at, Body: "comment"},
		{ID: "r1", Kind: "review", Body: "review body"},
	}
	pr.Reviews = []model.PRReview{
		{ID: "r1", Author: "alice", State: "DISMISSED", SubmittedAt: at.Add(time.Minute), Body: "review body", URL: "https://github.com/review"},
		{ID: "r2", Author: "bob", State: "CHANGES_REQUESTED", SubmittedAt: at.Add(2 * time.Minute)},
		{ID: "r3", Author: "carol", State: "APPROVED", SubmittedAt: at.Add(3 * time.Minute)},
		{ID: "r4", Author: "dan", State: "DISMISSED", SubmittedAt: at},
		{ID: "r5", Author: "carol", State: "CHANGES_REQUESTED", SubmittedAt: at},
	}
	pr.ReviewDismissals = []model.PRReviewDismissal{
		{ID: "d1", ReviewID: "r1", PreviousState: "APPROVED", Stale: true, CreatedAt: at.Add(4 * time.Minute), URL: "https://github.com/dismiss"},
		{ID: "d2", ReviewID: "r4", PreviousState: "APPROVED", CreatedAt: at.Add(5 * time.Minute)},
	}
	pr.HeadRefOID = "new-push"
	m.cfg.AuthorAliases = map[string]string{"trustly-atlantis-bot": "Atlantis"}
	m.reconcileComments()
	text, spans := m.summaryLayout(*pr, 80)
	for _, want := range []string{" - alice’s approval became stale", " - alice approved (now stale)", " - Atlantis commented", " - dan’s approval dismissed"} {
		if !strings.Contains(stripANSI(text), want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	entries := m.activityEntries(*pr)
	if len(spans) != 8 || entries[1].tone != activityStale || entries[2].tone != activityApproved || entries[3].tone != activityChanges || entries[4].tone != activityNeutral {
		t.Fatalf("rows: %+v, spans: %+v", entries, spans)
	}
	if entries[1].URL == entries[4].URL || entries[1].Key == entries[4].Key || entries[4].Comment.Body != "review body" {
		t.Fatalf("review and dismissal must stay separate: %+v", entries)
	}
}

func TestActivityWrapPreservesSelection(t *testing.T) {
	m := nativeCommentModel(t)
	m.prs[0].Comments[0].Author = "long-author-name-with-a-suffix[bot]"
	m.cfg.AuthorAliases = map[string]string{"long-author-name-with-a-suffix": strings.Repeat("Long Alias ", 5) + "FINISH"}
	text, spans := m.summaryLayout(m.prs[0], 30)
	if !strings.Contains(stripANSI(text), "commented") || strings.Contains(text, "FINISH") || spans[0].end != 1 {
		t.Fatalf("action lost or row overflowed: %q %+v", text, spans)
	}
	m.cfg.Wrap.Summary = true
	text, spans = m.summaryLayout(m.prs[0], 30)
	selected, _ := m.selectedActivity()
	if selected.Key != "comment:101" || spans[0].end <= 1 || !strings.Contains(text, "FINISH") {
		t.Fatalf("wrapped selection or spans lost: %+v %+v %q", selected, spans, text)
	}
}

func TestStaleActivityRendersWarningColor(t *testing.T) {
	m := nativeCommentModel(t)
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	entry := activityEntry{Author: "alice", Action: "approval became stale", tone: activityStale}
	action := m.theme.Styles.Wait.Bold(true).Render(entry.Action)
	if got := m.activityLine(entry, time.Now(), 80, true, false); !strings.HasSuffix(got, action) {
		t.Fatalf("stale action want %q, row was %q", action, got)
	}
}

func TestActivityAliasReload(t *testing.T) {
	m := nativeCommentModel(t)
	m.prs[0].Comments[0].Author = "appa[bot]"
	m.prs[0].Comments[1].Author = "keep[other]"
	m.reconcileComments()
	original, _ := m.selectedActivity()
	m.summaryLayout(m.prs[0], 80)
	cfg := m.cfg
	cfg.AuthorAliases = map[string]string{"appa": "New Name"}
	m.applyReloadedConfig(cfg, m.actions)
	text, _ := m.summaryLayout(m.prs[0], 80)
	selected, _ := m.selectedActivity()
	if !strings.Contains(stripANSI(text), "New Name commented") || !strings.Contains(stripANSI(text), "keep[other] commented") || selected.Key != original.Key || selected.Author != original.Author {
		t.Fatalf("alias reload changed identity or failed to repaint: %+v %q", selected, text)
	}
}
