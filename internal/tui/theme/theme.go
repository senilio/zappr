package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

type Palette struct {
	Name    string
	Safe    lipgloss.AdaptiveColor
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
	Pending     lipgloss.Style
	Add         lipgloss.Style
	Delete      lipgloss.Style
	Hunk        lipgloss.Style
	File        lipgloss.Style
}

func Default() Theme {
	return DefaultWithName("")
}

func DefaultWithName(name string) Theme {
	profile := termenv.ColorProfile()
	noColor := profile == termenv.Ascii
	var palette Palette
	if name != "" {
		palette = PaletteByName(name)
	} else if noColor {
		palette = DarkANSI()
	} else {
		palette = DarkDefault()
	}
	// Always use ANSI fallback for truly ascii-only terminals regardless of named choice.
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
		Pending:     base.Bold(true).Foreground(p.Pending),
		Add:         base.Foreground(p.Safe),
		Delete:      base.Foreground(p.Block),
		Hunk:        base.Foreground(p.Focus).Bold(true),
		File:        base.Foreground(p.Focus).Bold(true),
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

func NordPalette() Palette {
	return Palette{
		Name:    "nord",
		Safe:    lipgloss.AdaptiveColor{Light: "#4CAF7D", Dark: "#a3be8c"},
		Block:   lipgloss.AdaptiveColor{Light: "#BF616A", Dark: "#bf616a"},
		Pending: lipgloss.AdaptiveColor{Light: "#5E81AC", Dark: "#81a1c1"},
		Muted:   lipgloss.AdaptiveColor{Light: "#7B8EA8", Dark: "#4c566a"},
		Focus:   lipgloss.AdaptiveColor{Light: "#5E81AC", Dark: "#88c0d0"},
		Text:    lipgloss.AdaptiveColor{Light: "#2E3440", Dark: "#eceff4"},
		Subtle:  lipgloss.AdaptiveColor{Light: "#3B4252", Dark: "#d8dee9"},
		Border:  lipgloss.AdaptiveColor{Light: "#BBC3D4", Dark: "#3b4252"},
		Panel:   lipgloss.AdaptiveColor{Light: "#ECEFF4", Dark: "#2e3440"},
		Error:   lipgloss.AdaptiveColor{Light: "#BF616A", Dark: "#bf616a"},
	}
}

func GruvboxPalette() Palette {
	return Palette{
		Name:    "gruvbox",
		Safe:    lipgloss.AdaptiveColor{Light: "#79740E", Dark: "#b8bb26"},
		Block:   lipgloss.AdaptiveColor{Light: "#CC241D", Dark: "#fb4934"},
		Pending: lipgloss.AdaptiveColor{Light: "#076678", Dark: "#83a598"},
		Muted:   lipgloss.AdaptiveColor{Light: "#928374", Dark: "#928374"},
		Focus:   lipgloss.AdaptiveColor{Light: "#8F3F71", Dark: "#d3869b"},
		Text:    lipgloss.AdaptiveColor{Light: "#3C3836", Dark: "#ebdbb2"},
		Subtle:  lipgloss.AdaptiveColor{Light: "#504945", Dark: "#d5c4a1"},
		Border:  lipgloss.AdaptiveColor{Light: "#D5C4A1", Dark: "#504945"},
		Panel:   lipgloss.AdaptiveColor{Light: "#FBF1C7", Dark: "#282828"},
		Error:   lipgloss.AdaptiveColor{Light: "#CC241D", Dark: "#fb4934"},
	}
}

func CatppuccinPalette() Palette {
	return Palette{
		Name:    "catppuccin",
		Safe:    lipgloss.AdaptiveColor{Light: "#40A02B", Dark: "#a6e3a1"},
		Block:   lipgloss.AdaptiveColor{Light: "#D20F39", Dark: "#f38ba8"},
		Pending: lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89b4fa"},
		Muted:   lipgloss.AdaptiveColor{Light: "#8C8FA1", Dark: "#585b70"},
		Focus:   lipgloss.AdaptiveColor{Light: "#7287FD", Dark: "#cba6f7"},
		Text:    lipgloss.AdaptiveColor{Light: "#4C4F69", Dark: "#cdd6f4"},
		Subtle:  lipgloss.AdaptiveColor{Light: "#5C5F77", Dark: "#bac2de"},
		Border:  lipgloss.AdaptiveColor{Light: "#CCD0DA", Dark: "#313244"},
		Panel:   lipgloss.AdaptiveColor{Light: "#EFF1F5", Dark: "#1e1e2e"},
		Error:   lipgloss.AdaptiveColor{Light: "#D20F39", Dark: "#f38ba8"},
	}
}

func EverforestPalette() Palette {
	return Palette{
		Name:    "everforest",
		Safe:    lipgloss.AdaptiveColor{Light: "#8DA101", Dark: "#a7c080"},
		Block:   lipgloss.AdaptiveColor{Light: "#F85552", Dark: "#e67e80"},
		Pending: lipgloss.AdaptiveColor{Light: "#3A94C5", Dark: "#7fbbb3"},
		Muted:   lipgloss.AdaptiveColor{Light: "#939F91", Dark: "#859289"},
		Focus:   lipgloss.AdaptiveColor{Light: "#DF69BA", Dark: "#d699b6"},
		Text:    lipgloss.AdaptiveColor{Light: "#5C6A72", Dark: "#d3c6aa"},
		Subtle:  lipgloss.AdaptiveColor{Light: "#708089", Dark: "#9da9a0"},
		Border:  lipgloss.AdaptiveColor{Light: "#D8CAAC", Dark: "#4f5b58"},
		Panel:   lipgloss.AdaptiveColor{Light: "#FDF6E3", Dark: "#2d3b30"},
		Error:   lipgloss.AdaptiveColor{Light: "#F85552", Dark: "#e67e80"},
	}
}

// allPalettes is the ordered registry of built-in color palettes.
var allPalettes = []Palette{
	DarkDefault(),
	NordPalette(),
	GruvboxPalette(),
	CatppuccinPalette(),
	EverforestPalette(),
	DarkANSI(),
}

// AllPalettes returns all registered palettes in display order.
func AllPalettes() []Palette {
	return allPalettes
}

// PaletteByName returns the palette matching name (case-insensitive), or DarkDefault() if not found.
func PaletteByName(name string) Palette {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, p := range allPalettes {
		if strings.ToLower(p.Name) == name {
			return p
		}
	}
	return DarkDefault()
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
