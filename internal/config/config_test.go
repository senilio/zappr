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
