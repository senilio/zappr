package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/senilio/zappr/internal/model"
)

type Palette struct {
	Name    string
	Safe    lipgloss.AdaptiveColor
	Review  lipgloss.AdaptiveColor
	Block   lipgloss.AdaptiveColor
	Pending lipgloss.AdaptiveColor
	Muted   lipgloss.AdaptiveColor
	Focus   lipgloss.AdaptiveColor
	Text    lipgloss.AdaptiveColor
	Subtle  lipgloss.AdaptiveColor
	Border  lipgloss.AdaptiveColor
	Panel   lipgloss.AdaptiveColor
	Error   lipgloss.AdaptiveColor
}

type Theme struct {
	Palette Palette
	Styles  Styles
	Profile termenv.Profile
	NoColor bool
}

type Styles struct {
	App         lipgloss.Style
	Header      lipgloss.Style
	Footer      lipgloss.Style
	Pane        lipgloss.Style
	Focused     lipgloss.Style
	Title       lipgloss.Style
	TitleStrong lipgloss.Style
	Subtitle    lipgloss.Style
	Muted       lipgloss.Style
	Help        lipgloss.Style
	Error       lipgloss.Style
	Success     lipgloss.Style
	SuccessCI   lipgloss.Style
	RiskSafe    lipgloss.Style
	RiskReview  lipgloss.Style
	RiskBlock   lipgloss.Style
	Pending     lipgloss.Style
	Add         lipgloss.Style
	Delete      lipgloss.Style
	Hunk        lipgloss.Style
	File        lipgloss.Style
}

func Default() Theme {
	profile := termenv.ColorProfile()
	noColor := profile == termenv.Ascii
	palette := DarkDefault()
	if noColor {
		palette = DarkANSI()
	}
	return New(palette, profile, noColor)
}

func New(p Palette, profile termenv.Profile, noColor bool) Theme {
	base := lipgloss.NewStyle().Foreground(p.Text)
	border := lipgloss.RoundedBorder()
	focusBorder := lipgloss.ThickBorder()
	styles := Styles{
		App:         base,
		Header:      base.Bold(true).Padding(0, 1).Foreground(p.Focus),
		Footer:      base.Padding(0, 1).Foreground(p.Muted),
		Pane:        base.Border(border).BorderForeground(p.Border).Padding(0, 1),
		Focused:     base.Border(focusBorder).BorderForeground(p.Focus).Padding(0, 1),
		Title:       base.Bold(true).Foreground(p.Focus),
		TitleStrong: base.Bold(true).Foreground(p.Text),
		Subtitle:    base.Foreground(p.Subtle),
		Muted:       base.Foreground(p.Muted),
		Help:        base.Foreground(p.Muted),
		Error:       base.Bold(true).Foreground(p.Error),
		Success:     base.Bold(true).Foreground(p.Safe),
		SuccessCI:   base.Foreground(p.Safe),
		RiskSafe:    base.Bold(true).Foreground(p.Safe),
		RiskReview: base.Bold(true).
			Foreground(p.Review),
		RiskBlock: base.Bold(true).Foreground(p.Block),
		Pending:   base.Bold(true).Foreground(p.Pending),
		Add:       base.Foreground(p.Safe),
		Delete:    base.Foreground(p.Block),
		Hunk:      base.Foreground(p.Focus).Bold(true),
		File:      base.Foreground(p.Focus).Bold(true),
	}
	if noColor {
		styles = Styles{
			App:         lipgloss.NewStyle(),
			Header:      lipgloss.NewStyle().Bold(true).Padding(0, 1),
			Footer:      lipgloss.NewStyle().Padding(0, 1),
			Pane:        lipgloss.NewStyle().Border(border).Padding(0, 1),
			Focused:     lipgloss.NewStyle().Border(focusBorder).Padding(0, 1),
			Title:       lipgloss.NewStyle().Bold(true),
			TitleStrong: lipgloss.NewStyle().Bold(true),
			Subtitle:    lipgloss.NewStyle(),
			Muted:       lipgloss.NewStyle(),
			Help:        lipgloss.NewStyle(),
			Error:       lipgloss.NewStyle().Bold(true),
			Success:     lipgloss.NewStyle().Bold(true),
			SuccessCI:   lipgloss.NewStyle(),
			RiskSafe:    lipgloss.NewStyle().Bold(true),
			RiskReview:  lipgloss.NewStyle().Bold(true),
			RiskBlock:   lipgloss.NewStyle().Bold(true),
			Pending:     lipgloss.NewStyle().Bold(true),
			Add:         lipgloss.NewStyle(),
			Delete:      lipgloss.NewStyle(),
			Hunk:        lipgloss.NewStyle().Bold(true),
			File:        lipgloss.NewStyle().Bold(true),
		}
	}
	return Theme{Palette: p, Styles: styles, Profile: profile, NoColor: noColor}
}

func DarkDefault() Palette {
	return Palette{
		Name:    "dark",
		Safe:    lipgloss.AdaptiveColor{Light: "#047857", Dark: "#4ade80"},
		Review:  lipgloss.AdaptiveColor{Light: "#92400e", Dark: "#facc15"},
		Block:   lipgloss.AdaptiveColor{Light: "#b91c1c", Dark: "#f87171"},
		Pending: lipgloss.AdaptiveColor{Light: "#1d4ed8", Dark: "#60a5fa"},
		Muted:   lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#9ca3af"},
		Focus:   lipgloss.AdaptiveColor{Light: "#7c3aed", Dark: "#a78bfa"},
		Text:    lipgloss.AdaptiveColor{Light: "#111827", Dark: "#e5e7eb"},
		Subtle:  lipgloss.AdaptiveColor{Light: "#4b5563", Dark: "#d1d5db"},
		Border:  lipgloss.AdaptiveColor{Light: "#d1d5db", Dark: "#374151"},
		Panel:   lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#111827"},
		Error:   lipgloss.AdaptiveColor{Light: "#dc2626", Dark: "#fb7185"},
	}
}

func DarkANSI() Palette {
	return Palette{
		Name:    "ansi",
		Safe:    lipgloss.AdaptiveColor{Light: "2", Dark: "2"},
		Review:  lipgloss.AdaptiveColor{Light: "3", Dark: "3"},
		Block:   lipgloss.AdaptiveColor{Light: "1", Dark: "1"},
		Pending: lipgloss.AdaptiveColor{Light: "4", Dark: "4"},
		Muted:   lipgloss.AdaptiveColor{Light: "8", Dark: "8"},
		Focus:   lipgloss.AdaptiveColor{Light: "5", Dark: "5"},
		Text:    lipgloss.AdaptiveColor{Light: "15", Dark: "15"},
		Subtle:  lipgloss.AdaptiveColor{Light: "7", Dark: "7"},
		Border:  lipgloss.AdaptiveColor{Light: "8", Dark: "8"},
		Panel:   lipgloss.AdaptiveColor{Light: "0", Dark: "0"},
		Error:   lipgloss.AdaptiveColor{Light: "1", Dark: "1"},
	}
}

func (t Theme) RiskBadge(r model.Risk) string {
	label := strings.ToUpper(string(r))
	switch r {
	case model.RiskSafe:
		return t.Styles.RiskSafe.Render("[" + label + " ok]")
	case model.RiskReview:
		return t.Styles.RiskReview.Render("[" + label + " !]")
	case model.RiskBlock:
		return t.Styles.RiskBlock.Render("[" + label + " x]")
	default:
		return t.Styles.Muted.Render("[unknown]")
	}
}

func (t Theme) CIStatus(status string) string {
	switch strings.ToLower(status) {
	case "passing", "success":
		return t.Styles.SuccessCI.Render("CI pass")
	case "pending", "queued", "in_progress":
		return t.Styles.Pending.Render("CI pending")
	case "failing", "failed", "failure", "error":
		return t.Styles.Error.Render("CI failed")
	default:
		return t.Styles.Muted.Render(status)
	}
}

func (t Theme) ReviewStatus(decision string) string {
	switch strings.ToUpper(decision) {
	case "APPROVED":
		return t.Styles.SuccessCI.Render("approved")
	case "CHANGES_REQUESTED":
		return t.Styles.Error.Render("changes requested")
	case "REVIEW_REQUIRED":
		return t.Styles.Muted.Render("awaiting approval")
	default:
		return ""
	}
}
