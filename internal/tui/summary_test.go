package tui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func TestSelectedCommitUsesFileListStyle(t *testing.T) {
	pr := model.PullRequest{Commits: []model.PRCommit{
		{OID: "first", Headline: "selected headline", Author: "selected author", Date: time.Now()},
		{OID: "second", Headline: "other headline", Author: "other author"},
	}}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	for _, index := range []int{0, 1} {
		m.commitIndex = index
		var b strings.Builder
		m.writeCommitSummary(&b, pr, 80)
		rows := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
		for i, commit := range pr.Commits {
			style := m.theme.Styles.Muted
			if i == index {
				style = m.theme.Styles.TitleStrong
			}
			meta := commit.Author
			if !commit.Date.IsZero() {
				meta = summaryTimestamp(commit.Date, time.Now()) + " · " + meta
			}
			if rows[i*2] != style.Render(meta) {
				t.Errorf("commit %d metadata does not use expected selection style: %q", i, rows[i*2])
			}
			if rows[i*2+1] != m.theme.Styles.File.Render(commit.OID)+" "+style.Render(commit.Headline) {
				t.Errorf("commit %d row does not use expected selection style: %q", i, rows[i*2+1])
			}
		}
	}
}

func TestPRSelectionLoadsCommitDiff(t *testing.T) {
	for _, detailsLoaded := range []bool{true, false} {
		t.Run(fmt.Sprintf("detailsLoaded=%t", detailsLoaded), func(t *testing.T) {
			selected := model.PullRequest{Repo: "org/repo", Number: 2, DetailsLoaded: detailsLoaded, ReviewCommentsLoaded: detailsLoaded}
			commits := []model.PRCommit{{OID: "newest", Date: time.Now()}, {OID: "older"}}
			if detailsLoaded {
				selected.Commits = commits
			}
			m := modelWithPRs([]model.PullRequest{{Repo: "org/repo", Number: 1, DetailsLoaded: true, ReviewCommentsLoaded: true}, selected}, 0)
			provider := &commentRecordingGitHub{}
			m.app = app.New(provider, cache.NewMemoryStore())
			m.diffEnabled = true
			m.summaryMode = SummaryModeCommits
			m.focus = FocusInbox
			m.commitDiffKey = "previous"
			m.renderedCommitDiff = "previous diff"
			next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			m = next.(Model)
			if cmd != nil {
				runCmd(cmd)
			}
			if !detailsLoaded {
				if len(provider.commitDiffSHAs) != 0 {
					t.Fatal("requested commit diff before details arrived")
				}
				selected.DetailsLoaded = true
				selected.ReviewCommentsLoaded = true
				selected.Commits = commits
				next, cmd = m.Update(refreshPRMsg{pr: selected})
				m = next.(Model)
				if cmd != nil {
					runCmd(cmd)
				}
			}
			if !reflect.DeepEqual(provider.commitDiffSHAs, []string{"newest"}) {
				t.Fatalf("requested commit diffs = %v, want [newest]", provider.commitDiffSHAs)
			}
			next, _ = m.Update(commitDiffMsg{sha: "newest", rendered: "loaded commit diff"})
			m = next.(Model)
			if m.renderedCommitDiff != "loaded commit diff" || strings.Contains(stripANSI(m.diffPanel(80, 20)), "loading commit diff") {
				t.Fatal("selected commit diff still loading after response")
			}
			if cmd := m.selectionCmd(); cmd != nil {
				runCmd(cmd)
			}
			if len(provider.commitDiffSHAs) != 1 {
				t.Fatal("repeated selection requested the same commit again")
			}
		})
	}
}

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
		{Author: "older", Body: "older", CreatedAt: older},
		{Author: "yesterday", Body: "yesterday", CreatedAt: yesterday},
		{Author: "today", Body: "today", CreatedAt: today.UTC()},
		{Author: "undated", Body: "undated"},
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
			for _, label := range []string{"Today 14:35", "Yesterday 14:35", older.Format("Jan 2 15:04")} {
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
	m.app = app.New(provider, nil)
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
		{"older date", "2026-10-05T13:10:00Z", "2026-10-07T12:00:00Z", "Oct 5 09:10"},
		{"future date", "2026-10-08T13:10:00Z", "2026-10-07T12:00:00Z", "Oct 8 09:10"},
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

func TestCommentSummaryIncludesReviewAndInlineContext(t *testing.T) {
	var pr model.PullRequest
	data := `{"Comments":[{"Author":"bot","Body":"Review summary","Kind":"review","CreatedAt":"2026-10-14T12:40:53Z"},{"Author":"bot","Body":"Inline finding","Kind":"inline","Path":"roles.tf","Line":291,"OriginalLine":290,"Side":"RIGHT","CreatedAt":"2026-10-14T12:40:53Z"},{"Author":"alice","Body":"Old finding","Kind":"inline","Path":"old.tf","OriginalLine":10,"Side":"LEFT"},{"Author":"bob","Body":"File finding","Kind":"inline","Path":"file.tf"},{"Author":"carol","Body":"Deleted line finding","Kind":"inline","Path":"deleted.tf","Line":5,"Side":"LEFT"},{"Author":"dave","Body":"Conversation comment"}]}`
	if err := json.Unmarshal([]byte(data), &pr); err != nil {
		t.Fatal(err)
	}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	var details []string
	for _, comment := range pr.Comments {
		details = append(details, stripANSI(m.commentDetail(comment)))
	}
	view := strings.Join(details, "\n")
	for _, want := range []string{"bot · review", "bot · inline", "dave · comment", "roles.tf:291", "old.tf:10 (outdated) (left)", "file.tf", "deleted.tf:5 (left)", "Review summary", "Inline finding", "Conversation comment"} {
		if !strings.Contains(view, want) {
			t.Errorf("comment detail missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "dave · review") || strings.Contains(view, "roles.tf:290") || strings.Contains(view, "file.tf:0") {
		t.Fatalf("incorrect comment context: %q", view)
	}
	// The summary list shows only author and timestamp, within the pane width.
	var b strings.Builder
	m.writeCommentSummary(&b, pr, 12)
	list := stripANSI(b.String())
	if strings.Contains(list, "Review summary") || strings.Contains(list, "roles.tf") || strings.Contains(list, "review") {
		t.Fatalf("comment list shows content: %q", list)
	}
	for _, line := range strings.Split(b.String(), "\n") {
		if lipgloss.Width(line) > 12 {
			t.Fatalf("comment line exceeds pane width: %q", line)
		}
	}
}

func TestEmptyCommentSummary(t *testing.T) {
	m := minimalModel()
	var b strings.Builder
	m.writeCommentSummary(&b, model.PullRequest{}, 80)
	if got := stripANSI(b.String()); got != "no comments\n" {
		t.Fatalf("empty comment summary = %q, want no comments", got)
	}
}

func TestSelectedPRReloadsLegacyCommentDetails(t *testing.T) {
	pr := model.PullRequest{Repo: "org/repo", Number: 1, DetailsLoaded: true}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	m.app = app.New(&commentRecordingGitHub{}, cache.NewMemoryStore())
	if cmd := m.autoDetailsCmd(); cmd == nil {
		t.Fatal("old cached details without review comments must be fetched again")
	}
	if cmd := m.autoDetailsCmd(); cmd != nil {
		t.Fatal("legacy details must not cause duplicate requests")
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
		{5, "Sep 5 12:00"},
	} {
		date := time.Date(2026, 9, tc.day, 12, 0, 0, 0, location)
		if got := summaryTimestamp(date, now); got != tc.want {
			t.Errorf("day %d timestamp = %q, want %q", tc.day, got, tc.want)
		}
	}
}

func TestSelectedCommentRowUsesStrongStyle(t *testing.T) {
	pr := model.PullRequest{Comments: []model.PRComment{{Author: "first"}, {Author: "second"}}}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	m.commentIndex = 1
	var b strings.Builder
	m.writeCommentSummary(&b, pr, 80)
	rows := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	if rows[0] != truncateANSIWidth(m.theme.Styles.Muted.Render("first"), 80) || rows[1] != truncateANSIWidth(m.theme.Styles.TitleStrong.Render("second"), 80) {
		t.Fatalf("unexpected comment row styles: %q", rows)
	}
}
