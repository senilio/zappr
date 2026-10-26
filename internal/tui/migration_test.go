package tui

import (
	"image/color"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

func TestViewDeclaresTerminalModes(t *testing.T) {
	m := scrollModel(t, 3)
	for _, mode := range []Mode{ModeInbox, ModeDetail, ModeDiff, ModeHelp, ModeComment, ModeConfirmApprove, ModeCommand, ModeSettings} {
		m.mode = mode
		v := m.View()
		if !v.AltScreen || v.MouseMode != tea.MouseModeCellMotion {
			t.Fatalf("mode %d lost terminal modes: %+v", mode, v)
		}
	}
}

func TestBorderedModalsPreserveAllocatedWidth(t *testing.T) {
	m := scrollModel(t, 3)
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	m.width, m.height = 120, 40
	m.syncHelpVP()
	// These widths included the two border columns outside Width in v1.
	for _, tc := range []struct {
		name, content string
		width         int
	}{
		{"help", m.renderHelpModal(), 62},
		{"command", m.renderCommandModal(), 62},
		{"confirmation", m.renderModalBox("Confirm?"), 74},
	} {
		if got := lipgloss.Width(tc.content); got != tc.width {
			t.Errorf("%s width = %d, want %d", tc.name, got, tc.width)
		}
	}
}

func TestV2InputTextAndRelease(t *testing.T) {
	m := scrollModel(t, 3)
	m.mode = ModeComment
	next, _ := m.Update(tea.KeyReleaseMsg{Code: 'q', Text: "q"})
	m = next.(Model)
	if m.comment != "" {
		t.Fatal("key release entered text")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = next.(Model)
	if m.comment != " " {
		t.Fatalf("space text = %q", m.comment)
	}
	next, cmd := m.Update(tea.PasteMsg{Content: "hello\nworld"})
	m = next.(Model)
	if m.comment != " hello\nworld" || cmd != nil {
		t.Fatalf("paste was not literal text: %q", m.comment)
	}
	m.mode = ModeInbox
	next, cmd = m.Update(tea.PasteMsg{Content: "a"})
	if next.(Model).mode != ModeInbox || cmd != nil {
		t.Fatal("paste triggered an action")
	}
}

func TestBackgroundChangeInvalidatesLayout(t *testing.T) {
	m := scrollModel(t, 3)
	m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
	m.syncInboxVP()
	before := m.inboxVPLayout
	next, _ := m.Update(tea.BackgroundColorMsg{Color: color.White})
	m = next.(Model)
	if m.theme.IsDark || m.inboxVPLayout == before {
		t.Fatal("light background did not update theme and layout")
	}
	if got := m.theme.Styles.Title.Render("title"); !strings.Contains(got, "124;58;237") {
		t.Fatalf("light foreground not selected: %q", got)
	}
}

func TestInitRequestsBackground(t *testing.T) {
	m := scrollModel(t, 3)
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok || len(batch) != 4 {
		t.Fatalf("initial commands must include terminal background query: %T (%d)", batch, len(batch))
	}
	if !reflect.DeepEqual(batch[3](), tea.RequestBackgroundColor()) {
		t.Fatal("missing terminal background query")
	}
}

func TestColorschemeCancelPreservesTerminalAppearance(t *testing.T) {
	m := scrollModel(t, 3)
	m.theme = uitheme.New(uitheme.DarkDefault(false), colorprofile.TrueColor, false, false)
	m.mode, m.prevMode = ModeColorscheme, ModeInbox
	m.cfg.Theme = "dark"
	m = pressKey(m, "j")
	m = pressSpecial(m, tea.KeyEsc)
	if m.theme.IsDark || m.theme.Profile != colorprofile.TrueColor || m.theme.NoColor {
		t.Fatalf("cancel lost detected terminal appearance: %+v", m.theme)
	}
	if !strings.Contains(m.theme.Styles.Title.Render("title"), "124;58;237") {
		t.Fatal("cancel did not restore light palette")
	}
}

func TestColorProfilesRemainModelLocal(t *testing.T) {
	for _, profile := range []colorprofile.Profile{colorprofile.NoTTY, colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256, colorprofile.TrueColor} {
		t.Run(profile.String(), func(t *testing.T) {
			t.Parallel()
			m := scrollModel(t, 3)
			m.theme = uitheme.New(uitheme.DarkDefault(), colorprofile.TrueColor, false)
			original := m.theme.Styles.Title.Render("title")
			next, _ := m.Update(tea.ColorProfileMsg{Profile: profile})
			n := next.(Model)
			if n.theme.Profile != profile || n.theme.NoColor != (profile <= colorprofile.ASCII) {
				t.Fatal("profile was not applied")
			}
			if got := m.theme.Styles.Title.Render("title"); got != original {
				t.Fatal("profile update mutated another model")
			}
			if n.theme.NoColor && strings.Contains(n.theme.Styles.Title.Render("title"), "38;") {
				t.Fatal("no-color style contains foreground color")
			}
		})
	}
}
