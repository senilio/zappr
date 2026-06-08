package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	got := parseTime("2024-01-02T03:04:05Z")
	want := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parseTime = %s, want %s", got, want)
	}
}

func TestParseTimeInvalid(t *testing.T) {
	if !parseTime("nope").IsZero() {
		t.Fatal("invalid timestamp should return zero time")
	}
}

func TestApproveOmitsBodyWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "gh")
	script := "#!/bin/sh\nprintf '%s\n' \"$@\" > " + testShellQuote(argsPath) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	provider := &GHProvider{Bin: bin}
	if err := provider.Approve(context.Background(), "owner/repo", 123, ""); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	data, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"pr", "review", "123", "--repo", "owner/repo", "--approve"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func testShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
