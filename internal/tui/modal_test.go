package tui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func assertModalColor(t *testing.T, got, want color.Color) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Errorf("color = %v, want %v", got, want)
		}
		return
	}
	if color.RGBAModel.Convert(got) != color.RGBAModel.Convert(want) {
		t.Errorf("color = %v, want %v", got, want)
	}
}

func TestModalBackdropRetainsPills(t *testing.T) {
	for _, dark := range []bool{true, false} {
		for _, state := range []uitheme.PillState{uitheme.PillNone, uitheme.PillPassed, uitheme.PillWait, uitheme.PillFailed} {
			t.Run(fmt.Sprintf("dark=%v/state=%v", dark, state), func(t *testing.T) {
				m := minimalModel()
				m.theme = uitheme.New(uitheme.DarkDefault(dark), colorprofile.TrueColor, false, dark)
				pill := m.theme.Pill("CI", state, uitheme.PillRounded, 0.7)
				original := styleAtColumn(pill, 1)
				got := m.dimLineWithPreserve(pill, 4, nil)
				left, body, right := styleAtColumn(got, 0), styleAtColumn(got, 1), styleAtColumn(got, 3)
				assertModalColor(t, body.Bg, m.theme.Fade(original.Bg, 0.4))
				assertModalColor(t, body.Fg, m.theme.Fade(original.Fg, 0.4))
				assertModalColor(t, left.Fg, body.Bg)
				assertModalColor(t, right.Fg, body.Bg)
				if body.Fg != nil && body.Bg != nil && color.RGBAModel.Convert(body.Fg) == color.RGBAModel.Convert(body.Bg) {
					t.Error("dimmed label disappeared into its background")
				}
				if ansi.StringWidth(got) != 4 || stripANSI(got) != stripANSI(pill) {
					t.Fatalf("dimming changed pill geometry: %q", got)
				}
				if end := styleAtColumn(got, 10); !end.Empty() {
					t.Fatal("backdrop leaked its final style")
				}
			})
		}
	}
}

func TestModalBackdropPreservesANSIColorsAfterConversion(t *testing.T) {
	render16 := func(s string) string {
		t.Helper()
		var b strings.Builder
		w := colorprofile.Writer{Forward: &b, Profile: colorprofile.ANSI}
		if _, err := w.WriteString(s); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	for _, tc := range []struct {
		name    string
		palette uitheme.Palette
		profile colorprofile.Profile
	}{
		{"ANSI palette/16 colors", uitheme.DarkANSI(), colorprofile.ANSI},
		{"ANSI palette/truecolor", uitheme.DarkANSI(), colorprofile.TrueColor},
		{"default palette/16 colors", uitheme.DarkDefault(), colorprofile.ANSI},
	} {
		for _, state := range []uitheme.PillState{uitheme.PillNone, uitheme.PillPassed, uitheme.PillWait, uitheme.PillFailed} {
			t.Run(fmt.Sprintf("%s/state=%d", tc.name, state), func(t *testing.T) {
				m := minimalModel()
				m.theme = uitheme.New(tc.palette, tc.profile, false)
				line := m.theme.Styles.Muted.Render("title · #2 user ─ ") + m.theme.Pill("CI", state, uitheme.PillRounded, 0.7)
				original := render16(line)
				got := render16(m.dimLineWithPreserve(line, ansi.StringWidth(line), nil))
				if stripANSI(got) != stripANSI(original) || ansi.StringWidth(got) != ansi.StringWidth(original) {
					t.Fatalf("ANSI backdrop geometry changed: %q", got)
				}
				for col := 0; col < ansi.StringWidth(line); col++ {
					before, after := styleAtColumn(original, col), styleAtColumn(got, col)
					assertModalColor(t, after.Fg, before.Fg)
					assertModalColor(t, after.Bg, before.Bg)
					if after.Attrs&cellbuf.FaintAttr == 0 {
						t.Errorf("column %d is not dimmed with attributes", col)
					}
				}
			})
		}
	}
}

func TestModalBackdropStyleTransitionsAndWideCharacters(t *testing.T) {
	m := minimalModel()
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	line := "\x1b[1;38;2;100;180;200;48;2;20;60;80mAB\x1b[0mC\x1b[34;48;5;22m界D\x1b[0mE"
	got := m.dimLineWithPreserve(line, 7, nil)
	if stripANSI(got) != "ABC界DE" || ansi.StringWidth(got) != 7 {
		t.Fatalf("backdrop changed wide text: %q", got)
	}
	for _, col := range []int{0, 2, 3, 5, 6} {
		original, dimmed := styleAtColumn(line, col), styleAtColumn(got, col)
		assertModalColor(t, dimmed.Fg, m.theme.Fade(original.Fg, 0.4))
		assertModalColor(t, dimmed.Bg, m.theme.Fade(original.Bg, 0.4))
		if original.Fg == nil && dimmed.Attrs&cellbuf.FaintAttr == 0 {
			t.Errorf("default text not dimmed at column %d", col)
		}
		if original.Attrs&cellbuf.BoldAttr != dimmed.Attrs&cellbuf.BoldAttr {
			t.Errorf("bold attribute changed at column %d", col)
		}
	}
}

func TestModalPreservesStyledSidesAndSelection(t *testing.T) {
	m := minimalModel()
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	pill := m.theme.Pill("CI", uitheme.PillPassed, uitheme.PillRounded, 0.7)
	line := "  " + pill + strings.Repeat(" ", 24) + pill + "  "
	modal := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render("MODAL")
	view := m.overlayModal(strings.Join([]string{line, line, line}, "\n"), modal, 36, 3,
		[]preserveRange{{row: 1, start: 2, end: 6}})
	rows := strings.Split(view, "\n")
	for _, row := range rows {
		if ansi.StringWidth(row) != 36 {
			t.Fatalf("overlay row width = %d: %q", ansi.StringWidth(row), row)
		}
		if end := styleAtColumn(row, 100); !end.Empty() {
			t.Fatal("overlay leaked its final style")
		}
	}
	selected, original := styleAtColumn(rows[1], 3), styleAtColumn(pill, 1)
	if !selected.Equal(&original) {
		t.Error("selected pill lost its style beside the modal")
	}
	rightCap, rightBody := styleAtColumn(rows[1], 30), styleAtColumn(rows[1], 31)
	assertModalColor(t, rightCap.Fg, rightBody.Bg)
	assertModalColor(t, rightBody.Bg, m.theme.Fade(original.Bg, 0.4))
	modalStyle, wantModalStyle := styleAtColumn(rows[1], 16), styleAtColumn(modal, 1)
	if !modalStyle.Equal(&wantModalStyle) {
		t.Error("backdrop styling leaked into the modal")
	}
}

func TestModalClipsWideCharactersAtEdges(t *testing.T) {
	m := minimalModel()
	line := strings.Repeat("界", 8)
	view := m.overlayModal(line, "X", 16, 1, nil)
	if ansi.StringWidth(view) != 16 || !strings.Contains(stripANSI(view), "X") {
		t.Fatalf("overlay split a wide character incorrectly: %q", view)
	}
}

func TestModalBackdropPreservesCombiningCharacters(t *testing.T) {
	m := minimalModel()
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	for _, line := range []string{
		"Cafe\u0301",
		"\x1b[31mCafe\u0301\x1b[0m",
		"Ae\x1b[31m\u0301\x1b[0mB",
		"n\u0303o\u0308 · 界\u0301",
		"A1\ufe0f\u20e3B · 👨‍👩‍👧‍👦",
	} {
		t.Run(fmt.Sprintf("%q", line), func(t *testing.T) {
			width := ansi.StringWidth(line)
			got := m.dimLineWithPreserve(line, width, nil)
			if stripANSI(got) != stripANSI(line) || ansi.StringWidth(got) != width {
				t.Fatalf("backdrop changed graphemes: got %q, want %q", stripANSI(got), stripANSI(line))
			}
		})
	}
}

func TestCutModalLineKeepsGraphemesWithTheirBase(t *testing.T) {
	for _, tc := range []struct {
		name, line  string
		left, right int
		want        string
	}{
		{"exclude accent and base", "Ae\u0301B", 0, 1, "A"},
		{"include accent and base", "Ae\u0301B", 1, 2, "e\u0301"},
		{"exclude preceding accent", "Ae\u0301B", 2, 3, "B"},
		{"include whole prefix", "Ae\u0301B", 0, 2, "Ae\u0301"},
		{"padding excludes accents", "Ae\u0301B", 3, 5, "  "},
		{"styled excluded accent", "Ae\x1b[31m\u0301\x1b[0mB", 0, 1, "A"},
		{"styled included accent", "Ae\x1b[31m\u0301\x1b[0mB", 1, 2, "e\u0301"},
		{"partial wide cluster", "A界\u0301B", 1, 2, " "},
		{"partial keycap cluster", "A1\ufe0f\u20e3B", 1, 2, " "},
		{"whole keycap cluster", "A1\ufe0f\u20e3B", 1, 3, "1\ufe0f\u20e3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := cutModalLine(tc.line, tc.left, tc.right)
			if stripANSI(got) != tc.want || ansi.StringWidth(got) != tc.right-tc.left {
				t.Fatalf("clipped grapheme changed: got %q, want %q", stripANSI(got), tc.want)
			}
		})
	}
}

func TestModalOverlayPreservesCombiningCharactersBesideModal(t *testing.T) {
	m := minimalModel()
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	line := m.theme.Styles.Muted.Render(strings.Repeat("Ae\u0301", 6))
	got := m.overlayModal(line, "X", 12, 1, []preserveRange{{row: 0, start: 0, end: 4}})
	want := "Ae\u0301Ae\u0301AXAe\u0301Ae\u0301Ae\u0301"
	if stripANSI(got) != want || ansi.StringWidth(got) != 12 {
		t.Fatalf("overlay changed graphemes beside modal: got %q, want %q", stripANSI(got), want)
	}
}

func TestModalHelpAndCommandRetainUnselectedPills(t *testing.T) {
	for _, mode := range []Mode{ModeHelp, ModeCommand} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			m := modelWithPRs([]model.PullRequest{
				{Number: 1, Title: "selected", CIStatus: "passing"},
				{Number: 2, Title: "other", Author: "unselected", CIStatus: "passing"},
			}, 0)
			m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
			m.width, m.height, m.mode = 130, 36, mode
			m.resizeViewports()
			for _, row := range strings.Split(m.View().Content, "\n") {
				plain := stripANSI(row)
				if !strings.Contains(plain, "unselected") {
					continue
				}
				parts := strings.SplitN(plain, "\ue0b6CI\ue0b4", 2)
				if len(parts) != 2 {
					t.Fatalf("pill missing from unselected row: %q", plain)
				}
				col := ansi.StringWidth(parts[0])
				cap, body := styleAtColumn(row, col), styleAtColumn(row, col+1)
				if body.Bg == nil {
					t.Fatal("unselected pill lost its fill behind the modal")
				}
				assertModalColor(t, cap.Fg, body.Bg)
				return
			}
			t.Fatal("unselected row not visible in backdrop")
		})
	}
}
