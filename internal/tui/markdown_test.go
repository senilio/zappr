package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func TestCommentSummaryRendersMarkdown(t *testing.T) {
	body := "# Plan\n\nTo **apply** this _plan_, use `make apply`.\n\n" +
		"- First item\n- [Workflow](https://example.com/run)\n\n" +
		"<details><summary>Show Output</summary>\n\n```diff\n+ added\n- removed\n unchanged\n```\n</details>\n\n" +
		"<!-- hidden bot marker -->\n\n```shell\nmake plan\n```"
	pr := model.PullRequest{Comments: []model.PRComment{{Author: "bot", Body: body}}}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	var b strings.Builder
	m.writeCommentSummary(&b, pr, 100)
	view := stripANSI(b.String())
	for _, want := range []string{"Plan", "To apply this plan, use make apply.", "First item", "Workflow", "https://example.com/run", "Show Output", "+ added", "- removed", " unchanged", "make plan"} {
		if !strings.Contains(view, want) {
			t.Errorf("rendered comment missing %q: %q", want, view)
		}
	}
	for _, unwanted := range []string{"# Plan", "**", "_plan_", "`", "[Workflow]", "<details>", "<summary>", "</details>", "<!--", "hidden bot marker"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("rendered comment contains source markup %q: %q", unwanted, view)
		}
	}
	if pr.Comments[0].Body != body {
		t.Fatal("rendering changed the original comment")
	}
}

func TestCommentMarkdownStyles(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := minimalModel()
	m.theme = uitheme.New(uitheme.DarkDefault(), termenv.TrueColor, false)
	view := m.renderCommentMarkdown("**bold** _italic_ [link](https://example.com)\n\n```diff\n+ added\n- removed\n```")
	for _, want := range []string{
		m.theme.Styles.App.Bold(true).Render("bold"),
		m.theme.Styles.App.Italic(true).Render("italic"),
		m.theme.Styles.App.Underline(true).Render("link"),
		m.theme.Styles.Add.Render("+ added"),
		m.theme.Styles.Delete.Render("- removed"),
	} {
		if !strings.Contains(view, want) {
			t.Errorf("comment missing styled text %q: %q", want, view)
		}
	}
	m.theme = uitheme.New(uitheme.DarkDefault(), termenv.TrueColor, true)
	view = m.renderCommentMarkdown("# Heading\n\n`code`")
	if strings.Contains(view, "38;") || strings.Contains(view, "48;") {
		t.Errorf("no-color comment contains color escapes: %q", view)
	}
}

func TestCommentMarkdownDiscardsHiddenHTMLAndTerminalControls(t *testing.T) {
	m := minimalModel()
	body := "Visible &amp; readable\n\n<script>secret</script>\n<style>hidden</style>\n\n" +
		"<!-- multiline\nhidden marker -->\n\nAfter\x1b[2J controls &#27;[2J\n\n" +
		"[safe](https://example.com)\x1b]52;c;ZXZpbA==\x07"
	view := stripANSI(m.renderCommentMarkdown(body))
	for _, want := range []string{"Visible & readable", "After", "safe", "https://example.com"} {
		if !strings.Contains(view, want) {
			t.Errorf("comment missing %q: %q", want, view)
		}
	}
	for _, unwanted := range []string{"secret", "hidden", "\x1b", "\x07", "52;c;"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("comment leaked hidden or terminal content %q: %q", unwanted, view)
		}
	}
}

func TestCommentMarkdownLooseAndNestedLists(t *testing.T) {
	m := minimalModel()
	for _, tc := range []struct {
		body string
		want []string
	}{
		{"- First\n\n- Second", []string{"• First", "• Second"}},
		{"1. Apply\n\n   More context\n\n   ```shell\n   make apply\n   ```\n\n2. Next", []string{"1. Apply", "   More context", "   make apply", "2. Next"}},
		{"- Parent\n  - Nested\n\n    More detail\n- Next", []string{"• Parent", "  • Nested", "    More detail", "• Next"}},
	} {
		view := stripANSI(m.renderCommentMarkdown(tc.body))
		lines := strings.Split(view, "\n")
		for _, want := range tc.want {
			found := false
			for _, line := range lines {
				if line == want {
					found = true
				}
			}
			if !found {
				t.Errorf("list missing line %q: %q", want, view)
			}
		}
	}
}

func TestCommentMarkdownKeepsCodeLiteralAndTruncates(t *testing.T) {
	code := "<details> **literal** `code` " + strings.Repeat("x", 100)
	body := "```text\n" + code + "\n```"
	pr := model.PullRequest{Comments: []model.PRComment{{Author: "bot", Body: body}}}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	var b strings.Builder
	m.writeCommentSummary(&b, pr, 40)
	view := stripANSI(b.String())
	if !strings.Contains(view, "<details> **literal** `code`") {
		t.Fatalf("code contents were interpreted as markup: %q", view)
	}
	if strings.Contains(view, "```text") {
		t.Fatalf("code fence leaked: %q", view)
	}
	codeLines := 0
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 40 {
			t.Fatalf("rendered line exceeds pane width: %q", line)
		}
		if strings.Contains(line, "xxx") {
			codeLines++
		}
	}
	if codeLines != 1 {
		t.Fatalf("long code wrapped into %d lines", codeLines)
	}
}
