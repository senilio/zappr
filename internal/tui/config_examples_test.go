package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/senilio/zappr/internal/config"
)

func TestREADMEConfigExamples(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	blocks := strings.Split(string(data), "```yaml\n")[1:]
	if len(blocks) == 0 {
		t.Fatal("README contains no YAML examples")
	}
	for i, block := range blocks {
		t.Run(fmt.Sprint(i+1), func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			path, err := config.DefaultConfigPath()
			if err != nil {
				t.Fatalf("DefaultConfigPath: %v", err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatalf("create config directory: %v", err)
			}
			example, _, ok := strings.Cut(block, "```")
			if !ok {
				t.Fatal("unclosed YAML example")
			}
			if err := os.WriteFile(path, []byte(example), 0o600); err != nil {
				t.Fatalf("write example config: %v", err)
			}
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("load example config: %v", err)
			}
			if _, err := newActionRegistry(cfg.Actions); err != nil {
				t.Fatalf("validate example actions: %v", err)
			}
		})
	}
}
