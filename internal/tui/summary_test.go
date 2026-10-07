package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/model"
)

func TestSummaryChronologyAndTimestamps(t *testing.T) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 14, 35, 0, 0, time.Local)
	yesterday := today.AddDate(0, 0, -1)
	older := today.AddDate(0, 0, -2)
	commits := []model.PRCommit{
		{OID: "older", Author: "alice", Date: older},
		{OID: "yesterday", Author: "alice", Date: yesterday},
		{OID: "today", Author: "alice", Date: today.UTC()},
		{OID: "undated", Author: "alice"},
	}
	comments := []model.PRComment{
		{Author: "alice", Body: "older", CreatedAt: older},
		{Author: "alice", Body: "yesterday", CreatedAt: yesterday},
		{Author: "alice", Body: "today", CreatedAt: today.UTC()},
		{Author: "alice", Body: "undated"},
	}
	pr := model.PullRequest{Commits: commits, Comments: comments}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	for _, kind := range []string{"commits", "comments"} {
		t.Run(kind, func(t *testing.T) {
			var b strings.Builder
			if kind == "commits" {
				m.writeCommitSummary(&b, pr, 80)
			} else {
				m.writeCommentSummary(&b, pr, 80)
			}
			view := stripANSI(b.String())
			for _, label := range []string{"Today 14:35", "Yesterday 14:35", older.Format("2006-01-02 15:04")} {
				if !strings.Contains(view, label) {
					t.Errorf("summary missing %q: %q", label, view)
				}
			}
			if todayIndex, yesterdayIndex, olderIndex := strings.Index(view, "today"), strings.Index(view, "yester"), strings.Index(view, "older"); todayIndex < 0 || yesterdayIndex < todayIndex || olderIndex < yesterdayIndex {
				t.Errorf("summary not newest first: %q", view)
			}
			if strings.Index(view, "undated") < strings.Index(view, "older") {
				t.Errorf("undated entry should be last: %q", view)
			}
		})
	}
	if pr.Commits[0].OID != "older" || pr.Comments[0].Body != "older" {
		t.Fatal("rendering changed original entry order")
	}
}

func TestCommitDiffUsesSummaryOrder(t *testing.T) {
	older := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	pr := model.PullRequest{Repo: "org/repo", Commits: []model.PRCommit{
		{OID: "older", Date: older},
		{OID: "newer", Date: newer},
	}}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	provider := &commentRecordingGitHub{}
	m.app = app.New(provider, nil, nil)
	m.diffEnabled = true
	for index, want := range []string{"newer", "older"} {
		m.commitIndex = index
		cmd := m.autoCommitDiffCmd()
		if cmd == nil || m.commitDiffKey != want {
			t.Errorf("commit index %d loads %q, want %q", index, m.commitDiffKey, want)
		}
		if cmd != nil {
			cmd()
		}
	}
	if !reflect.DeepEqual(provider.commitDiffSHAs, []string{"newer", "older"}) {
		t.Errorf("requested commit diffs = %v", provider.commitDiffSHAs)
	}
}

func TestSummaryTimestampCalendarBoundaries(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	original := time.Local
	time.Local = location
	t.Cleanup(func() { time.Local = original })
	for _, tc := range []struct {
		name, date, now, want string
	}{
		{"local today across UTC midnight", "2026-10-08T01:35:00Z", "2026-10-07T23:00:00-04:00", "Today 21:35"},
		{"yesterday across year boundary", "2025-12-31T23:59:00-05:00", "2026-01-01T00:01:00-05:00", "Yesterday 23:59"},
		{"yesterday during DST change", "2026-03-07T23:30:00-05:00", "2026-03-08T23:45:00-04:00", "Yesterday 23:30"},
		{"older date", "2026-10-05T13:10:00Z", "2026-10-07T12:00:00Z", "2026-10-05 09:10"},
		{"future date", "2026-10-08T13:10:00Z", "2026-10-07T12:00:00Z", "2026-10-08 09:10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			date, err := time.Parse(time.RFC3339, tc.date)
			if err != nil {
				t.Fatal(err)
			}
			now, err := time.Parse(time.RFC3339, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			if got := summaryTimestamp(date, now); got != tc.want {
				t.Errorf("summaryTimestamp = %q, want %q", got, tc.want)
			}
		})
	}
	if got := summaryTimestamp(time.Time{}, time.Now()); got != "" {
		t.Errorf("zero timestamp = %q, want empty", got)
	}
}

func TestSummaryOrderingIsStable(t *testing.T) {
	date := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	commits := []model.PRCommit{{OID: "first", Date: date}, {OID: "second", Date: date}}
	comments := []model.PRComment{{Body: "first", CreatedAt: date}, {Body: "second", CreatedAt: date}}
	if got := newestCommits(commits); !reflect.DeepEqual(got, commits) {
		t.Errorf("equal-date commits reordered: %v", got)
	}
	if got := newestComments(comments); !reflect.DeepEqual(got, comments) {
		t.Errorf("equal-date comments reordered: %v", got)
	}
}

func TestSummaryTimestampMidnightDST(t *testing.T) {
	location, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatal(err)
	}
	original := time.Local
	time.Local = location
	t.Cleanup(func() { time.Local = original })
	now := time.Date(2026, 9, 7, 0, 30, 0, 0, location)
	for _, tc := range []struct {
		day  int
		want string
	}{
		{6, "Yesterday 12:00"},
		{5, "2026-09-05 12:00"},
	} {
		date := time.Date(2026, 9, tc.day, 12, 0, 0, 0, location)
		if got := summaryTimestamp(date, now); got != tc.want {
			t.Errorf("day %d timestamp = %q, want %q", tc.day, got, tc.want)
		}
	}
}
