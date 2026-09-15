package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInitAndLoad(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	path, err := Init(false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Refresh.Interval.Duration() != 2*time.Minute {
		t.Fatalf("interval = %s", cfg.Refresh.Interval.Duration())
	}
	if cfg.AI.Timeout.Duration() != 60*time.Second {
		t.Fatalf("timeout = %s", cfg.AI.Timeout.Duration())
	}
}

func TestInitAndSavePrivateModes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	path, err := Init(false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if mode := fileMode(t, filepath.Dir(path)); mode != 0o700 {
		t.Fatalf("config directory mode = %o, want 700", mode)
	}
	if mode := fileMode(t, path); mode != 0o600 {
		t.Fatalf("config file mode = %o, want 600", mode)
	}

	if err := os.Chmod(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("chmod config directory: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod config file: %v", err)
	}
	if err := Save(Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if mode := fileMode(t, filepath.Dir(path)); mode != 0o700 {
		t.Fatalf("corrected config directory mode = %o, want 700", mode)
	}
	if mode := fileMode(t, path); mode != 0o600 {
		t.Fatalf("corrected config file mode = %o, want 600", mode)
	}
}

func TestInitExistingConfigTightensModes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir config directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("theme: dark\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Init(false); err == nil {
		t.Fatal("Init existing config error = nil")
	}
	if mode := fileMode(t, filepath.Dir(path)); mode != 0o700 {
		t.Fatalf("config directory mode = %o, want 700", mode)
	}
	if mode := fileMode(t, path); mode != 0o600 {
		t.Fatalf("config file mode = %o, want 600", mode)
	}
}

func TestLoadDefaultsWhenMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Cache.Path == "" {
		t.Fatal("cache path empty")
	}
}

func TestLoadCreatesConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	if mode := fileMode(t, filepath.Dir(path)); mode != 0o700 {
		t.Fatalf("config directory mode = %o, want 700", mode)
	}
	if mode := fileMode(t, path); mode != 0o600 {
		t.Fatalf("config file mode = %o, want 600", mode)
	}
	if !reflect.DeepEqual(cfg.Actions, DefaultActions()) {
		t.Fatalf("actions = %#v, want defaults", cfg.Actions)
	}
}

func TestActionDefaultsSeparateFixedActions(t *testing.T) {
	if _, ok := DefaultActions()["move-down"]; ok {
		t.Fatal("move-down must be fixed")
	}
	if got := FixedActions()["move-down"].Keys; !reflect.DeepEqual(got, []string{"j", "down"}) {
		t.Fatalf("fixed move-down keys = %v", got)
	}
}

func TestLoadLegacyFixedActions(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"exact move-down", "actions:\n  move-down:\n    keys: [j, down]\n", ""},
		{"changed move-down key", "actions:\n  move-down:\n    keys: [J, down]\n", "actions.move-down is now built in; remove it from config"},
		{"added help command", "actions:\n  help:\n    commands: [help]\n", "actions.help is now built in; remove it from config"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			path, err := DefaultConfigPath()
			if err != nil {
				t.Fatalf("DefaultConfigPath: %v", err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(path, []byte(test.yaml), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := Load()
			if test.want != "" {
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("Load error = %v, want containing %q", err, test.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if _, ok := cfg.Actions["move-down"]; ok {
				t.Fatal("legacy move-down must not be configurable")
			}
		})
	}
}

func TestLoadOverlaysActionFields(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", dir)

	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := `actions:
  hello:
    keys: [H]
    commands: [hello]
    comment: hello world
  refresh-inbox:
    keys: [R]
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for name, want := range map[string]ActionConfig{
		"hello": {Keys: []string{"H"}, Commands: []string{"hello"}, Comment: "hello world"},
	} {
		got := cfg.Actions[name]
		if !reflect.DeepEqual(got.Keys, want.Keys) || !reflect.DeepEqual(got.Commands, want.Commands) || got.Comment != want.Comment {
			t.Errorf("%s = %#v, want %#v", name, got, want)
		}
		if got.Confirm == nil || !*got.Confirm {
			t.Errorf("%s confirm = %v, want true", name, got.Confirm)
		}
	}
	if got := cfg.Actions["refresh-inbox"]; !reflect.DeepEqual(got.Keys, []string{"R"}) || !reflect.DeepEqual(got.Commands, []string{"refresh-all"}) {
		t.Errorf("refresh-inbox = %#v, want keys [R] and commands [refresh-all]", got)
	}
}

func TestLoadRejectsInvalidAction(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"blank comment", "actions:\n  hello:\n    comment: '   '\n", "actions.hello.comment"},
		{"invalid command", "actions:\n  hello:\n    keys: [H]\n    commands: [bad name]\n    comment: hello\n", "actions.hello.commands"},
		{"conflicting command", "actions:\n  hello:\n    commands: [AI]\n    comment: hello\n", "ai-review"},
		{"no trigger", "actions:\n  hello:\n    comment: hello\n", "actions.hello"},
		{"leading command colon", "actions:\n  hello:\n    commands: [':hello']\n    comment: hello\n", "actions.hello.commands"},
		{"duplicate command case", "actions:\n  hello:\n    commands: [hello]\n    comment: hello\n  goodbye:\n    commands: [HELLO]\n    comment: goodbye\n", "hello"},
		{"builtin comment", "actions:\n  merge-pr:\n    comment: hello\n", "actions.merge-pr.comment"},
		{"invalid id", "actions:\n  Hello:\n    keys: [H]\n    comment: hello\n", "actions.Hello"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			path, err := DefaultConfigPath()
			if err != nil {
				t.Fatalf("DefaultConfigPath: %v", err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(path, []byte(test.yaml), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestSavePreservesActions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	cfg := Default()
	confirmed := true
	cfg.Actions["hello"] = ActionConfig{Keys: []string{"H"}, Commands: []string{"hello"}, Comment: "hello world", Confirm: &confirmed}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got.Actions, cfg.Actions) {
		t.Fatalf("actions = %#v, want %#v", got.Actions, cfg.Actions)
	}
}

func TestLoadParsesTrustAuthors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := "trust:\n  authors:\n    - alice\n    - bob\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if mode := fileMode(t, filepath.Dir(path)); mode != 0o700 {
		t.Fatalf("config directory mode after Load = %o, want 700", mode)
	}
	if mode := fileMode(t, path); mode != 0o600 {
		t.Fatalf("config file mode after Load = %o, want 600", mode)
	}
	if len(cfg.Trust.Authors) != 2 || cfg.Trust.Authors[0] != "alice" || cfg.Trust.Authors[1] != "bob" {
		t.Fatalf("trust authors = %v", cfg.Trust.Authors)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}
