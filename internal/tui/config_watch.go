package tui

import (
	"crypto/sha256"
	"os"
	"reflect"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/config"
)

type configObservation struct {
	digest [sha256.Size]byte
	err    string
}

func observeConfig(data []byte, err error) configObservation {
	observation := configObservation{digest: sha256.Sum256(data)}
	if err != nil {
		observation.err = err.Error()
	}
	return observation
}

func newSettingsSaver(cfg config.Config) *settingsSaver {
	saver := &settingsSaver{}
	path, err := config.DefaultConfigPath()
	if err != nil {
		return saver
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return saver
	}
	// A disk read newer than the startup load is not yet accepted by the UI.
	// Only seed the observation when it represents the loaded settings.
	loaded, err := config.Parse(data)
	if err == nil && reflect.DeepEqual(loaded, cfg) {
		saver.observed = observeConfig(data, nil)
	}
	return saver
}

type configWatchTickMsg struct{}
type configWatchCheckedMsg struct{ result *configReloadedMsg }

func configWatchTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return configWatchTickMsg{} })
}

func (m *Model) checkConfig() tea.Cmd {
	if m.settingsSaver == nil {
		m.settingsSaver = newSettingsSaver(m.cfg)
	}
	saver := m.settingsSaver
	revision := saver.revision.Load()
	return func() tea.Msg {
		saver.mu.Lock()
		defer saver.mu.Unlock()
		if revision != saver.revision.Load() {
			return configWatchCheckedMsg{}
		}
		msg := readConfig(revision)
		if msg.observed == saver.observed {
			return configWatchCheckedMsg{}
		}
		// Changed files supersede older queued saves just like manual reloads.
		// A preference queued during the read wins instead.
		if !saver.revision.CompareAndSwap(revision, revision+1) {
			return configWatchCheckedMsg{}
		}
		msg.revision = revision + 1
		return configWatchCheckedMsg{result: &msg}
	}
}

func (m *Model) acceptConfigReload(msg configReloadedMsg) tea.Cmd {
	if m.settingsSaver == nil {
		return nil
	}
	saver := m.settingsSaver
	saver.mu.Lock()
	if msg.revision != saver.revision.Load() {
		saver.mu.Unlock()
		return nil
	}
	// Only acknowledge observations accepted by the UI. A delayed result must
	// not consume an external edit that a newer preference has superseded.
	saver.observed = msg.observed
	saver.mu.Unlock()
	if msg.err != nil {
		m.error = "failed to reload configuration: " + msg.err.Error()
		return m.finishStatus(m.error, true)
	}
	return m.applyReloadedConfig(msg.cfg, msg.actions)
}
