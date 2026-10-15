package tui

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
)

func styleAtColumn(line string, column int) cellbuf.Style {
	pen := cellbuf.NewPenWriter(io.Discard)
	defer pen.Close()
	var state byte
	x := 0
	for len(line) > 0 {
		seq, width, n, next := ansi.DecodeSequence(line, state, nil)
		_, _ = pen.Write([]byte(seq))
		if width > 0 && x+width > column {
			return pen.Style()
		}
		x += width
		line, state = line[n:], next
	}
	return pen.Style()
}

func TestWrappedSummaryKeepsStylesOnEveryRow(t *testing.T) {
	style := "\x1b[1;38;2;100;120;140m"
	content := wrapStructured(style+strings.Repeat("wrapped word ", 10)+"\x1b[0m", 18)
	rows := strings.Split(fitPaneContentWidth(content, 18), "\n")
	want := styleAtColumn(style+"X", 0)
	if len(rows) < 3 {
		t.Fatal("fixture did not wrap")
	}
	for i, row := range rows {
		got := styleAtColumn(row, 0)
		if !got.Equal(&want) {
			t.Errorf("summary row %d lost foreground/bold style: %q", i, row)
		}
	}
}

func TestWrappedDiffKeepsBodyAndGutterStyles(t *testing.T) {
	bodyStyle := "\x1b[48;2;0;50;0m\x1b[38;2;110;100;90m"
	gutterStyle := "\x1b[34m"
	prefix := " 58 ⋮ 62 │ "
	m := wrappedModel(t, "diff")
	m.diffEnabled = true
	m.diff = gutterStyle + prefix + "\x1b[0m" + bodyStyle + strings.Repeat("# rollout comment ", 8) + "\x1b[0m"
	wantBody := styleAtColumn(bodyStyle+"X", 0)
	wantGutter := styleAtColumn(gutterStyle+"│", 0)
	view := m.renderDiffViewport(38, 12)
	for i, row := range strings.Split(view, "\n") {
		plain := stripANSI(row)
		if strings.TrimSpace(plain) == "" {
			continue
		}
		gutterColumn := ansi.StringWidth(strings.SplitN(plain, "│", 2)[0])
		gotGutter := styleAtColumn(row, gutterColumn)
		if !gotGutter.Equal(&wantGutter) {
			t.Errorf("diff row %d lost gutter color: %q", i, row)
		}
		gotBody := styleAtColumn(row, ansi.StringWidth(prefix))
		if !gotBody.Equal(&wantBody) {
			t.Errorf("diff row %d lost body foreground/background: %q", i, row)
		}
	}
}

func TestWrappedRowsPreserveStyleTransitionsAndResets(t *testing.T) {
	content := "\x1b[1;31mred bold words before " +
		"\x1b[22;38;2;30;150;200mblue words after " +
		"\x1b[0mnormal words then " +
		"\x1b[3;4;48;5;22mbackground italic underlined words\x1b[0m"
	wantText := strings.Join(strings.Fields(stripANSI(content)), "")
	for _, kind := range []string{"summary", "diff"} {
		t.Run(kind, func(t *testing.T) {
			var rows []string
			if kind == "summary" {
				rows = strings.Split(wrapStructured(content, 14), "\n")
			} else {
				rows = wrapDiffLine(content, 14)
			}
			var gotText strings.Builder
			originalColumn := 0
			original := stripANSI(content)
			for i, row := range rows {
				row = truncateANSIWidth(row, 14)
				for col, char := range stripANSI(row) {
					if char == ' ' {
						continue
					}
					for originalColumn < len(original) && original[originalColumn] == ' ' {
						originalColumn++
					}
					got, want := styleAtColumn(row, col), styleAtColumn(content, originalColumn)
					if !got.Equal(&want) {
						t.Errorf("row %d column %d has wrong style for %q: %q", i, col, char, row)
					}
					gotText.WriteRune(char)
					originalColumn++
				}
				endStyle := styleAtColumn(row, 1000)
				if !endStyle.Empty() {
					t.Errorf("row %d leaks styling into pane padding", i)
				}
			}
			if gotText.String() != wantText {
				t.Errorf("wrapping changed content: %q, want %q", gotText.String(), wantText)
			}
		})
	}
}
