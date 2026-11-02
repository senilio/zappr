package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/model"
)

func TestPlanNeverShownForAnotherPRAfterClose(t *testing.T) {
	m := planModel(t)
	other := m.prs[0]
	other.Number, other.Comments, other.Title = 2, nil, "other"
	m.prs = append(m.prs, other)
	m.allPRs = append([]model.PullRequest(nil), m.prs...)
	m = pressKey(m, "4")
	if m.summaryMode != SummaryModePlan {
		t.Fatal("setup did not open Plan")
	}
	m.handlePRClosed(m.prs[0].Repo, 1)
	if pr, _ := m.current(); pr.Number != 2 {
		t.Fatalf("selected %d after close", pr.Number)
	}
	if m.planAvailable() || m.summaryMode == SummaryModePlan || strings.Contains(m.currentDiffContent(), "source complete") {
		t.Fatal("closed PR's report shown against the newly selected PR")
	}
}

func TestInboxHeightFocusedKeepsLargeShare(t *testing.T) {
	m := planModel(t)
	for _, mode := range []Mode{ModeInbox, ModeDetail} {
		m.focus, m.mode = FocusInbox, mode
		if inbox, summary := m.leftColumnHeights(40); inbox != 40*64/100 || inbox+summary != 40 {
			t.Fatalf("focused Inbox mode=%v heights=%d/%d, want inbox=%d", mode, inbox, summary, 40*64/100)
		}
	}
	m.focus, m.mode = FocusSummary, ModeInbox
	if inbox, _ := m.leftColumnHeights(40); inbox != 40*64/100 {
		t.Fatalf("Inbox mode with Summary focus = %d, want focused share", inbox)
	}
}

func TestInboxHeightUnfocusedFitsContentWithMinimum(t *testing.T) {
	m := planModel(t)
	m.mode = ModeDetail
	for _, tc := range []struct{ count, height, want int }{
		{1, 40, inboxMinContentRows + paneFrame},
		{12, 40, max(inboxMinContentRows, (40-2*paneFrame)/3) + paneFrame},
		{1, 10, 10 - paneFrame - 1},
	} {
		m.prs = []model.PullRequest{m.prs[0]}
		for len(m.prs) < tc.count {
			pr := m.prs[0]
			pr.Number += len(m.prs)
			m.prs = append(m.prs, pr)
		}
		for _, focus := range []Focus{FocusSummary, FocusDiff} {
			m.focus = focus
			inbox, summary := m.leftColumnHeights(tc.height)
			if inbox != tc.want || inbox+summary != tc.height || summary <= paneFrame {
				t.Fatalf("count=%d height=%d focus=%v heights=%d/%d, want inbox=%d", tc.count, tc.height, focus, inbox, summary, tc.want)
			}
		}
	}
}

func TestInboxHeightPreservesOversizedWrappedItems(t *testing.T) {
	m := planModel(t)
	m.cfg.Wrap.Inbox = true
	m.focus, m.mode = FocusSummary, ModeDetail
	m.prs[0].Title = "START " + strings.Repeat("a long wrapped title ", 100)
	m.resizeViewports()
	m.syncInboxVP()
	w, _, _ := m.paneInnerWidths()
	_, spans := m.inboxContent(w)
	if spans[0].end <= m.inboxVP.Height() || m.inboxVP.YOffset() != 0 || !strings.Contains(stripANSI(m.inboxVP.View()), "START") {
		t.Fatal("oversized selected item did not start at its title")
	}
	m.inboxManualScroll = true
	m.inboxVP.SetYOffset(3)
	m.syncInboxVP()
	if m.inboxVP.YOffset() != 3 {
		t.Fatal("content-aware height disabled manual scrolling")
	}
	if m.inboxVP.Height() > max(inboxMinContentRows, (m.height-chromeLines-2*paneFrame)/3) {
		t.Fatal("wrapped title consumed the entire left column")
	}
}

func TestQuietFileRowsKeepMeaningfulStatus(t *testing.T) {
	m := planModel(t)
	pr := model.PullRequest{Files: []model.ChangedFile{
		{Path: "src/main.go", Status: "modified", Additions: 2},
		{Path: "src/new.go", Status: "added", Additions: 3},
		{Path: "src/old.go", Status: "removed", Deletions: 5},
		{Path: "src/moved.go", Status: "renamed"},
	}}
	var b strings.Builder
	m.writeFileSummary(&b, pr, 60)
	got := stripANSI(b.String())
	for _, absent := range []string{"modified", "+0", "-0"} {
		if strings.Contains(got, absent) {
			t.Fatalf("file summary retained %q: %s", absent, got)
		}
	}
	for _, present := range []string{"src/", "main.go", "+2", "added", "+3", "removed", "-5", "renamed"} {
		if !strings.Contains(got, present) {
			t.Fatalf("file summary lost %q: %s", present, got)
		}
	}
}

func TestReaderUntrustedUnicodeAndControlsRemainBounded(t *testing.T) {
	for _, width := range []int{65, 95, 130} {
		m := planModel(t)
		m.width = width
		unsafe := "海\x1b]8;;https://invalid.example/\x1b\\text\x1b]8;;\x1b\\\a\x00"
		m.prs[0].Title = unsafe + strings.Repeat("界", 100)
		m.prs[0].Author = unsafe
		m.prs[0].Files = []model.ChangedFile{{Path: "src/" + unsafe + ".go", Status: "modified", Additions: 1}}
		m.prs[0].Commits = []model.PRCommit{{OID: "abcdef123", Author: unsafe, Headline: unsafe}}
		m.prs[0].Comments[0].Body = strings.Replace(m.prs[0].Comments[0].Body, `+ name = "alpha"`, `+ name = "`+unsafe+`"`, 1)
		original := m.prs[0].Comments[0].Body
		m.reconcilePlan()
		m.resizeViewports()
		m = pressKey(m, "4")
		m = pressSpecial(pressSpecial(m, tea.KeyDown), tea.KeyDown)
		for _, key := range []string{"", "G", "g", "3", "2", "1"} {
			if key != "" {
				m = pressKey(m, key)
			}
			view := m.View().Content
			if strings.Contains(view, "\x1b]") || strings.ContainsAny(view, "\a\x00") {
				t.Fatalf("width=%d key=%q leaked source terminal controls", width, key)
			}
			for _, row := range strings.Split(view, "\n") {
				if ansi.StringWidth(row) > width {
					t.Fatalf("width=%d key=%q exceeded terminal width", width, key)
				}
			}
		}
		if m.prs[0].Comments[0].Body != original {
			t.Fatal("display sanitization changed cached original source")
		}
	}
}

func TestReaderHelpAndFeedbackWithoutShortcutChrome(t *testing.T) {
	m := planModel(t)
	m.message = "no matches"
	header := stripANSI(m.headerView())
	if !strings.Contains(header, "no matches") || !strings.Contains(header, "? help") {
		t.Fatalf("header hides feedback or existing help hint: %q", header)
	}
	help := stripANSI(m.helpPanel())
	for _, label := range []string{"Plan reader and comments", "next resource", "changed project", "full title", "author filter"} {
		if strings.Contains(help, label) {
			t.Fatalf("help advertises removed function %q", label)
		}
	}
	if strings.Contains(help, "Atlantis plan (Atlantis repos)") {
		t.Fatal("help still advertises removed TF badge")
	}
}

func TestReaderTabsFitActualSummaryPane(t *testing.T) {
	for _, width := range []int{65, 95, 130} {
		m := planModel(t)
		m.width = width
		m = pressKey(m, "4")
		_, summaryW, _ := m.paneInnerWidths()
		title := stripANSI(m.summaryTitle())
		if ansi.StringWidth(title) > summaryW || !strings.Contains(title, "4 plan") {
			t.Fatalf("width=%d title hides selected Plan tab: %q (budget %d)", width, title, summaryW)
		}
		m = pressKey(m, "3")
		title = stripANSI(m.summaryTitle())
		if ansi.StringWidth(title) > summaryW || !strings.Contains(title, "comments") || !strings.Contains(title, "1/1") {
			t.Fatalf("width=%d title loses context or exceeds pane: %q", width, title)
		}
	}
}
