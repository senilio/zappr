package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "zappr-tui-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "isolate TUI tests:", err)
		os.Exit(1)
	}
	for key, dir := range map[string]string{"XDG_CONFIG_HOME": "config", "XDG_CACHE_HOME": "cache"} {
		if err := os.Setenv(key, filepath.Join(root, dir)); err != nil {
			fmt.Fprintln(os.Stderr, "isolate TUI tests:", err)
			_ = os.RemoveAll(root)
			os.Exit(1)
		}
	}
	code := m.Run()
	if err := os.RemoveAll(root); err != nil {
		fmt.Fprintln(os.Stderr, "clean up TUI tests:", err)
		code = 1
	}
	os.Exit(code)
}
