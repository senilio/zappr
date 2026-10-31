package theme

import (
	"image/color"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
)

var pillStates = []PillState{PillNone, PillPassed, PillWait, PillFailed}

func colorTheme() Theme   { return New(DarkDefault(), colorprofile.TrueColor, false) }
func ansiTheme() Theme    { return New(DarkANSI(), colorprofile.ANSI, false) }
func noColorTheme() Theme { return New(DarkANSI(), colorprofile.ASCII, true) }

func TestPillRoundedText(t *testing.T) {
	for _, st := range pillStates {
		if got := ansi.Strip(colorTheme().Pill("CI", st, PillRounded, 0.7)); got != "\ue0b6CI\ue0b4" {
			t.Errorf("state %v: %q", st, got)
		}
	}
}

func TestPillSquareText(t *testing.T) {
	for _, st := range pillStates {
		if got := ansi.Strip(colorTheme().Pill("CI", st, PillSquare, 0.7)); got != " CI " {
			t.Errorf("state %v: %q", st, got)
		}
	}
}

func TestPillWidthConstantAcrossModes(t *testing.T) {
	for _, th := range []Theme{colorTheme(), ansiTheme(), noColorTheme()} {
		for _, label := range []string{"CI", "TF", "conflict"} {
			for _, st := range pillStates {
				for _, shape := range []PillShape{PillRounded, PillSquare} {
					got := th.Pill(label, st, shape, 0.7)
					if w := ansi.StringWidth(got); w != len(label)+2 {
						t.Errorf("%s %q state %v shape %v: width %d", th.Palette.Name, label, st, shape, w)
					}
				}
			}
		}
	}
}

func TestPillAnsiPaletteHasNoRoundedEnds(t *testing.T) {
	for _, st := range pillStates {
		if got := ansi.Strip(ansiTheme().Pill("CI", st, PillRounded, 0.7)); got != " CI " {
			t.Errorf("state %v: %q", st, got)
		}
	}
}

func TestPillNoColorAddsSymbols(t *testing.T) {
	want := map[PillState]string{PillPassed: " CI✓", PillWait: " CI…", PillFailed: " CI✗", PillNone: " CI–"}
	for st, w := range want {
		got := noColorTheme().Pill("CI", st, PillRounded, 0.7)
		if got != w || got != ansi.Strip(got) {
			t.Errorf("state %v: %q, want %q", st, got, w)
		}
	}
}

func TestPillFailedUsesSolidBackground(t *testing.T) {
	th := colorTheme()
	for _, st := range []PillState{PillFailed, PillPassed} {
		if got := th.Pill("CI", st, PillSquare, 1); !strings.Contains(got, "48;2;") {
			t.Errorf("state %v has no truecolor background: %q", st, got)
		}
	}
	if got := ansiTheme().Pill("CI", PillPassed, PillSquare, 0.7); strings.Contains(got, "48;") {
		t.Errorf("ansi passed pill has a background: %q", got)
	}
}

func pillStyleAt(line string, column int) cellbuf.Style {
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

func TestPillOpacityPreservesForegroundAndShape(t *testing.T) {
	equal := func(a, b color.Color) bool { return color.RGBAModel.Convert(a) == color.RGBAModel.Convert(b) }
	for _, dark := range []bool{true, false} {
		th := New(DarkDefault(dark), colorprofile.TrueColor, false, dark)
		for _, state := range pillStates {
			full := pillStyleAt(th.Pill("CI", state, PillRounded, 1), 1)
			for _, opacity := range []float64{0, 0.7, 1} {
				pill := th.Pill("CI", state, PillRounded, opacity)
				left, body, right := pillStyleAt(pill, 0), pillStyleAt(pill, 1), pillStyleAt(pill, 3)
				if body.Fg == nil || body.Bg == nil || left.Fg == nil || right.Fg == nil {
					t.Fatalf("pill lost colors: %q", pill)
				}
				if state != PillFailed && !equal(body.Fg, full.Fg) {
					t.Error("opacity changed label foreground")
				}
				if !equal(left.Fg, body.Bg) || !equal(right.Fg, body.Bg) {
					t.Error("rounded caps differ from the fill")
				}
				if opacity == 0 && !equal(body.Bg, th.Palette.Panel) {
					t.Error("zero opacity did not use panel color")
				}
				if opacity == 0.7 && !equal(full.Bg, th.Palette.Panel) && equal(body.Bg, full.Bg) {
					t.Errorf("opacity did not soften background: dark=%v state=%v bg=%v panel=%v", dark, state, full.Bg, th.Palette.Panel)
				}
				if ansi.StringWidth(pill) != 4 {
					t.Errorf("opacity changed width: %q", pill)
				}
			}
		}
	}
}

func TestPillOpacityKeepsFailureReadable(t *testing.T) {
	for _, dark := range []bool{true, false} {
		th := New(DarkDefault(dark), colorprofile.TrueColor, false, dark)
		for _, opacity := range []float64{0, 0.3, 0.7, 1} {
			body := pillStyleAt(th.Pill("FAILED", PillFailed, PillSquare, opacity), 1)
			if color.RGBAModel.Convert(body.Fg) == color.RGBAModel.Convert(body.Bg) {
				t.Errorf("failure label disappeared: dark=%v opacity=%v", dark, opacity)
			}
		}
	}
}

func TestPillWithoutPalette(t *testing.T) {
	if got := (Theme{}).Pill("CI", PillFailed, PillSquare, 0.7); ansi.Strip(got) != " CI " {
		t.Fatalf("zero-theme pill = %q", got)
	}
}

func TestReviewIcon(t *testing.T) {
	decisions := []string{"APPROVED", "CHANGES_REQUESTED", "REVIEW_REQUIRED", ""}
	cases := []struct {
		th    Theme
		shape PillShape
		want  []string
	}{
		{colorTheme(), PillRounded, []string{"\uf4a4", "\uf530", "\uf4ab", "\uf4aa"}},
		{colorTheme(), PillSquare, []string{"✓", "✗", "●", "○"}},
		{ansiTheme(), PillRounded, []string{"✓", "✗", "●", "○"}},
		{noColorTheme(), PillRounded, []string{"✓", "✗", "●", "○"}},
	}
	for _, tc := range cases {
		for i, d := range decisions {
			got := tc.th.ReviewIcon(d, false, tc.shape)
			if ansi.Strip(got) != tc.want[i] || ansi.StringWidth(got) != 1 {
				t.Errorf("%s shape %v decision %q: %q", tc.th.Palette.Name, tc.shape, d, got)
			}
		}
	}
	if got := ansi.Strip(colorTheme().ReviewIcon("APPROVED", true, PillRounded)); got != "\uf4aa" {
		t.Errorf("draft icon = %q", got)
	}
	if got := noColorTheme().ReviewIcon("APPROVED", false, PillRounded); got != "✓" {
		t.Errorf("no-color icon is styled: %q", got)
	}
}

func TestParsePillShape(t *testing.T) {
	cases := map[string]PillShape{"": PillRounded, "square": PillSquare, " Square ": PillSquare, "round": PillRounded}
	for in, want := range cases {
		if got := ParsePillShape(in); got != want {
			t.Errorf("ParsePillShape(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestAllPalettesHaveWait(t *testing.T) {
	for _, dark := range []bool{true, false} {
		for _, p := range AllPalettes(dark) {
			if p.Wait == nil {
				t.Errorf("palette %s (dark=%v) has no Wait color", p.Name, dark)
			}
		}
	}
}
