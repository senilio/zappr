package config

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReloadMissingDoesNotCreateConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := Reload(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Reload error = %v, want not-exist", err)
	}
	path, _ := DefaultConfigPath()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reload created config: %v", err)
	}
}

func TestReloadUsesLoadParsing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := Init(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, data, wantError string }{
		{"valid", "theme: nord\nwrap:\n  inbox: true\n", ""},
		{"yaml", "wrap: [", "parse config"},
		{"action", "actions:\n  custom:\n    operation: destroy\n", "unknown operation"},
		{"reserved command", "actions:\n  approve:\n    commands: [reload]\n", "conflicts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Reload()
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Theme != "nord" || !cfg.Wrap.Inbox || cfg.Refresh.Interval.Duration() != 30*time.Second || len(cfg.Actions) == 0 {
				t.Fatalf("defaults or overrides lost: %#v", cfg)
			}
		})
	}
}

func TestReloadActionConfigurable(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := Init(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, commands := range []string{"", "\n    commands: [reload-config]"} {
		if err := os.WriteFile(path, []byte("actions:\n  reload:\n    keys: [ctrl+r]"+commands+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Reload()
		if err != nil {
			t.Fatal(err)
		}
		a := cfg.Actions["reload"]
		wantCommand := "reload"
		if commands != "" {
			wantCommand = "reload-config"
		}
		if len(a.Keys) != 1 || a.Keys[0] != "ctrl+r" || len(a.Commands) != 1 || a.Commands[0] != wantCommand {
			t.Fatalf("reload action = %#v", a)
		}
	}
}
