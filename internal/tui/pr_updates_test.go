package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/config"
	"github.com/senilio/zappr/internal/model"
)

func observationModel(t *testing.T) (Model, *commentRecordingGitHub, model.PullRequest, model.PullRequest) {
	t.Helper()
	a := model.PullRequest{Repo: "org/repo", Number: 1, State: "OPEN", HeadRefOID: "old"}
	b := a
	b.Number = 2
	rows := []model.PullRequest{a, b}
	store := cache.NewMemoryStore()
	for _, pr := range rows {
		if err := store.Put(pr); err != nil {
			t.Fatal(err)
		}
	}
	provider := &commentRecordingGitHub{listedPRs: rows, getPR: a}
	cfg := config.Default()
	cfg.Actions["preset"] = config.ActionConfig{Operation: "comment", Keys: []string{"P"}, Commands: []string{"preset"}, Comment: "hello"}
	m, err := New(app.New(provider, store), a.Repo, 0, "", cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.prs, m.allPRs = rows, append([]model.PullRequest(nil), rows...)
	m.selected, m.diffEnabled = 0, false
	return m, provider, a, b
}

func keyUpdate(m Model, key string) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Text: key})
	return next.(Model), cmd
}

func deliver(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func testPRUpdate(pr model.PullRequest, revision uint64) app.PRUpdate {
	return app.PRUpdate{Key: keyForPR(pr), PR: pr, Revision: revision}
}

func testPRRemoval(pr model.PullRequest, revision uint64) refreshPRMsg {
	return refreshPRMsg{update: app.PRUpdate{Key: keyForPR(pr), Revision: revision, ClosedRevision: revision, Closed: true}}
}

func testInboxUpdates(revision uint64, prs ...model.PullRequest) []app.PRUpdate {
	updates := make([]app.PRUpdate, 0, len(prs))
	for _, pr := range prs {
		updates = append(updates, testPRUpdate(pr, revision))
	}
	return updates
}

func commandResult[T any](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	var find func(tea.Cmd) (T, bool)
	find = func(cmd tea.Cmd) (T, bool) {
		if cmd != nil {
			switch msg := cmd().(type) {
			case T:
				return msg, true
			case tea.BatchMsg:
				for _, c := range msg {
					if result, ok := find(c); ok {
						return result, true
					}
				}
			}
		}
		var zero T
		return zero, false
	}
	msg, ok := find(cmd)
	if !ok {
		t.Fatalf("command did not produce %T", msg)
	}
	return msg
}

func TestPRRefreshDeliveryOrder(t *testing.T) {
	for _, scenario := range []string{"head", "manual head", "closure"} {
		for _, order := range []string{"full first", "targeted first"} {
			t.Run(scenario+"/"+order, func(t *testing.T) {
				m, provider, a, b := observationModel(t)
				provider.getPR.HeadRefOID = "new"
				var full refreshMsg
				var targeted tea.Msg
				fullRefresh := func() {
					var cmd tea.Cmd
					m, cmd = keyUpdate(m, "R")
					full = commandResult[refreshMsg](t, cmd)
				}
				if scenario == "closure" {
					targeted = commandResult[refreshPRMsg](t, refreshPRCmd(m.app, a))
					provider.listedPRs = []model.PullRequest{b}
					fullRefresh()
				} else {
					fullRefresh()
					cmd := refreshPRCmd(m.app, a)
					if scenario == "manual head" {
						m, cmd = keyUpdate(m, "r")
						targeted = commandResult[prActionResult](t, cmd)
					} else {
						targeted = commandResult[refreshPRMsg](t, cmd)
					}
				}
				if order == "full first" {
					m = deliver(deliver(m, full), targeted)
				} else {
					m = deliver(deliver(m, targeted), full)
				}
				if scenario == "closure" {
					if len(m.prs) != 1 || m.prs[0].Number != b.Number {
						t.Fatalf("closure resurrected or removed an unrelated PR: %+v", m.prs)
					}
				} else if pr, ok := m.current(); !ok || pr.Number != a.Number || pr.HeadRefOID != "new" {
					t.Fatalf("selected PR = %+v, want the newer head of #%d", pr, a.Number)
				}
			})
		}
	}
}

func TestPRModalKeepsOriginalTarget(t *testing.T) {
	for _, modal := range []struct {
		name, open, submit string
	}{
		{"editor", "c", "enter"},
		{"approve", "a", "y"},
		{"confirm", "P", "y"},
		{"palette", ":", "enter"},
	} {
		t.Run(modal.name, func(t *testing.T) {
			m, provider, _, b := observationModel(t)
			m = pressKey(m, modal.open)
			switch modal.name {
			case "editor":
				m = pressKey(m, "X")
			case "palette":
				for _, ch := range "preset" {
					m = pressKey(m, string(ch))
				}
			}
			provider.listedPRs = []model.PullRequest{b}
			refresh := m.startInboxRefresh()
			m = deliver(m, commandResult[refreshMsg](t, refresh))
			pr, _ := m.current()
			if pr.Number != b.Number {
				t.Fatalf("expected selection to fall back to #%d, got #%d", b.Number, pr.Number)
			}
			m, cmd := keyUpdate(m, modal.submit)
			if cmd != nil {
				runCmd(cmd)
			}
			if len(provider.comments) != 0 || provider.approvals != 0 {
				t.Fatalf("background removal redirected a mutation to #%d", b.Number)
			}
			if m.mode == ModeComment || m.mode == ModeConfirmApprove || m.mode == ModeConfirmCommand {
				t.Fatal("a modal for a removed target is still open")
			}
		})
	}
	t.Run("still visible", func(t *testing.T) {
		m, provider, a, _ := observationModel(t)
		m = pressKey(pressKey(m, "c"), "X")
		m.selected = 1 // Selection moved, but the editor is still bound to a.
		updateKey(m, "enter")
		if len(provider.commentTargets) != 1 || provider.commentTargets[0] != keyForPR(a) {
			t.Fatalf("editor posted to %+v, want #%d", provider.commentTargets, a.Number)
		}
	})
	t.Run("reopened before removal delivered", func(t *testing.T) {
		m, provider, a, _ := observationModel(t)
		m = pressKey(pressKey(m, "c"), "X")
		closed, err := m.app.ClosePRUpdate(t.Context(), keyForPR(a), "")
		if err != nil || !closed.Closed {
			t.Fatalf("close update = %+v, %v", closed, err)
		}
		m = deliver(m, commandResult[refreshPRMsg](t, refreshPRCmd(m.app, a)))
		m = updateKey(m, "enter")
		if m.mode == ModeComment || len(provider.comments) != 0 || len(m.prs) != 2 {
			t.Fatalf("reopened PR did not invalidate old editor: mode=%v comments=%v prs=%d", m.mode, provider.comments, len(m.prs))
		}
	})
}

func TestPRModalWriteStopsAtKnownClosure(t *testing.T) {
	for _, mode := range []string{"editor", "approve", "merge"} {
		for _, reopened := range []bool{false, true} {
			name := mode + "/closed"
			if reopened {
				name = mode + "/reopened"
			}
			t.Run(name, func(t *testing.T) {
				m, provider, a, b := observationModel(t)
				submit := "y"
				switch mode {
				case "editor":
					m = pressKey(pressKey(m, "c"), "X")
					submit = "enter"
				case "approve":
					m = pressKey(m, "a")
				case "merge":
					m = pressKey(m, ":")
					m.commandInput = "merge"
					m = pressKey(m, "enter")
				}

				provider.listedPRs = []model.PullRequest{b}
				closure := commandResult[refreshMsg](t, m.startInboxRefresh())
				// GitHub's response is accepted by App; Bubble Tea has not
				// received the closure yet.
				if closure.err != nil || len(closure.updates) != 2 {
					t.Fatalf("did not observe closure: %+v", closure)
				}
				_, cmd := keyUpdate(m, submit)
				if reopened {
					if _, err := m.app.RefreshPRUpdate(t.Context(), keyForPR(a)); err != nil {
						t.Fatal(err)
					}
				}
				result := commandResult[actionMsg](t, cmd)
				if !errors.Is(result.err, app.ErrPRNotOpen) || len(provider.comments) != 0 || provider.approvals != 0 {
					t.Fatalf("write to stale open period: error=%v comments=%v approves=%d", result.err, provider.comments, provider.approvals)
				}
			})
		}
	}
}

func TestPRModalInvalidationEatsConfirmationShortcut(t *testing.T) {
	m, provider, _, b := observationModel(t)
	immediate := m.cfg.Actions["approve-now"]
	immediate.Keys = []string{"y"}
	m.cfg.Actions["approve-now"] = immediate
	var err error
	m.actions, err = newActionRegistry(m.cfg.Actions)
	if err != nil {
		t.Fatal(err)
	}

	m = pressKey(m, "a")
	assertMode(t, m, ModeConfirmApprove)
	provider.listedPRs = []model.PullRequest{b}
	m = deliver(m, commandResult[refreshMsg](t, m.startInboxRefresh()))
	for range 2 {
		m = updateKey(m, "y")
	}
	if provider.approvals != 0 {
		t.Fatal("invalidated approval input triggered the normal shortcut on the next PR")
	}
	m = pressKey(m, "g")
	updateKey(m, "y")
	if provider.approvals != 1 {
		t.Fatal("the explicit normal shortcut did not become available again")
	}
}

func TestPRRefreshOtherHeadKeepsDiff(t *testing.T) {
	m, provider, a, b := observationModel(t)
	m.selected = 1
	m.diff = "diff for #2"
	provider.getPR.HeadRefOID = "new"
	m = deliver(m, commandResult[refreshPRMsg](t, refreshPRCmd(m.app, a)))
	pr, _ := m.current()
	if pr.Number != b.Number || m.diff != "diff for #2" {
		t.Fatalf("another PR changed the selected diff: #%d, %q", pr.Number, m.diff)
	}
}

func TestInboxRefreshUsesCurrentDraftFilter(t *testing.T) {
	m, provider, _, _ := observationModel(t)
	provider.listedPRs[0].Draft = true
	m, cmd := keyUpdate(m, "R")
	m.hideDrafts = false
	m = deliver(m, commandResult[refreshMsg](t, cmd))
	if len(m.prs) != 2 {
		t.Fatalf("unhidden draft missing from Inbox: %+v", m.prs)
	}
}
