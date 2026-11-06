package tui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/config"
	"go.yaml.in/yaml/v3"
)

// Skip network requests and config polling while exercising the real event loop.
type settingsTestProgram struct{ Model }

func (settingsTestProgram) Init() tea.Cmd { return nil }

func runSettingsProgram(t *testing.T, m Model, input string) Model {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p := tea.NewProgram(settingsTestProgram{m}, tea.WithContext(ctx), tea.WithInput(strings.NewReader(input)), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignals())
	next, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	return next.(Model)
}

func newSettingsModel(t *testing.T) Model {
	t.Helper()
	m, err := New(nil, "org/repo", 0, "", config.Default())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func assertSettingsFile(t *testing.T, m Model) {
	t.Helper()
	path, err := config.DefaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	got, err = yaml.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	want, err := yaml.Marshal(m.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("settings not persisted\ngot %s\nwant %s", got, want)
	}
}

func TestQuitDoesNotWriteSettings(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "missing"
		if existing {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			m := newSettingsModel(t)
			path, err := config.DefaultConfigPath()
			if err != nil {
				t.Fatal(err)
			}
			original := "# Keep user formatting and external edits.\nsort: updated\nhide_drafts: false\n"
			if existing {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}
			runSettingsProgram(t, m, "q")
			got, err := os.ReadFile(path)
			if !existing {
				if !os.IsNotExist(err) {
					t.Fatalf("quit created config: err=%v, data=%q", err, got)
				}
			} else if err != nil || string(got) != original {
				t.Fatalf("quit changed config: err=%v, data=%q", err, got)
			}
		})
	}
}

func TestSettingToggleThenImmediateQuitPersists(t *testing.T) {
	for _, setting := range []string{"wrap", "drafts", "sort", "theme"} {
		t.Run(setting, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			m := newSettingsModel(t)
			input := "\rq"
			switch setting {
			case "wrap":
				input = "wq"
			case "drafts":
				m.mode = ModeSettings
				input = "\rfq"
				// Hiding drafts avoids a network refresh in this test.
				m.hideDrafts, m.cfg.HideDrafts = false, false
			case "sort":
				m.mode, m.settingsCursor = ModeSettings, 1
				input = "\rfq"
			case "theme":
				m.mode, m.colorschemeIndex = ModeColorscheme, 1
			}
			before, err := yaml.Marshal(m.cfg)
			if err != nil {
				t.Fatal(err)
			}
			m = runSettingsProgram(t, m, input)
			after, err := yaml.Marshal(m.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) == string(after) {
				t.Fatal("setting did not change")
			}
			assertSettingsFile(t, m)
		})
	}
}

func TestQuitWaitsForPendingSettingsSaves(t *testing.T) {
	for _, newestFirst := range []bool{false, true} {
		name := "oldest-first"
		if newestFirst {
			name = "newest-first"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			m := newSettingsModel(t)
			next, first := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
			m = next.(Model)
			m.focus = FocusSummary
			next, second := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
			m = next.(Model)
			if first == nil || second == nil {
				t.Fatal("missing save commands")
			}
			revision := m.settingsSaver.revision.Load()
			// Repeated quit requests must not schedule additional writes.
			for range 2 {
				next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
				m = next.(Model)
				if cmd != nil || m.settingsSaver.revision.Load() != revision {
					t.Fatal("quit scheduled work instead of waiting for pending saves")
				}
			}
			if newestFirst {
				first, second = second, first
			}
			next, cmd := m.Update(first())
			m = next.(Model)
			if cmd != nil {
				t.Fatal("quit before all save results arrived")
			}
			next, cmd = m.Update(second())
			m = next.(Model)
			if cmd == nil {
				t.Fatal("did not quit after all saves completed")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("final save did not trigger quit")
			}
			assertSettingsFile(t, m)
		})
	}
}

func TestSaveFailureCancelsPendingQuit(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocked)
	m := newSettingsModel(t)
	next, save := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(Model)
	if save == nil {
		t.Fatal("missing save command")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("quit did not wait for pending save")
	}
	msg := save().(settingsSavedMsg)
	if msg.err == nil {
		t.Fatal("expected save failure")
	}
	next, _ = m.Update(msg)
	m = next.(Model)
	if !m.statusErr || !strings.Contains(m.error, "failed to save settings") {
		t.Fatalf("save failure not reported: %q", m.error)
	}
	// A later successful save must not silently resume the canceled quit.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	next, save = m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(Model)
	if save == nil {
		t.Fatal("missing retry save command")
	}
	next, cmd = m.Update(save())
	if cmd != nil || next.(Model).error != m.error {
		t.Fatal("save success resumed a canceled quit or cleared the error")
	}
	assertSettingsFile(t, next.(Model))
}
