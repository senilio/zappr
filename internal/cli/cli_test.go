package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWritablePrivateModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	path := filepath.Join(dir, "zappr.db")

	if err := ensureWritable(path); err != nil {
		t.Fatalf("ensureWritable: %v", err)
	}
	if mode := writableFileMode(t, dir); mode != 0o700 {
		t.Fatalf("cache directory mode = %o, want 700", mode)
	}
	if mode := writableFileMode(t, path); mode != 0o600 {
		t.Fatalf("cache file mode = %o, want 600", mode)
	}

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod cache directory: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod cache file: %v", err)
	}
	if err := ensureWritable(path); err != nil {
		t.Fatalf("ensureWritable existing file: %v", err)
	}
	if mode := writableFileMode(t, dir); mode != 0o700 {
		t.Fatalf("corrected cache directory mode = %o, want 700", mode)
	}
	if mode := writableFileMode(t, path); mode != 0o600 {
		t.Fatalf("corrected cache file mode = %o, want 600", mode)
	}
}

func TestUsageTextSeparatesRefreshAndAIReview(t *testing.T) {
	text := usageText()
	if !strings.Contains(text, "r refresh") || !strings.Contains(text, ":ai AI review") {
		t.Fatalf("usage text = %q, want separate refresh and AI review commands", text)
	}
	if strings.Contains(text, "r refresh/AI review") {
		t.Fatalf("usage text = %q, contains stale combined shortcut", text)
	}
}

func writableFileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}
