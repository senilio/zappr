package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIgnoresRemovedReaderBindings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{
		"reader-search", "reader-next-match", "reader-previous-match", "reader-source", "reader-return", "show-full-title",
		"comments-filter-author", "comments-clear-filters", "plan-next-resource", "plan-previous-resource",
		"plan-next-project", "plan-previous-project", "plan-toggle-project", "plan-toggle-value", "plan-expand-values",
		"plan-summary", "plan-next-report", "plan-previous-report",
	} {
		t.Run(id, func(t *testing.T) {
			body := "actions:\n  " + id + ": {keys: [z], commands: [" + id + "]}\n  reuse-key: {operation: comment, keys: [z], comment: hello}\n"
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := cfg.Actions[id]; exists || len(cfg.Actions["reuse-key"].Keys) != 1 {
				t.Fatal("removed binding survived or key could not be reused")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != body {
				t.Fatal("loading rewrote existing config")
			}
		})
	}
}
