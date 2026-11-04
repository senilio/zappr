package tui

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/config"
	"github.com/senilio/zappr/internal/model"
)

func TestReloadCommandIsFixedAndPRIndependent(t *testing.T) {
	r, err := newActionRegistry(config.Default().Actions)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := r.commands["reload"]
	if !ok || a.ID != "reload" || a.RequiresPR || a.Confirmation != confirmationNone {
		t.Fatalf("reload unavailable or requires PR/confirmation: %#v", a)
	}
}

func reloadFixture(t *testing.T) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.Default()
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	m, _ := configuredModel(t, cfg, false)
	m.loading = false
	return m
}

func TestReloadAppliesRuntimeSettings(t *testing.T) {
	m := reloadFixture(t)
	m.focus = FocusDiff
	cfg := m.cfg
	cfg.Theme, cfg.Pills, cfg.Sort = "nord", "square", "updated"
	opacity, disabled := 0.3, false
	cfg.PillOpacity, cfg.Atlantis.Enabled = &opacity, &disabled
	cfg.Wrap = config.WrapConfig{Inbox: true, Summary: true, Diff: true}
	cfg.Cache.Path = "new.db"
	cfg.Actions = config.DefaultActions()
	a := cfg.Actions["toggle-wrap"]
	a.Keys = []string{"W"}
	cfg.Actions["toggle-wrap"] = a
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	next, _ := m.update(m.reloadConfig()())
	m = next.(Model)
	if m.cfg.Theme != "nord" || m.theme.Palette.Name != "nord" || m.cfg.Pills != "square" || !m.cfg.Wrap.Diff || m.cfg.EffectivePillOpacity() != 0.3 || m.atlantisEnabled() {
		t.Fatalf("runtime settings not applied: %#v", m.cfg)
	}
	if m.focus != FocusDiff {
		t.Fatal("focus changed")
	}
	if _, ok := m.actions.actionForKey(actionContextNormal, "W"); !ok {
		t.Fatal("new binding missing")
	}
	if _, ok := m.actions.actionForKey(actionContextNormal, "w"); ok {
		t.Fatal("old binding remains")
	}
	if !strings.Contains(m.message, "restart") {
		t.Fatalf("missing cache warning: %s", m.message)
	}
	m.saveSettings()()
	saved, err := config.Reload()
	if err != nil || saved.Cache.Path != cfg.Cache.Path {
		t.Fatalf("requested cache path lost: %v", err)
	}
}

func TestReloadRejectsInvalidCandidateAtomically(t *testing.T) {
	for _, contents := range []string{"theme: [", "actions:\n  quit:\n    keys: []\n", "refresh:\n  interval: -1s\n"} {
		t.Run(contents, func(t *testing.T) {
			m := reloadFixture(t)
			before, theme, actions := m.cfg, m.theme, m.actions
			path, _ := config.DefaultConfigPath()
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			next, _ := m.update(m.reloadConfig()())
			m = next.(Model)
			if !reflect.DeepEqual(m.cfg, before) || !reflect.DeepEqual(m.theme, theme) || !reflect.DeepEqual(m.actions, actions) || m.error == "" {
				t.Fatal("invalid candidate replaced live state or error missing")
			}
		})
	}
}

func TestReloadOrdering(t *testing.T) {
	m := reloadFixture(t)
	oldSave := m.saveSettings()
	cfg := m.cfg
	cfg.Theme = "nord"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	first := m.reloadConfig()()
	oldSave()
	loaded, err := config.Reload()
	if err != nil || loaded.Theme != "nord" {
		t.Fatal("queued save overwrote disk")
	}
	second := m.reloadConfig()()
	next, _ := m.update(second)
	m = next.(Model)
	m.cfg.Theme = "local"
	m.saveSettings()
	next, cmd := m.update(first)
	m = next.(Model)
	if m.cfg.Theme != "local" || cmd != nil {
		t.Fatal("stale result applied")
	}
	third := m.reloadConfig()()
	m.cfg.Wrap.Diff = true
	m.saveSettings()
	next, _ = m.update(third)
	if !next.(Model).cfg.Wrap.Diff {
		t.Fatal("reload overwrote newer preference")
	}
}

func TestReloadMissingFileKeepsSettings(t *testing.T) {
	m := reloadFixture(t)
	before := m.cfg
	path, _ := config.DefaultConfigPath()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	next, _ := m.update(m.reloadConfig()())
	m = next.(Model)
	if !reflect.DeepEqual(m.cfg, before) || m.error == "" {
		t.Fatal("missing file did not retain state")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("reload created missing file")
	}
}

func TestReloadPreservesSelectionAndReconcilesDraft(t *testing.T) {
	m := reloadFixture(t)
	m.prs = []model.PullRequest{{Number: 1, Repo: "org/repo", Draft: true}, {Number: 2, Repo: "org/repo"}}
	m.allPRs = m.prs
	m.cfg.HideDrafts, m.hideDrafts = false, false
	m.rawDiff, m.diff = "old PR", "old PR"
	cfg := m.cfg
	cfg.HideDrafts = true
	actions, _ := newActionRegistry(cfg.Actions)
	m.applyReloadedConfig(cfg, actions)
	if m.prs[m.selected].Number != 2 || m.diff != "" || m.detailsLoadKey == "" {
		t.Fatal("filtered selection retained stale view or failed to load replacement")
	}
	m.applyReloadedConfig(cfg, actions)
	if m.prs[m.selected].Number != 2 {
		t.Fatal("selection changed")
	}
}

func TestReloadPollingGeneration(t *testing.T) {
	m := reloadFixture(t)
	oldGeneration := m.pollGeneration
	cfg := m.cfg
	cfg.Refresh.Interval = config.Duration(time.Minute)
	actions, _ := newActionRegistry(cfg.Actions)
	m.applyReloadedConfig(cfg, actions)
	if m.pollEvery != time.Minute || m.pollGeneration != oldGeneration+1 {
		t.Fatal("changed interval did not replace timer generation")
	}
	next, cmd := m.update(tickMsg{generation: oldGeneration})
	if cmd != nil || next.(Model).loading {
		t.Fatal("old tick fetched or rescheduled")
	}
	m.applyReloadedConfig(cfg, actions)
	if m.pollGeneration != oldGeneration+1 {
		t.Fatal("unchanged interval replaced timer")
	}
	for _, loading := range []bool{true, false} {
		m.loading = loading
		_, cmd := m.update(tickMsg{generation: m.pollGeneration})
		if cmd == nil {
			t.Fatal("current generation did not schedule polling")
		}
	}
}

func TestReloadDisablingAtlantisClosesPlan(t *testing.T) {
	m := planModel(t)
	m.enterPlan()
	cfg := m.cfg
	disabled := false
	cfg.Atlantis.Enabled = &disabled
	actions, _ := newActionRegistry(cfg.Actions)
	m.applyReloadedConfig(cfg, actions)
	if m.summaryMode == SummaryModePlan || m.planState.doc != nil {
		t.Fatal("disabled Atlantis retained active plan reader")
	}
}

func TestReloadRejectsOldDraftFilteredRefresh(t *testing.T) {
	m := reloadFixture(t)
	old := refreshMsg{prs: []model.PullRequest{{Number: 1, Repo: "org/repo"}}}
	cfg := m.cfg
	cfg.HideDrafts = false
	actions, _ := newActionRegistry(cfg.Actions)
	m.applyReloadedConfig(cfg, actions)
	draft := model.PullRequest{Number: 2, Repo: "org/repo", Draft: true}
	next, _ := m.update(refreshMsg{prs: append(old.prs, draft), generation: m.refreshGeneration})
	m = next.(Model)
	if len(m.prs) != 2 {
		t.Fatal("current refresh did not show drafts")
	}
	next, _ = m.update(old)
	if len(next.(Model).prs) != 2 {
		t.Fatal("older filtered refresh removed newly visible drafts")
	}
}

func TestReloadUnchangedPreservesScrollOffsets(t *testing.T) {
	m := reloadFixture(t)
	m.width, m.height = 40, 10
	m.cfg.Wrap.Inbox, m.cfg.Wrap.Summary = true, true
	m.prs = []model.PullRequest{{Number: 1, Repo: "org/repo", Title: strings.Repeat("long title ", 40), Commits: []model.PRCommit{{OID: "abcdef0", Headline: strings.Repeat("long headline ", 40)}}}}
	m.allPRs = m.prs
	m.summaryMode = SummaryModeCommits
	m.resizeViewports()
	m.inboxVP.SetYOffset(2)
	m.summaryVP.SetYOffset(2)
	if m.inboxVP.YOffset() != 2 || m.summaryVP.YOffset() != 2 {
		t.Fatal("fixture does not allow scrolling")
	}
	actions, _ := newActionRegistry(m.cfg.Actions)
	m.applyReloadedConfig(m.cfg, actions)
	if m.inboxVP.YOffset() != 2 || m.summaryVP.YOffset() != 2 {
		t.Fatalf("unchanged reload moved scroll: inbox=%d summary=%d", m.inboxVP.YOffset(), m.summaryVP.YOffset())
	}
}
