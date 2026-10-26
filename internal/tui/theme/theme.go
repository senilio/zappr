package theme

import (
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

type Palette struct {
	Name    string
	Safe    color.Color
	Block   color.Color
	Pending color.Color
	Muted   color.Color
	Focus   color.Color
	Text    color.Color
	Subtle  color.Color
	Border  color.Color
	Panel   color.Color
	Error   color.Color
}

type Theme struct {
	Palette Palette
	Styles  Styles
	Profile colorprofile.Profile
	NoColor bool
	IsDark  bool
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
	profile := colorprofile.Detect(os.Stdout, os.Environ())
	noColor := profile <= colorprofile.ASCII
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

func New(p Palette, profile colorprofile.Profile, noColor bool, isDark ...bool) Theme {
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
	return Theme{Palette: p, Styles: styles, Profile: profile, NoColor: noColor, IsDark: len(isDark) == 0 || isDark[0]}
}

// WithBackground resolves the palette for this model, without global state.
func (t Theme) WithBackground(isDark bool) Theme {
	return New(PaletteByName(t.Palette.Name, isDark), t.Profile, t.NoColor, isDark)
}

func DarkDefault(isDark ...bool) Palette {
	ld := lipgloss.LightDark(len(isDark) == 0 || isDark[0])
	return Palette{
		Name:    "dark",
		Safe:    ld(lipgloss.Color("#047857"), lipgloss.Color("#4ade80")),
		Block:   ld(lipgloss.Color("#b91c1c"), lipgloss.Color("#f87171")),
		Pending: ld(lipgloss.Color("#1d4ed8"), lipgloss.Color("#60a5fa")),
		Muted:   ld(lipgloss.Color("#6b7280"), lipgloss.Color("#9ca3af")),
		Focus:   ld(lipgloss.Color("#7c3aed"), lipgloss.Color("#a78bfa")),
		Text:    ld(lipgloss.Color("#111827"), lipgloss.Color("#e5e7eb")),
		Subtle:  ld(lipgloss.Color("#4b5563"), lipgloss.Color("#d1d5db")),
		Border:  ld(lipgloss.Color("#d1d5db"), lipgloss.Color("#374151")),
		Panel:   ld(lipgloss.Color("#ffffff"), lipgloss.Color("#111827")),
		Error:   ld(lipgloss.Color("#dc2626"), lipgloss.Color("#fb7185")),
	}
}

func DarkANSI(isDark ...bool) Palette {
	return Palette{
		Name:    "ansi",
		Safe:    lipgloss.Color("2"),
		Block:   lipgloss.Color("1"),
		Pending: lipgloss.Color("4"),
		Muted:   lipgloss.Color("8"),
		Focus:   lipgloss.Color("5"),
		Text:    lipgloss.Color("15"),
		Subtle:  lipgloss.Color("7"),
		Border:  lipgloss.Color("8"),
		Panel:   lipgloss.Color("0"),
		Error:   lipgloss.Color("1"),
	}
}

func NordPalette(isDark ...bool) Palette {
	ld := lipgloss.LightDark(len(isDark) == 0 || isDark[0])
	return Palette{
		Name:    "nord",
		Safe:    ld(lipgloss.Color("#4CAF7D"), lipgloss.Color("#a3be8c")),
		Block:   ld(lipgloss.Color("#BF616A"), lipgloss.Color("#bf616a")),
		Pending: ld(lipgloss.Color("#5E81AC"), lipgloss.Color("#81a1c1")),
		Muted:   ld(lipgloss.Color("#7B8EA8"), lipgloss.Color("#4c566a")),
		Focus:   ld(lipgloss.Color("#5E81AC"), lipgloss.Color("#88c0d0")),
		Text:    ld(lipgloss.Color("#2E3440"), lipgloss.Color("#eceff4")),
		Subtle:  ld(lipgloss.Color("#3B4252"), lipgloss.Color("#d8dee9")),
		Border:  ld(lipgloss.Color("#BBC3D4"), lipgloss.Color("#3b4252")),
		Panel:   ld(lipgloss.Color("#ECEFF4"), lipgloss.Color("#2e3440")),
		Error:   ld(lipgloss.Color("#BF616A"), lipgloss.Color("#bf616a")),
	}
}

func GruvboxPalette(isDark ...bool) Palette {
	ld := lipgloss.LightDark(len(isDark) == 0 || isDark[0])
	return Palette{
		Name:    "gruvbox",
		Safe:    ld(lipgloss.Color("#79740E"), lipgloss.Color("#b8bb26")),
		Block:   ld(lipgloss.Color("#CC241D"), lipgloss.Color("#fb4934")),
		Pending: ld(lipgloss.Color("#076678"), lipgloss.Color("#83a598")),
		Muted:   ld(lipgloss.Color("#928374"), lipgloss.Color("#928374")),
		Focus:   ld(lipgloss.Color("#8F3F71"), lipgloss.Color("#d3869b")),
		Text:    ld(lipgloss.Color("#3C3836"), lipgloss.Color("#ebdbb2")),
		Subtle:  ld(lipgloss.Color("#504945"), lipgloss.Color("#d5c4a1")),
		Border:  ld(lipgloss.Color("#D5C4A1"), lipgloss.Color("#504945")),
		Panel:   ld(lipgloss.Color("#FBF1C7"), lipgloss.Color("#282828")),
		Error:   ld(lipgloss.Color("#CC241D"), lipgloss.Color("#fb4934")),
	}
}

func CatppuccinPalette(isDark ...bool) Palette {
	ld := lipgloss.LightDark(len(isDark) == 0 || isDark[0])
	return Palette{
		Name:    "catppuccin",
		Safe:    ld(lipgloss.Color("#40A02B"), lipgloss.Color("#a6e3a1")),
		Block:   ld(lipgloss.Color("#D20F39"), lipgloss.Color("#f38ba8")),
		Pending: ld(lipgloss.Color("#1E66F5"), lipgloss.Color("#89b4fa")),
		Muted:   ld(lipgloss.Color("#8C8FA1"), lipgloss.Color("#585b70")),
		Focus:   ld(lipgloss.Color("#7287FD"), lipgloss.Color("#cba6f7")),
		Text:    ld(lipgloss.Color("#4C4F69"), lipgloss.Color("#cdd6f4")),
		Subtle:  ld(lipgloss.Color("#5C5F77"), lipgloss.Color("#bac2de")),
		Border:  ld(lipgloss.Color("#CCD0DA"), lipgloss.Color("#313244")),
		Panel:   ld(lipgloss.Color("#EFF1F5"), lipgloss.Color("#1e1e2e")),
		Error:   ld(lipgloss.Color("#D20F39"), lipgloss.Color("#f38ba8")),
	}
}

func EverforestPalette(isDark ...bool) Palette {
	ld := lipgloss.LightDark(len(isDark) == 0 || isDark[0])
	return Palette{
		Name:    "everforest",
		Safe:    ld(lipgloss.Color("#8DA101"), lipgloss.Color("#a7c080")),
		Block:   ld(lipgloss.Color("#F85552"), lipgloss.Color("#e67e80")),
		Pending: ld(lipgloss.Color("#3A94C5"), lipgloss.Color("#7fbbb3")),
		Muted:   ld(lipgloss.Color("#939F91"), lipgloss.Color("#859289")),
		Focus:   ld(lipgloss.Color("#DF69BA"), lipgloss.Color("#d699b6")),
		Text:    ld(lipgloss.Color("#5C6A72"), lipgloss.Color("#d3c6aa")),
		Subtle:  ld(lipgloss.Color("#708089"), lipgloss.Color("#9da9a0")),
		Border:  ld(lipgloss.Color("#D8CAAC"), lipgloss.Color("#4f5b58")),
		Panel:   ld(lipgloss.Color("#FDF6E3"), lipgloss.Color("#2d3b30")),
		Error:   ld(lipgloss.Color("#F85552"), lipgloss.Color("#e67e80")),
	}
}

// AllPalettes returns all registered palettes in display order.
func AllPalettes(isDark ...bool) []Palette {
	return []Palette{DarkDefault(isDark...), NordPalette(isDark...), GruvboxPalette(isDark...), CatppuccinPalette(isDark...), EverforestPalette(isDark...), DarkANSI(isDark...)}
}

// PaletteByName returns the palette matching name (case-insensitive), or DarkDefault() if not found.
func PaletteByName(name string, isDark ...bool) Palette {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, p := range AllPalettes(isDark...) {
		if strings.ToLower(p.Name) == name {
			return p
		}
	}
	return DarkDefault(isDark...)
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
