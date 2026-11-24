package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/model"
)

type inboxReadFailStore struct{ cache.Store }

func (inboxReadFailStore) ListRepo(string) ([]model.PullRequest, error) {
	return nil, errors.New("cache read failed")
}

func TestInboxRefreshStatusAndAge(t *testing.T) {
	m, provider, a, b := queuedPlanModel(t, true)
	m.width, m.height = 180, 30
	m, cmd := keyUpdate(m, "R")
	if header, title := stripANSI(m.headerView()), stripANSI(m.inboxTitle()); !strings.Contains(title, "refreshing") ||
		strings.Contains(header, "loading") || strings.Contains(header, "refreshing") {
		t.Fatalf("header = %q, Inbox = %q, want refreshing only in the Inbox", header, title)
	}
	m = deliver(m, commandResult[refreshMsg](t, cmd))
	if m.lastInboxRefresh.IsZero() || strings.Contains(m.inboxTitle(), "refreshing") {
		t.Fatalf("full success did not settle Inbox: %q at %v", m.inboxTitle(), m.lastInboxRefresh)
	}
	lastSuccess := m.lastInboxRefresh
	if label := m.inboxRefreshLabel(lastSuccess.Add(42 * time.Second)); label != "updated 42s ago" {
		t.Fatalf("Inbox age = %q", label)
	}
	m = deliver(m, commandResult[refreshPRMsg](t, refreshPRCmd(m.app, a)))
	m, cmd = keyUpdate(m, "r")
	m = deliver(m, commandResult[prActionResult](t, cmd))
	m, cmd = keyUpdate(m, "P")
	m = deliver(m, commandResult[prActionResult](t, cmd))
	m = deliver(m, commandResult[closePRMsg](t, closePRCmd(m.app, b, "", 0, 0)))
	if m.lastInboxRefresh != lastSuccess {
		t.Fatalf("targeted work advanced full refresh age: %v -> %v", lastSuccess, m.lastInboxRefresh)
	}

	provider.listedPRs = []model.PullRequest{a}
	m.app.Cache = inboxReadFailStore{m.app.Cache}
	m, cmd = keyUpdate(m, "R")
	msg := commandResult[refreshMsg](t, cmd)
	if msg.err == nil {
		t.Fatal("real App refresh did not report the cache error")
	}
	m = deliver(m, msg)
	if len(m.prs) != 1 || m.prs[0].Number != a.Number || m.lastInboxRefresh != lastSuccess {
		t.Fatalf("failed refresh changed displayed rows or age: rows=%+v, last=%v", m.prs, m.lastInboxRefresh)
	}
	if title, header := stripANSI(m.inboxTitle()), stripANSI(m.headerView()); !strings.Contains(title, "refresh failed") ||
		!strings.Contains(title, "cache read failed") || strings.Contains(header, "cache read failed") {
		t.Fatalf("failed refresh: Inbox = %q, header = %q", title, header)
	}
	m.width = 0
	if view := stripANSI(m.inboxView()); !strings.Contains(view, "cache read failed") || !strings.Contains(view, "updated ") {
		t.Fatalf("plain Inbox lost the error or age: %q", view)
	}
}

func TestInboxRefreshIgnoresSupersededResult(t *testing.T) {
	for _, failed := range []bool{false, true} {
		m, _, _, b := observationModel(t)
		m.width = 130
		m.lastInboxRefresh = timeNow().Add(-time.Minute)
		lastSuccess := m.lastInboxRefresh
		m.startStatus("fetching diff…")
		m, cmd := keyUpdate(m, "R")
		old := commandResult[refreshMsg](t, cmd)
		if failed {
			old.err = errors.New("obsolete failure")
		}
		b.Title = "obsolete row"
		old.updates = []app.PRUpdate{testPRUpdate(b, old.id)}
		m, cmd = keyUpdate(m, "R")
		current := commandResult[refreshMsg](t, cmd)
		m = deliver(m, old)
		if m.lastInboxRefresh != lastSuccess || len(m.prs) != 2 || !strings.Contains(m.inboxTitle(), "refreshing") ||
			strings.Contains(m.inboxTitle(), "obsolete") || m.statusText != "fetching diff…" {
			t.Fatalf("obsolete result affected Inbox or header: %q, %q", m.inboxTitle(), m.statusText)
		}
		for _, pr := range m.prs {
			if pr.Title == b.Title {
				t.Fatal("obsolete row applied")
			}
		}
		m = deliver(m, current)
		if !m.lastInboxRefresh.After(lastSuccess) || m.statusText != "fetching diff…" || len(m.prs) != 2 {
			t.Fatalf("latest full success affected unrelated status or did not advance age: header=%q age=%v", m.statusText, m.lastInboxRefresh)
		}
	}
}

func TestHeaderLoadStatusOwnership(t *testing.T) {
	fixture := func(t *testing.T) Model {
		m, _, a, _ := observationModel(t)
		a.Files = []model.ChangedFile{{Path: "main.go"}}
		m.prs[0], m.allPRs[0] = a, a
		m.width, m.height = 60, 24
		return m
	}
	for _, kind := range []string{"diff", "file"} {
		t.Run(kind+" success and failure", func(t *testing.T) {
			for _, failed := range []bool{false, true} {
				m := fixture(t)
				var msg tea.Msg
				var cmd tea.Cmd
				if kind == "file" {
					m.diffEnabled = true
					m, cmd = keyUpdate(m, "x")
					file := commandResult[fileContentMsg](t, cmd)
					file.content = "new file"
					if failed {
						file.err = errors.New("rate limited")
					}
					msg = file
				} else {
					m, cmd = keyUpdate(m, "v")
					diff := commandResult[diffMsg](t, cmd)
					diff.raw, diff.rendered = "new diff", "new diff"
					if failed {
						diff.err = errors.New("rate limited")
					}
					msg = diff
				}
				if m.statusText != "fetching "+kind+"…" {
					t.Fatalf("start status = %q", m.statusText)
				}
				m = deliver(m, msg)
				if failed {
					if !m.statusErr || !strings.Contains(m.statusText, "rate limited") {
						t.Fatalf("%s failure did not settle its status: %q", kind, m.statusText)
					}
				} else if m.statusActive || m.statusText != "" {
					t.Fatalf("%s success left loading status %q", kind, m.statusText)
				}
			}
		})
	}
	t.Run("overlapping sources and same-key requests", func(t *testing.T) {
		m, first := keyUpdate(fixture(t), "v")
		old := commandResult[diffMsg](t, first)
		old.err = errors.New("old diff error")
		m = pressKey(m, "v")
		m, second := keyUpdate(m, "v")
		current := commandResult[diffMsg](t, second)
		pr, _ := m.current()
		passive := commandResult[diffMsg](t, diffCmd(m.app, pr, 60, 0, 0))
		passive.err = errors.New("passive diff error")
		m.error, m.message = "", "existing notice"
		for _, msg := range []diffMsg{old, passive} {
			m = deliver(m, msg)
			if m.statusText != "fetching diff…" || m.statusErr || m.error != "" || m.message != "existing notice" {
				t.Fatalf("unowned response changed diff status or fallback: status=%q error=%q message=%q", m.statusText, m.error, m.message)
			}
		}
		m, cmd := keyUpdate(m, "x")
		file := commandResult[fileContentMsg](t, cmd)
		current.err = errors.New("diff failed after file started")
		m = deliver(m, current)
		if m.statusText != "fetching file…" || m.statusErr || m.error != "" {
			t.Fatalf("late diff completion touched file status: %q %q", m.statusText, m.error)
		}
		file.content = "stale file"
		file.err = errors.New("stale file failure")
		m.prs[0].HeadRefOID = "different"
		m = deliver(m, file)
		if m.statusActive || m.fileContent != "" || m.error != "" {
			t.Fatalf("current owner with discarded file payload did not settle quietly: %q %q %q", m.statusText, m.fileContent, m.error)
		}
	})
}

func TestInboxAgeDoesNotRebuildRowLayout(t *testing.T) {
	for _, width := range []int{45, 95, 150} {
		m := scrollModel(t, 25)
		m.width, m.height, m.selected = width, 24, 5
		m.resizeViewports()
		m.inboxManualScroll = true
		m.inboxVP.SetYOffset(4)
		m.lastInboxRefresh = timeNow().Add(-44 * time.Minute)
		m.startStatus("fetching diff…")
		view := stripANSI(m.View().Content)
		lines := strings.Split(view, "\n")
		title, header := lines[1], lines[0]
		if !strings.Contains(title, "updated 44m ago") || strings.Contains(header, "updated") || !strings.Contains(header, "fetching diff…") ||
			strings.ContainsFunc(header, func(r rune) bool { return r >= '\u2800' && r <= '\u28ff' }) {
			t.Fatalf("width %d: unexpected Inbox/header: %q / %q", width, title, header)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d: overflowing line %q", width, line)
			}
		}
		before, y := m.inboxVPLayout, m.inboxVP.YOffset()
		m.lastInboxRefresh = m.lastInboxRefresh.Add(-2 * time.Minute)
		m = deliver(m, uiTickMsg{})
		if !strings.Contains(m.inboxTitle(), "updated 46m ago") || m.inboxVPLayout != before || m.selected != 5 || m.inboxVP.YOffset() != y {
			t.Fatalf("width %d: tick failed to update only the title: %q", width, m.inboxTitle())
		}
	}
}

func TestHeaderLoadStatusOtherActionCompletionIsScoped(t *testing.T) {
	m, _, a, _ := observationModel(t)
	a.Files = []model.ChangedFile{{Path: "main.go"}}
	m.prs[0], m.allPRs[0] = a, a
	m.diffEnabled = true
	m, cmd := keyUpdate(m, "A")
	approval := commandResult[actionMsg](t, cmd)
	m, cmd = keyUpdate(m, "x")
	file := commandResult[fileContentMsg](t, cmd)
	if m.statusText != "fetching file…" {
		t.Fatalf("expected file status, got %q", m.statusText)
	}
	m = deliver(m, approval)
	if m.statusText != "fetching file…" || m.message != "" || m.error != "" {
		t.Fatalf("obsolete approval result changed the header: %q / %q / %q", m.statusText, m.message, m.error)
	}
	m = deliver(m, file)
	if m.statusActive || m.message != "" || m.error != "" {
		t.Fatalf("file completion did not clear only itself: %q / %q / %q", m.statusText, m.message, m.error)
	}
}

func TestOlderSameKeyLoadsCannotReplaceVisiblePayload(t *testing.T) {
	t.Run("diff", func(t *testing.T) {
		m, _, _, _ := observationModel(t)
		m = deliver(m, tea.WindowSizeMsg{Width: 65, Height: 20})
		m, first := keyUpdate(m, "v")
		old := commandResult[diffMsg](t, first)
		m = pressKey(m, "v")
		m, second := keyUpdate(m, "v")
		current := commandResult[diffMsg](t, second)
		current.raw, current.rendered = strings.Repeat("current diff\n", 40), strings.Repeat("current diff\n", 40)
		m = deliver(m, current)
		m.diffVP.SetYOffset(6)
		old.raw, old.rendered = "obsolete diff", "obsolete diff"
		m = deliver(m, old)
		view := stripANSI(m.View().Content)
		if !strings.Contains(view, "current diff") || strings.Contains(view, "obsolete diff") || m.diffVP.YOffset() != 6 {
			t.Fatalf("older same-head diff changed the pane or scroll (offset=%d): %q", m.diffVP.YOffset(), view)
		}
	})
	t.Run("file", func(t *testing.T) {
		m, _, a, _ := observationModel(t)
		a.Files = []model.ChangedFile{{Path: "main.go"}}
		m.prs[0], m.allPRs[0] = a, a
		m.diffEnabled = true
		m = deliver(m, tea.WindowSizeMsg{Width: 120, Height: 24})
		m, first := keyUpdate(m, "x")
		old := commandResult[fileContentMsg](t, first)
		m = pressKey(m, "x")
		m, second := keyUpdate(m, "x")
		current := commandResult[fileContentMsg](t, second)
		current.content = "current payload"
		m = deliver(m, current)
		old.content = "obsolete payload"
		m = deliver(m, old)
		if view := stripANSI(m.View().Content); !strings.Contains(view, "current payload") || strings.Contains(view, "obsolete payload") {
			t.Fatalf("older same-key file changed the pane: %q", view)
		}
	})
}

func TestHeaderLoadStatusSurvivesDelayedNotices(t *testing.T) {
	m, _, a, _ := observationModel(t)
	a.Files = []model.ChangedFile{{Path: "main.go"}}
	m.prs[0], m.allPRs[0] = a, a
	m.diffEnabled = true
	m = deliver(m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m, cmd := keyUpdate(m, "x")
	file := commandResult[fileContentMsg](t, cmd)
	m.message = "prior notice"
	cfg := m.cfg
	cfg.Sort = "number"
	actions, err := newActionRegistry(cfg.Actions)
	if err != nil {
		t.Fatal(err)
	}
	msg := configReloadedMsg{cfg: cfg, actions: actions, revision: m.settingsSaver.revision.Load()}
	badConfig := msg
	badConfig.err = errors.New("bad YAML")
	for _, notice := range []tea.Msg{
		clipboardMsg{}, clipboardMsg{err: errors.New("clipboard failed")},
		configWatchCheckedMsg{result: &msg}, configWatchCheckedMsg{result: &badConfig},
		settingsSavedMsg{err: errors.New("disk full")},
	} {
		m = deliver(m, notice)
		if header := stripANSI(m.headerView()); !strings.Contains(header, "fetching file") || m.message != "prior notice" || m.error != "" {
			t.Errorf("%T replaced another owner's status: %q / %q / %q", notice, header, m.message, m.error)
		}
	}
	if m.sortMode != app.SortNumber || m.cfg.Sort != "number" {
		t.Fatal("delayed config notice prevented the configuration change")
	}
	m = deliver(m, file)
	if m.statusActive {
		t.Fatalf("matching result did not settle: %q", m.statusText)
	}
}

func TestHeaderLoadStatusPassiveErrorStaysInDiffPane(t *testing.T) {
	m, _, a, _ := observationModel(t)
	m.diffEnabled = true
	m.startStatus("approving…")
	m = deliver(m, diffMsg{repo: a.Repo, number: a.Number, key: prRefreshKey(a), err: errors.New("GitHub rate limited")})
	if m.statusText != "approving…" || !strings.Contains(m.diffPlaceholder(), "GitHub rate limited") {
		t.Fatalf("passive error did not stay in the diff pane: header=%q pane=%q", m.statusText, m.diffPlaceholder())
	}
}
