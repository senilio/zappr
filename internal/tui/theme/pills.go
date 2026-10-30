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
func (t Theme) Pill(label string, state PillState, shape PillShape) string {
	if t.NoColor {
		return " " + label + pillSymbol(state)
	}
	fg := t.stateColor(state)
	if t.isANSIPalette() {
		style := lipgloss.NewStyle().Foreground(fg)
		if state == PillFailed {
			style = lipgloss.NewStyle().Background(lipgloss.Color("1")).Foreground(lipgloss.Color("0")).Bold(true)
		}
		return style.Render(" " + label + " ")
	}
	var bg color.Color
	switch {
	case state == PillFailed:
		bg, fg = t.Palette.Error, t.Palette.Panel
	case !t.IsDark:
		bg = lipgloss.Lighten(fg, 0.75)
	case state == PillNone:
		bg = lipgloss.Darken(fg, 0.75)
	default:
		bg = lipgloss.Darken(fg, 0.7)
	}
	body := lipgloss.NewStyle().Background(bg).Foreground(fg).Bold(state == PillFailed)
	if shape == PillSquare {
		return body.Render(" " + label + " ")
	}
	end := lipgloss.NewStyle().Foreground(bg)
	return end.Render(pillLeftEnd) + body.Render(label) + end.Render(pillRightEnd)
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

// ReviewIcon renders a one-cell review decision icon.
func (t Theme) ReviewIcon(decision string, draft bool, shape PillShape) string {
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
	nerd := map[PillState]string{PillPassed: "\uf4a4", PillFailed: "\uf530", PillWait: "\uf4ab", PillNone: "\uf4aa"}
	plain := map[PillState]string{PillPassed: "✓", PillFailed: "✗", PillWait: "●", PillNone: "○"}
	if t.NoColor {
		return plain[state]
	}
	icon := nerd[state]
	if shape == PillSquare || t.isANSIPalette() {
		icon = plain[state]
	}
	return lipgloss.NewStyle().Foreground(t.stateColor(state)).Render(icon)
}
