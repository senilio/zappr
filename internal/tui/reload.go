package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/config"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

type configReloadedMsg struct {
	cfg      config.Config
	actions  actionRegistry
	revision uint64
	err      error
	observed configObservation
}

func readConfig(revision uint64) configReloadedMsg {
	data, err := config.ReadSnapshot()
	msg := configReloadedMsg{revision: revision, observed: observeConfig(data, err), err: err}
	if msg.err == nil {
		msg.cfg, msg.err = config.Parse(data)
	}
	if msg.err == nil && time.Duration(msg.cfg.Refresh.Interval) < 0 {
		msg.err = fmt.Errorf("refresh.interval must not be negative")
	}
	if msg.err == nil {
		msg.actions, msg.err = newActionRegistry(msg.cfg.Actions)
	}
	return msg
}

func (m *Model) reloadConfig() tea.Cmd {
	if m.settingsSaver == nil {
		m.settingsSaver = newSettingsSaver(m.cfg)
	}
	saver := m.settingsSaver
	revision := saver.revision.Add(1)
	return func() tea.Msg {
		saver.mu.Lock()
		defer saver.mu.Unlock()
		if revision != saver.revision.Load() {
			return configReloadedMsg{revision: revision}
		}
		return readConfig(revision)
	}
}

func (m *Model) applyReloadedConfig(cfg config.Config, actions actionRegistry) tea.Cmd {
	inbox, summary, diff := m.inboxAnchor(), m.summaryAnchor(), m.diffAnchor()
	inboxOffset, summaryOffset, diffOffset := m.inboxVP.YOffset(), m.summaryVP.YOffset(), m.diffVP.YOffset()
	previousWrap, previousSummaryMode := m.cfg.Wrap, m.summaryMode
	keepInboxOffset := previousWrap.Inbox == cfg.Wrap.Inbox && m.cfg.Sort == cfg.Sort && m.hideDrafts == cfg.HideDrafts
	previous, hadPrevious := m.current()
	showDrafts := m.hideDrafts && !cfg.HideDrafts
	if m.hideDrafts != cfg.HideDrafts {
		// Ignore responses fetched under the previous visibility policy.
		m.refreshGeneration++
		m.loading = false
	}
	cachePath := m.cfg.Cache.Path
	if m.app != nil && m.app.Cache != nil {
		cachePath = m.app.Cache.Path()
	}
	if m.cfg.Wrap.Summary != cfg.Wrap.Summary {
		m.summaryXOffset = 0
	}
	if m.cfg.Wrap.Diff != cfg.Wrap.Diff {
		m.diffXOffset = 0
	}
	m.cfg, m.actions = cfg, actions
	m.sortMode, m.hideDrafts = app.ParseSortMode(cfg.Sort), cfg.HideDrafts
	m.theme = uitheme.New(uitheme.PaletteByName(cfg.Theme, m.theme.IsDark), m.theme.Profile, m.theme.NoColor, m.theme.IsDark)
	var poll tea.Cmd
	if interval := time.Duration(cfg.Refresh.Interval); interval != m.pollEvery {
		m.pollEvery = interval
		m.pollGeneration++
		poll = tickCmd(m.pollEvery, m.pollGeneration)
	}
	m.applyPRSettings()
	m.reconcilePlan()
	m.resizeViewports()
	m.restoreInboxAnchor(inbox)
	if keepInboxOffset {
		m.inboxVP.SetYOffset(inboxOffset)
	}
	current, hasCurrent := m.current()
	var selection tea.Cmd
	if hadPrevious == hasCurrent && previous.Repo == current.Repo && previous.Number == current.Number {
		m.restoreSummaryAnchor(summary)
		m.restoreDiffAnchor(diff)
		if previousSummaryMode == m.summaryMode {
			if previousWrap.Summary == cfg.Wrap.Summary {
				m.summaryVP.SetYOffset(summaryOffset)
			}
			if previousWrap.Diff == cfg.Wrap.Diff {
				m.diffVP.SetYOffset(diffOffset)
			}
		}
	} else {
		selection = m.selectionCmd()
	}
	m.error = ""
	m.message = "configuration reloaded"
	if cfg.Cache.Path != cachePath {
		m.message += "; cache.path requires restart"
	}
	var refresh tea.Cmd
	if showDrafts {
		refresh = m.refreshCmd(true)
	}
	return tea.Batch(selection, refresh, poll, m.finishStatus(m.message, false))
}
