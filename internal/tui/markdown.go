package tui

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"
)

// Raw HTML is tokenized for terminal display, never executed or sent to a browser.
// This retains expanded details content while hiding tags and bot marker comments.
var commentMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(goldmarkhtml.WithUnsafe()),
)

func commentPlainText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, ansi.Strip(s))
}

func (m Model) renderCommentMarkdown(body string) string {
	var source bytes.Buffer
	body = commentPlainText(body)
	if err := commentMarkdown.Convert([]byte(body), &source); err != nil {
		return body
	}
	z := html.NewTokenizer(&source)
	var out, code strings.Builder
	active := make(map[string]int)
	var lists []int // Zero is an unordered list; positive values are ordered counters.
	type listItem struct {
		start  int
		indent string
	}
	var items []listItem
	type link struct {
		href  string
		start int
	}
	var links []link
	pre, diff := false, false
	hidden := 0
	newline := func(count int) {
		text := out.String()
		if text == "" {
			return
		}
		trailing := len(text) - len(strings.TrimRight(text, "\n"))
		if trailing < count {
			out.WriteString(strings.Repeat("\n", count-trailing))
		}
	}
	writeContent := func(text string) {
		for i, line := range strings.Split(text, "\n") {
			if i > 0 {
				out.WriteByte('\n')
			}
			if line != "" && len(items) > 0 && strings.HasSuffix(out.String(), "\n") {
				out.WriteString(items[len(items)-1].indent)
			}
			out.WriteString(line)
		}
	}
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			break
		}
		token := z.Token()
		if kind == html.CommentToken || kind == html.DoctypeToken {
			continue
		}
		if kind == html.StartTagToken && (token.Data == "script" || token.Data == "style") {
			hidden++
			continue
		}
		if hidden > 0 {
			if kind == html.EndTagToken && (token.Data == "script" || token.Data == "style") {
				hidden--
			}
			continue
		}
		if kind == html.TextToken {
			text := commentPlainText(token.Data)
			if pre {
				code.WriteString(text)
				continue
			}
			// Renderer-generated newlines between HTML blocks are not body text.
			if strings.TrimSpace(text) == "" && strings.Contains(text, "\n") {
				continue
			}
			style := m.theme.Styles.App
			if active["strong"] > 0 || active["b"] > 0 || active["heading"] > 0 || active["summary"] > 0 {
				style = style.Bold(true)
			}
			if active["em"] > 0 || active["i"] > 0 {
				style = style.Italic(true)
			}
			if active["del"] > 0 {
				style = style.Strikethrough(true)
			}
			if !m.theme.NoColor && (active["code"] > 0 || active["heading"] > 0) {
				style = style.Foreground(m.theme.Palette.Focus)
			}
			if active["a"] > 0 {
				style = style.Underline(true)
			}
			if active["code"] == 0 {
				text = emojiShortcodePattern.ReplaceAllString(text, "")
			}
			writeContent(style.Render(text))
			continue
		}
		tag := token.Data
		if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
			tag = "heading"
		}
		attrs := make(map[string]string)
		for _, attr := range token.Attr {
			attrs[attr.Key] = commentPlainText(attr.Val)
		}
		if kind == html.StartTagToken || kind == html.SelfClosingTagToken {
			active[tag]++
			switch tag {
			case "p":
				if len(items) == 0 || out.Len() != items[len(items)-1].start {
					newline(2)
				}
			case "heading", "summary", "pre", "details", "blockquote":
				newline(2)
			case "br":
				out.WriteByte('\n')
			case "hr":
				newline(2)
				out.WriteString(m.theme.Styles.Muted.Render("───"))
				newline(2)
			case "ul":
				lists = append(lists, 0)
			case "ol":
				start := 1
				if _, err := fmt.Sscanf(attrs["start"], "%d", &start); err != nil || start < 1 {
					start = 1
				}
				lists = append(lists, start)
			case "li":
				newline(1)
				padding := strings.Repeat("  ", max(0, len(lists)-1))
				marker := "• "
				if len(lists) > 0 && lists[len(lists)-1] > 0 {
					marker = fmt.Sprintf("%d. ", lists[len(lists)-1])
					lists[len(lists)-1]++
				}
				out.WriteString(padding + marker)
				items = append(items, listItem{start: out.Len(), indent: padding + strings.Repeat(" ", ansi.StringWidth(marker))})
			case "input":
				if attrs["type"] == "checkbox" {
					if _, checked := attrs["checked"]; checked {
						out.WriteString("[x] ")
					} else {
						out.WriteString("[ ] ")
					}
				}
			case "a":
				links = append(links, link{href: attrs["href"], start: out.Len()})
			case "img":
				out.WriteString(attrs["alt"])
			case "code":
				if pre {
					diff = attrs["class"] == "language-diff"
				}
			case "tr":
				newline(1)
			case "td", "th":
				out.WriteString(" | ")
			}
			if tag == "pre" {
				pre = true
				code.Reset()
			}
		} else if kind == html.EndTagToken {
			active[tag] = max(0, active[tag]-1)
			switch tag {
			case "p", "heading", "summary", "details", "blockquote":
				newline(2)
			case "ul", "ol":
				if len(lists) > 0 {
					lists = lists[:len(lists)-1]
				}
				newline(1)
			case "li":
				if len(items) > 0 {
					items = items[:len(items)-1]
				}
			case "a":
				if len(links) > 0 {
					l := links[len(links)-1]
					links = links[:len(links)-1]
					if l.href != "" && stripANSI(out.String()[l.start:]) != l.href {
						out.WriteString(m.theme.Styles.Muted.Render(" (" + l.href + ")"))
					}
				}
			case "pre":
				for _, line := range strings.Split(strings.TrimSuffix(code.String(), "\n"), "\n") {
					style := m.theme.Styles.App
					if diff && strings.HasPrefix(line, "+") {
						style = m.theme.Styles.Add
					} else if diff && strings.HasPrefix(line, "-") {
						style = m.theme.Styles.Delete
					}
					writeContent(style.Render(line) + "\n")
				}
				pre, diff = false, false
				newline(2)
			}
		}
	}
	return strings.Trim(out.String(), "\n")
}
