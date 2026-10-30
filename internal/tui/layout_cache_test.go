package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/config"
	"github.com/senilio/zappr/internal/model"
)

func scrollModel(t testing.TB, count int) Model {
	t.Helper()
	m, err := New(nil, "org/repo", time.Hour, "", config.Default())
	if err != nil {
		t.Fatal(err)
	}
	m.loading = false
	m.width, m.height = 160, 48
	for i := 0; i < count; i++ {
		m.prs = append(m.prs, model.PullRequest{Repo: "org/repo", Number: i + 1, Title: fmt.Sprintf("Pull request %d with a reasonably long title", i+1), Author: "author", CIStatus: "success", ReviewDecision: "APPROVED"})
	}
	m.allPRs = m.prs
	m.resizeViewports()
	return m
}

func TestPaneLayoutsReuseRows(t *testing.T) {
	m := scrollModel(t, 100)
	_, inbox := m.inboxContent(30)
	_, again := m.inboxContent(30)
	if &inbox[0] != &again[0] {
		t.Error("unchanged Inbox rebuilt its row spans")
	}
	m.prs[0].Files = []model.ChangedFile{{Path: "main.go"}, {Path: "test.go"}}
	_, summary := m.summaryLayout(m.prs[0], 30)
	_, again = m.summaryLayout(m.prs[0], 30)
	if &summary[0] != &again[0] {
		t.Error("unchanged Summary rebuilt its row spans")
	}
	m.diff = strings.Repeat("+a long diff line to wrap into multiple visual rows\n", 100)
	_, diff := m.diffLayout(30)
	_, same := m.diffLayout(30)
	if &diff[0] != &same[0] {
		t.Error("unchanged Diff rebuilt its source rows")
	}
}

func TestInboxLayoutTracksInPlaceChanges(t *testing.T) {
	m := scrollModel(t, 2)
	before, _ := m.inboxContent(50)
	m.prs[0].Title = "updated title"
	m.prs[0].Draft, m.prs[0].AutoMerge = true, true
	m.prs[0].Author = "someone else"
	after, _ := m.inboxContent(50)
	if after == before || !strings.Contains(stripANSI(after), "updated title") || !strings.Contains(stripANSI(after), "draft") {
		t.Fatalf("stale Inbox after in-place update: %q", after)
	}
}

func TestSummaryLayoutTracksInPlaceChanges(t *testing.T) {
	for _, mode := range []SummaryMode{SummaryModeFiles, SummaryModeCommits, SummaryModeComments} {
		t.Run(mode.String(), func(t *testing.T) {
			m := scrollModel(t, 1)
			m.summaryMode = mode
			m.prs[0].Files = []model.ChangedFile{{Path: "old.go"}}
			m.prs[0].Commits = []model.PRCommit{{Headline: "old headline"}}
			m.prs[0].Comments = []model.PRComment{{Author: "old author"}}
			before, _ := m.summaryLayout(m.prs[0], 60)
			m.prs[0].Files[0].Path = "new.go"
			m.prs[0].Commits[0].Headline = "new headline"
			m.prs[0].Comments[0].Author = "new author"
			after, _ := m.summaryLayout(m.prs[0], 60)
			if after == before || !strings.Contains(stripANSI(after), "new") {
				t.Fatalf("stale Summary after in-place update: %q", after)
			}
		})
	}
}

func TestFullFileRenderingReused(t *testing.T) {
	m := scrollModel(t, 1)
	m.diffMode = DiffModeFile
	m.prs[0].Files = []model.ChangedFile{{Path: "main.go"}}
	m.fileContentKey = m.selectedFileContentKey()
	m.fileContent = strings.Repeat("some source code\n", 100)
	m.currentFileContent()
	if allocs := testing.AllocsPerRun(10, func() { m.currentFileContent() }); allocs > 5 {
		t.Fatalf("unchanged full-file view allocated %g times", allocs)
	}
	m.fileContent = "replacement content"
	if got := stripANSI(m.currentFileContent()); !strings.Contains(got, "replacement content") {
		t.Fatalf("stale full-file rendering: %q", got)
	}
}

func TestWheelUpdateSkipsUnrelatedDiffLayout(t *testing.T) {
	m := scrollModel(t, 100)
	m.diffEnabled = true
	m.diff = strings.Repeat("+large diff line\n", 1000)
	m.diffLayoutCache = nil
	m.inboxManualScroll = true
	small := m
	small.diff = ""
	mouse := tea.MouseWheelMsg{Button: tea.MouseWheelDown}
	baseline := testing.AllocsPerRun(5, func() { small.Update(mouse) })
	large := testing.AllocsPerRun(5, func() { m.Update(mouse) })
	if large > baseline+5 {
		t.Fatalf("Inbox wheel allocations grew with unrelated diff: %g vs %g", large, baseline)
	}
}

func TestPaneLayoutInvalidation(t *testing.T) {
	for _, change := range []string{"selection", "width", "wrap", "theme"} {
		t.Run(change, func(t *testing.T) {
			m := scrollModel(t, 2)
			m.prs[0].Files = []model.ChangedFile{{Path: "main.go"}}
			m.diff = "+a long line that can wrap into several rows"
			width := 30
			inbox := m.inboxPane(width)
			summary := m.summaryPane(m.prs[0], width)
			diff := m.diffPane(width)
			switch change {
			case "selection":
				m.selected, m.fileIndex = 1, 1
			case "width":
				width = 15
			case "wrap":
				m.cfg.Wrap.Inbox, m.cfg.Wrap.Summary, m.cfg.Wrap.Diff = true, true, true
			case "theme":
				m.theme.NoColor = !m.theme.NoColor
			}
			if got := m.inboxPane(width); got == inbox {
				t.Error("Inbox reused a stale layout")
			}
			if got := m.summaryPane(m.prs[0], width); got == summary {
				t.Error("Summary reused a stale layout")
			}
			// Selection and theme do not affect an already-rendered diff string.
			if change == "width" || change == "wrap" {
				if got := m.diffPane(width); got == diff {
					t.Error("Diff reused a stale layout")
				}
			}
			m.diff = "+replacement diff"
			if got := m.diffPane(width); !strings.Contains(got.content, "replacement") {
				t.Error("Diff content change was ignored")
			}
		})
	}
}

func TestSummaryLayoutRefreshesRelativeTime(t *testing.T) {
	m := scrollModel(t, 1)
	m.summaryMode = SummaryModeCommits
	m.prs[0].Commits = []model.PRCommit{{Headline: "headline", Date: time.Now().Add(-time.Minute)}}
	m.summaryPane(m.prs[0], 40)
	previous := m.summaryLayoutCache
	previous.key.second = 0 // Simulate a layout from an earlier clock tick.
	m.summaryPane(m.prs[0], 40)
	if m.summaryLayoutCache == previous {
		t.Fatal("relative timestamps never refresh")
	}
}

func TestDiffPaddingDoesNotModifyCachedRows(t *testing.T) {
	m := scrollModel(t, 1)
	m.diff = "one\ntwo\nthree\nfour"
	layout := m.diffPane(20)
	m.diffVP.SetContent(m.diff)
	m.diffVP.SetHeight(2)
	m.diffVP.SetYOffset(2)
	if got := m.diffVP.YOffset(); got != 2 {
		t.Fatalf("diff offset = %d, want 2", got)
	}
	m.renderDiffViewport(20, 8)
	if got := strings.Join(layout.lines, "\n"); got != m.diff {
		t.Fatalf("padding modified cached rows: %q", got)
	}
}

func TestDiffWidthCalculationReused(t *testing.T) {
	m := scrollModel(t, 1)
	m.diffEnabled = true
	m.diff = strings.Repeat(strings.Repeat("wide line", 40)+"\n", 100)
	m.syncDiffVP()
	if allocs := testing.AllocsPerRun(10, func() { m.maxDiffXOffset() }); allocs != 0 {
		t.Fatalf("unchanged diff width rescanned content: %g allocations", allocs)
	}
	if got, want := m.maxDiffXOffset(), len(strings.Repeat("wide line", 40))-m.diffVP.Width(); got != want {
		t.Fatalf("max horizontal offset = %d, want %d", got, want)
	}
}

func BenchmarkWheelScrolling(b *testing.B) {
	for _, count := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("PRs%d", count), func(b *testing.B) {
			m := scrollModel(b, count)
			m.inboxManualScroll = true
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				button := tea.MouseWheelDown
				if i%2 != 0 {
					button = tea.MouseWheelUp
				}
				next, _ := m.Update(tea.MouseWheelMsg{Button: button})
				m = next.(Model)
				m.View()
			}
		})
	}
}

func TestInboxLayoutCacheInvalidatesOnAtlantisChange(t *testing.T) {
	m := scrollModel(t, 2)
	m.prs[0].Checks = atlantisChecks("PENDING", time.Time{})
	before, _ := m.inboxContent(80)
	m.prs[0].Checks = atlantisChecks("SUCCESS", time.Now().Add(-4*time.Minute))
	after, _ := m.inboxContent(80)
	if after == before || !strings.Contains(stripANSI(after), "PLAN OK") {
		t.Fatalf("stale Inbox after Atlantis change: %q", stripANSI(after))
	}
}

func TestInboxLayoutCacheInvalidatesOnReviewers(t *testing.T) {
	m := scrollModel(t, 2)
	before, _ := m.inboxContent(80)
	m.prs[0].Reviewers = []model.Reviewer{{Login: "zelda", State: "APPROVED"}}
	m.prs[0].ReviewersLoaded = true
	after, _ := m.inboxContent(80)
	if after == before || !strings.Contains(stripANSI(after), "zelda") {
		t.Fatalf("stale Inbox after reviewers change: %q", stripANSI(after))
	}
}
