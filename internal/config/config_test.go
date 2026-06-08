package config

import (
	"os"
	"path/filepath"
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
	if len(cfg.Trust.Authors) != 2 || cfg.Trust.Authors[0] != "alice" || cfg.Trust.Authors[1] != "bob" {
		t.Fatalf("trust authors = %v", cfg.Trust.Authors)
	}
}
