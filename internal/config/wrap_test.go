package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWrapConfigRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	path, _ := DefaultConfigPath()
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "inbox: false") {
		t.Fatalf("default wrap preferences missing: %s", data)
	}
	if err := os.WriteFile(path, []byte("theme: light\nsort: updated\nhide_drafts: false\nwrap:\n  inbox: true\n  summary: true\n  diff: true\nactions:\n  toggle-wrap:\n    keys: [W]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if err = Save(cfg); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Clean(path))
	for _, want := range []string{"inbox: true", "summary: true", "diff: true", "theme: light", "sort: updated", "hide_drafts: false", "keys: [W]"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("round trip missing %q", want)
		}
	}
}
