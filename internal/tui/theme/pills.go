package theme

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// PillState is the status a pill conveys through its color.
type PillState int

const (
	PillNone PillState = iota
	PillPassed
	PillWait
	PillFailed
)

// PillShape selects Nerd Font rounded ends or plain padded labels.
type PillShape int

const (
	PillRounded PillShape = iota
	PillSquare
)

const (
	pillLeftEnd  = "\ue0b6"
	pillRightEnd = "\ue0b4"
)

// ParsePillShape maps the pills config value to a shape; anything other
// than "square" is rounded.
func ParsePillShape(s string) PillShape {
	if strings.EqualFold(strings.TrimSpace(s), "square") {
		return PillSquare
	}
	return PillRounded
}

func (t Theme) stateColor(state PillState) color.Color {
	switch state {
	case PillPassed:
		return t.Palette.Safe
	case PillWait:
		return t.Palette.Wait
	case PillFailed:
		return t.Palette.Error
	default:
		return t.Palette.Muted
	}
}

func (t Theme) isANSIPalette() bool { return t.Palette.Name == "ansi" }

// Pill renders label as a status pill. Its width is always len(label)+2
// cells so pills line up in columns regardless of shape or color mode.
func (t Theme) Pill(label string, state PillState, shape PillShape, opacity float64) string {
	if t.NoColor {
		return " " + label + pillSymbol(state)
	}
	fg := t.stateColor(state)
	if t.isANSIPalette() {
		return lipgloss.NewStyle().Foreground(fg).Render(" " + label + " ")
	}
	var bg color.Color
	switch {
	case !t.IsDark:
		bg = lipgloss.Lighten(fg, 0.75)
	case state == PillNone:
		bg = lipgloss.Darken(fg, 0.75)
	default:
		bg = lipgloss.Darken(fg, 0.7)
	}
	bg = t.Fade(bg, opacity)
	body := lipgloss.NewStyle().Background(bg).Foreground(fg)
	if shape == PillSquare {
		return body.Render(" " + label + " ")
	}
	end := lipgloss.NewStyle().Foreground(bg)
	return end.Render(pillLeftEnd) + body.Render(label) + end.Render(pillRightEnd)
}

// Fade blends a color toward the panel background, without terminal alpha.
func (t Theme) Fade(c color.Color, opacity float64) color.Color {
	if c == nil || t.Palette.Panel == nil {
		return c
	}
	opacity = max(0, min(1, opacity))
	r, g, b, _ := c.RGBA()
	pr, pg, pb, _ := t.Palette.Panel.RGBA()
	blend := func(c, panel uint32) uint8 {
		return uint8(float64(panel>>8)*(1-opacity) + float64(c>>8)*opacity)
	}
	return color.RGBA{R: blend(r, pr), G: blend(g, pg), B: blend(b, pb), A: 255}
}

func pillSymbol(state PillState) string {
	switch state {
	case PillPassed:
		return "✓"
	case PillWait:
		return "…"
	case PillFailed:
		return "✗"
	default:
		return "–"
	}
}

// ReviewPill renders a review decision symbol in a three-cell status pill.
func (t Theme) ReviewPill(decision string, draft bool, shape PillShape, opacity float64) string {
	state := PillNone
	if !draft {
		switch strings.ToUpper(decision) {
		case "APPROVED":
			state = PillPassed
		case "CHANGES_REQUESTED":
			state = PillFailed
		case "REVIEW_REQUIRED":
			state = PillWait
		}
	}
	icon := pillSymbol(state)
	if t.NoColor {
		return " " + icon + " "
	}
	return t.Pill(icon, state, shape, opacity)
}
