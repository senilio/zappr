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
	if cfg.Refresh.Interval.Duration() != 30*time.Second {
		t.Fatalf("interval = %s", cfg.Refresh.Interval.Duration())
	}
	if _, ok := cfg.Actions["ai-review"]; ok {
		t.Fatal("default config contains AI review action")
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
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(data), "ai:") || strings.Contains(string(data), "trust:") {
		t.Fatalf("generated config contains removed settings: %q", data)
	}
	want := `refresh:
  interval: 30s
cache:
  path: ` + cfg.Cache.Path + `
sort: number
hide_drafts: true
actions:
  approve:
    operation: approve
    keys: [a]
    confirm: true
  approve-now:
    operation: approve
    keys: [A]
    confirm: false
  atlantis-apply:
    operation: comment
    commands: [aa]
    comment: atlantis apply
    confirm: true
  atlantis-cancel:
    operation: comment
    commands: [ac]
    comment: atlantis cancel
    confirm: true
  atlantis-plan:
    operation: comment
    keys: [P]
    commands: [ap]
    comment: atlantis plan
    confirm: false
  atlantis-unlock:
    operation: comment
    keys: [U]
    commands: [au]
    comment: atlantis unlock
    confirm: true
  close-pr:
    commands: [close]
  comment:
    operation: comment
    keys: [c]
  copy-pr-url:
    keys: [C]
  merge-pr:
    commands: [merge]
  open-diff-pager:
    keys: [V]
  open-pr:
    keys: [o]
  refresh-inbox:
    keys: [R]
  refresh-pr:
    keys: [r]
  request-changes:
    commands: [block]
  review:
    operation: comment
    commands: [review]
    comment: review
    confirm: true
  toggle-diff:
    keys: [v]
  toggle-draft:
    commands: [draft]
  toggle-file-content:
    keys: [x]
  toggle-side-by-side:
    keys: [s]
  update-branch:
    commands: [update]
`
	if string(data) != want {
		t.Fatalf("generated config:\n%s\nwant:\n%s", data, want)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("reload generated config: %v", err)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("reloaded config = %#v, want %#v", loaded, cfg)
	}
}

func TestActionDefaultsSeparateFixedActions(t *testing.T) {
	if _, ok := DefaultActions()["move-down"]; ok {
		t.Fatal("move-down must be fixed")
	}
	if got := FixedActions()["move-down"].Keys; !reflect.DeepEqual(got, []string{"down"}) {
		t.Fatalf("fixed move-down keys = %v", got)
	}
}

func TestLoadLegacyFixedActions(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"exact move-down", "actions:\n  move-down:\n    keys: [down]\n", ""},
		{"changed move-down key", "actions:\n  move-down:\n    keys: [j, down]\n", "actions.move-down is now built in; remove it from config"},
		{"null move-down keys", "actions:\n  move-down:\n    keys: null\n", "actions.move-down is now built in; remove it from config"},
		{"null move-down comment", "actions:\n  move-down:\n    comment: null\n", "actions.move-down is now built in; remove it from config"},
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

func TestLoadGenericActions(t *testing.T) {
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
  deploy:
    operation: comment
    keys: [H]
    comment: deploy preview
    confirm: false
    description: Deploy the selected PR
  approve-now:
    operation: approve
    confirm: false
  comment:
    operation: comment
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	deploy := cfg.Actions["deploy"]
	if deploy.Operation != "comment" || deploy.Comment != "deploy preview" || deploy.Description != "Deploy the selected PR" || deploy.Confirm == nil || *deploy.Confirm {
		t.Fatalf("deploy = %#v", deploy)
	}
	if approve := cfg.Actions["approve-now"]; approve.Operation != "approve" || approve.Confirm == nil || *approve.Confirm {
		t.Fatalf("approve-now = %#v", approve)
	}
	if approve := cfg.Actions["approve"]; approve.Operation != "approve" || approve.Confirm == nil || !*approve.Confirm {
		t.Fatalf("approve = %#v", approve)
	}
	if comment := cfg.Actions["comment"]; comment.Operation != "comment" || comment.Confirm != nil {
		t.Fatalf("comment = %#v", comment)
	}
}

func TestLoadRejectsInvalidGenericActions(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"missing operation", "actions:\n  hello:\n    keys: [h]\n", "actions.hello requires operation"},
		{"unknown operation", "actions:\n  hello:\n    operation: merge\n    keys: [h]\n", "actions.hello has unknown operation"},
		{"approve comment", "actions:\n  approve:\n    operation: approve\n    comment: nope\n    keys: [a]\n", "actions.approve approve actions cannot set comment"},
		{"editor confirm", "actions:\n  comment:\n    operation: comment\n    confirm: true\n    keys: [c]\n", "actions.comment editor comments cannot set confirm"},
		{"blank static", "actions:\n  hello:\n    operation: comment\n    comment: \"\"\n    keys: [h]\n", "actions.hello static comment cannot be blank"},
		{"no trigger", "actions:\n  hello:\n    operation: comment\n    comment: hello\n", "actions.hello: custom action requires a key or command"},
		{"invalid command", "actions:\n  hello:\n    operation: comment\n    keys: [H]\n    commands: [bad name]\n    comment: hello\n", "actions.hello.commands"},
		{"named builtin comment", "actions:\n  merge-pr:\n    comment: hello\n", "actions.merge-pr.comment"},
		{"named builtin operation", "actions:\n  merge-pr:\n    operation: approve\n", "actions.merge-pr.operation"},
		{"invalid id", "actions:\n  Hello:\n    operation: comment\n    keys: [H]\n    comment: hello\n", "actions.Hello"},
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

func TestLoadAllowsAtlantisActionNames(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", dir)

	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := "actions:\n  atlantis-plan:\n    operation: comment\n    comment: atlantis plan\n    commands: [ap]\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if action := cfg.Actions["atlantis-plan"]; action.Operation != "comment" || action.Comment != "atlantis plan" {
		t.Fatalf("atlantis-plan = %#v", action)
	}
}

func TestSavePreservesActions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	cfg := Default()
	confirmed := true
	cfg.Actions["hello"] = ActionConfig{Operation: "comment", Keys: []string{"H"}, Commands: []string{"hello"}, Comment: "hello world", Confirm: &confirmed, Description: "Say hello"}
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

func TestSaveRoundTripsHideDrafts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	cfg := Default()
	cfg.HideDrafts = true
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.HideDrafts {
		t.Fatal("HideDrafts = false, want true")
	}
}

func TestSaveGenericActions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	cfg := Default()
	confirmed := false
	cfg.Actions["deploy-preview"] = ActionConfig{
		Operation:   "comment",
		Keys:        []string{"P"},
		Commands:    []string{"preview"},
		Comment:     "deploy preview",
		Confirm:     &confirmed,
		Description: "Deploy preview",
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	if strings.Contains(string(data), "{") {
		t.Fatalf("generated config contains flow mappings: %q", data)
	}
	if strings.Contains(string(data), "next-file:") || strings.Contains(string(data), "prev-file:") {
		t.Fatalf("generated config contains file navigation actions: %q", data)
	}
	if strings.Contains(string(data), "move-down:") {
		t.Fatal("generated config contains fixed move-down action")
	}
	for _, field := range []string{"deploy-preview:\n", "operation: comment\n", "keys: [P]\n", "commands: [preview]\n", "comment: deploy preview\n", "confirm: false\n", "description: Deploy preview\n"} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("generated config missing %q: %q", field, data)
		}
	}
}

func TestDefaultGenericActions(t *testing.T) {
	actions := DefaultActions()
	for _, id := range []string{"approve", "approve-now", "comment"} {
		if _, ok := actions[id]; !ok {
			t.Fatalf("missing default action %q", id)
		}
	}
	for _, id := range []string{"approve-with-confirmation", "approve-immediately", "write-comment"} {
		if _, ok := actions[id]; ok {
			t.Fatalf("removed default action %q present", id)
		}
	}
}

func TestSaveRoundTripsEmptyActionAliases(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	cfg := Default()
	action := cfg.Actions["refresh-inbox"]
	action.Keys = []string{}
	cfg.Actions["refresh-inbox"] = action
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "refresh-inbox:\n    keys: []") {
		t.Fatalf("refresh-inbox = %q, want explicit empty keys", data)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if keys := got.Actions["refresh-inbox"].Keys; keys == nil || len(keys) != 0 {
		t.Fatalf("refresh-inbox keys = %#v, want empty slice", keys)
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

func actionLine(data, id string) string {
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), id+":") {
			return line
		}
	}
	return ""
}
