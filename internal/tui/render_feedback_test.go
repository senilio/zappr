package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

const badgeReviewBody = "<!-- review-summary -->\n## Code Review\n\n" +
	"| Priority | Location | Finding |\n|---|---|---|\n" +
	"| <sub><sub>![P2 Badge](https://img.shields.io/badge/P2-yellow?style=flat)</sub></sub> | main.tf:35 | Bucket name |\n\n" +
	"---\nModel: example - total time: 00:01:12\n"

func TestMarkdownKeepsClockTimesAndTablesStartCleanly(t *testing.T) {
	m := minimalModel()
	view := stripANSI(m.renderCommentMarkdown(badgeReviewBody))
	if !strings.Contains(view, "00:01:12") {
		t.Errorf("clock time mangled: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			t.Errorf("table row starts with a separator: %q", line)
		}
	}
	if !strings.Contains(view, "Priority | Location | Finding") {
		t.Errorf("table cells not separated: %q", view)
	}
	if got := stripANSI(m.renderCommentMarkdown("Ship it :rocket: now")); strings.Contains(got, ":rocket:") {
		t.Errorf("emoji shortcode kept: %q", got)
	}
}

func TestReaderProseWrapsAtWordBoundaries(t *testing.T) {
	m := commentPaneModel(t)
	m.cfg.Wrap.Diff = true
	for i := range m.prs[0].Comments {
		m.prs[0].Comments[i].Body = strings.Repeat("alpha beta gamma delta ", 12)
	}
	m.commentState.selectedKey = ""
	m.reconcileComments()
	content := m.currentDiffContent()
	if !strings.Contains(stripANSI(content), "alpha beta") {
		t.Fatalf("selected comment body missing: %q", stripANSI(content))
	}
	layout, _ := m.buildDiffLayout(content, 30)
	words := map[string]bool{"alpha": true, "beta": true, "gamma": true, "delta": true}
	for _, row := range strings.Split(stripANSI(layout), "\n") {
		if !strings.Contains(row, "alpha") && !strings.Contains(row, "gamma") && !strings.Contains(row, "beta") {
			continue
		}
		for _, word := range strings.Fields(row) {
			if !words[word] {
				t.Fatalf("word split across rows: %q in %q", word, row)
			}
		}
	}
}

func TestPlanSourceRendersMarkdown(t *testing.T) {
	m := planModel(t)
	m = pressKey(m, "4")
	m = pressKey(m, "G")
	view := stripANSI(m.currentDiffContent())
	for _, unwanted := range []string{"<details>", "<summary>", "```diff", "</details>"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("source view shows raw markup %q", unwanted)
		}
	}
	if !strings.Contains(view, "Ran Plan for project:") || !strings.Contains(view, "Show Output") {
		t.Errorf("source view lost content: %q", view)
	}
}

func TestPlanTextColorsChanges(t *testing.T) {
	m := planModel(t)
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	m = pressSpecial(pressSpecial(pressKey(m, "4"), tea.KeyDown), tea.KeyDown)
	content := m.currentDiffContent()
	if !strings.Contains(content, "\x1b[") {
		t.Fatal("plan resource text has no styling")
	}
	for _, line := range strings.Split(content, "\n") {
		plain := stripANSI(line)
		if strings.HasPrefix(strings.TrimSpace(plain), "+ ") && line == plain {
			t.Fatalf("added line is unstyled: %q", plain)
		}
	}
}

func TestPlanContextWordingAndSparseHeader(t *testing.T) {
	m := planModel(t)
	m = pressKey(m, "4")
	report, _ := m.selectedPlanReport()
	content := stripANSI(m.currentDiffContent())
	title := stripANSI(m.readerTitle())
	for _, text := range []string{content, title} {
		for _, bad := range []string{"1 source comments", "1 projects", "· projects ·"} {
			if strings.Contains(text, bad) {
				t.Errorf("careless wording %q in %q", bad, text)
			}
		}
	}
	if title != "Plan" || !strings.HasPrefix(content, "source complete") && !strings.HasPrefix(content, "source incomplete") {
		t.Errorf("context should appear in content, not title: title=%q content=%q", title, content)
	}
	for _, node := range m.planNodes() {
		label := m.planNodeLabel(node)
		if strings.HasPrefix(label, "Source comment:") {
			t.Errorf("source row shows an internal key: %q", label)
		}
	}
	_ = report
}

func TestPaneTitleShortensInsteadOfDisappearing(t *testing.T) {
	m := planModel(t)
	title := "Plan · changes · commit unverified · " + strings.Repeat("very-long-resource-address.", 10)
	box := m.pane(title, "body", 60, 5, FocusDiff)
	top := stripANSI(strings.SplitN(box, "\n", 2)[0])
	if !strings.Contains(top, "Plan · changes") {
		t.Fatalf("long title disappeared: %q", top)
	}
}

func TestPlanUnavailableMessageClearsOnPRChange(t *testing.T) {
	m := planModel(t)
	other := m.prs[0]
	other.Number, other.Comments = 2, nil
	other.ConversationCommentsLoaded = true
	m.prs = append(m.prs, other)
	m.allPRs = append([]model.PullRequest(nil), m.prs...)
	m.selected = 1
	m.resetPRViewState()
	m = pressKey(m, "4")
	if m.message != "no supported plan report found" {
		t.Fatalf("precondition message = %q", m.message)
	}
	m = pressSpecial(m, tea.KeyUp)
	if strings.Contains(m.message, "plan report") {
		t.Fatalf("stale message survived PR change: %q", m.message)
	}
	m.handlePRClosed(m.prs[m.selected].Repo, m.prs[m.selected].Number)
	if m.message != "PR closed" {
		t.Fatalf("close feedback lost: %q", m.message)
	}
}
