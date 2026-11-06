package tui

import (
	"os"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/config"
)

func writeWatchedConfig(t *testing.T, content string) string {
	t.Helper()
	path, err := config.DefaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func watchCheck(t *testing.T, m *Model) configWatchCheckedMsg {
	t.Helper()
	msg := m.checkConfig()().(configWatchCheckedMsg)
	next, cmd := m.update(msg)
	*m = next.(Model)
	if cmd == nil {
		t.Fatal("watch chain stopped")
	}
	return msg
}

func TestConfigWatchChangesAndUnchanged(t *testing.T) {
	m := reloadFixture(t)
	if msg := watchCheck(t, &m); msg.result != nil {
		t.Fatal("unchanged file reloaded")
	}
	writeWatchedConfig(t, "theme: nord\nactions:\n  reload:\n    keys: [ctrl+r]\n")
	if msg := watchCheck(t, &m); msg.result == nil || msg.result.err != nil {
		t.Fatalf("missing reload: %#v", msg)
	}
	if m.cfg.Theme != "nord" {
		t.Fatal("theme not applied")
	}
	if _, ok := m.actions.actionForKey(actionContextNormal, "ctrl+r"); !ok {
		t.Fatal("binding not applied")
	}
	if msg := watchCheck(t, &m); msg.result != nil {
		t.Fatal("unchanged file reloaded again")
	}
}

func TestConfigWatchEditBeforeModelConstruction(t *testing.T) {
	for _, content := range []string{"theme: nord\n", "theme: [", "missing"} {
		t.Run(content, func(t *testing.T) {
			initial := reloadFixture(t)
			loaded := initial.cfg
			path := writeWatchedConfig(t, content)
			if content == "missing" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			m, _ := configuredModel(t, loaded, false)
			msg := watchCheck(t, &m)
			if msg.result == nil {
				t.Fatal("startup edit consumed without applying or reporting it")
			}
			if content == "theme: nord\n" {
				if msg.result.err != nil || m.cfg.Theme != "nord" {
					t.Fatal("startup edit not applied")
				}
			} else if msg.result.err == nil || m.error == "" || !reflect.DeepEqual(m.cfg, loaded) {
				t.Fatal("startup failure not reported with settings preserved")
			}
		})
	}
}

func TestConfigWatchSameMetadataAndAtomicReplacement(t *testing.T) {
	m := reloadFixture(t)
	path := writeWatchedConfig(t, "theme: nord\n")
	watchCheck(t, &m)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	writeWatchedConfig(t, "theme: gruv\n")
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	watchCheck(t, &m)
	if m.cfg.Theme != "gruv" {
		t.Fatal("same-size/timestamp edit missed")
	}
	if err := os.WriteFile(path+".new", []byte("theme: nord\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".new", path); err != nil {
		t.Fatal(err)
	}
	watchCheck(t, &m)
	if m.cfg.Theme != "nord" {
		t.Fatal("atomic replacement missed")
	}
}

func TestConfigWatchFailuresAndRecovery(t *testing.T) {
	for _, content := range []string{"theme: [", "actions:\n  quit:\n    keys: []\n", "refresh:\n  interval: -1s\n", "missing", "directory"} {
		t.Run(content, func(t *testing.T) {
			m := reloadFixture(t)
			before := m.cfg
			path, _ := config.DefaultConfigPath()
			switch content {
			case "missing", "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if content == "directory" {
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				}
			default:
				writeWatchedConfig(t, content)
			}
			msg := watchCheck(t, &m)
			if msg.result == nil || msg.result.err == nil || m.error == "" || !reflect.DeepEqual(m.cfg, before) {
				t.Fatal("failed candidate changed live settings or error missing")
			}
			if content == "missing" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("watch recreated config")
				}
			}
			if msg := watchCheck(t, &m); msg.result != nil {
				t.Fatal("repeated failure reported")
			}
			if content == "directory" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			writeWatchedConfig(t, "theme: nord\n")
			watchCheck(t, &m)
			if m.cfg.Theme != "nord" || m.error != "" {
				t.Fatal("watch did not recover")
			}
		})
	}
}

func TestConfigWatchSaveAndReloadOrdering(t *testing.T) {
	m := reloadFixture(t)
	m.cfg.Wrap.Diff = true
	save := m.saveSettings()
	watchCheck(t, &m)
	save()
	saved, err := config.Reload()
	if err != nil || !saved.Wrap.Diff {
		t.Fatal("unchanged check superseded queued save")
	}
	if msg := watchCheck(t, &m); msg.result != nil {
		t.Fatal("in-app save reloaded")
	}
	writeWatchedConfig(t, "theme: nord\n")
	stale := m.checkConfig()()
	m.cfg.Wrap.Summary = true
	m.saveSettings() // supersedes result without executing the write
	next, _ := m.update(stale)
	m = next.(Model)
	if m.cfg.Theme == "nord" || !m.cfg.Wrap.Summary {
		t.Fatal("stale check overwrote preferences")
	}
	if msg := watchCheck(t, &m); msg.result == nil || m.cfg.Theme != "nord" {
		t.Fatal("unapplied content was consumed")
	}
	writeWatchedConfig(t, "theme: gruv\n")
	next, _ = m.update(m.reloadConfig()())
	m = next.(Model)
	if msg := watchCheck(t, &m); msg.result != nil {
		t.Fatal("manual reload triggered automatic reload")
	}
}

func TestConfigWatchTickStartsCheck(t *testing.T) {
	m := reloadFixture(t)
	_, cmd := m.update(configWatchTickMsg{})
	if cmd == nil {
		t.Fatal("tick did not check config")
	}
	if _, ok := cmd().(configWatchCheckedMsg); !ok {
		t.Fatal("tick returned wrong message")
	}
	m.pollEvery = -1
	batch := m.Init()().(tea.BatchMsg)
	if len(batch) < 5 {
		t.Fatal("Init did not schedule config watcher")
	}
}

func TestConfigWatchChangedFileSupersedesQueuedSave(t *testing.T) {
	m := reloadFixture(t)
	save := m.saveSettings()
	writeWatchedConfig(t, "theme: nord\n")
	watchCheck(t, &m)
	save()
	cfg, err := config.Reload()
	if err != nil || cfg.Theme != "nord" {
		t.Fatal("queued save overwrote automatically loaded config")
	}
}
