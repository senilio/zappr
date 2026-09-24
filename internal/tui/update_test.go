package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/config"
	"github.com/senilio/zappr/internal/model"
	"github.com/senilio/zappr/internal/tui/theme"
)

type commentRecordingGitHub struct {
	comments  []string
	approvals int
}

func (f *commentRecordingGitHub) ListPRs(context.Context, string) ([]model.PullRequest, bool, error) {
	return nil, true, nil
}

func (f *commentRecordingGitHub) GetPR(context.Context, string, int) (model.PullRequest, error) {
	return model.PullRequest{}, nil
}

func (f *commentRecordingGitHub) GetDiff(context.Context, string, int) (string, error) {
	return "", nil
}

func (f *commentRecordingGitHub) GetCommitDiff(context.Context, string, string) (string, error) {
	return "", nil
}

func (f *commentRecordingGitHub) GetFileContent(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (f *commentRecordingGitHub) Approve(context.Context, string, int, string) error {
	f.approvals++
	return nil
}

func (f *commentRecordingGitHub) Comment(_ context.Context, _ string, _ int, body string) error {
	f.comments = append(f.comments, body)
	return nil
}

func (f *commentRecordingGitHub) ToggleDraft(context.Context, string, int, bool) error { return nil }

func (f *commentRecordingGitHub) ClosePR(context.Context, string, int, string) error { return nil }

func (f *commentRecordingGitHub) UpdateBranch(context.Context, string, int) error { return nil }

func (f *commentRecordingGitHub) RequestChanges(context.Context, string, int, string) error {
	return nil
}

func (f *commentRecordingGitHub) MergePR(context.Context, string, int) error { return nil }

func TestFilterPRsShowsDraftsByDefault(t *testing.T) {
	prs := []model.PullRequest{
		{Number: 1, Draft: true},
		{Number: 2},
		{Number: 3, Draft: true},
	}

	got := filterPRs(prs, false)

	if len(got) != 3 {
		t.Fatalf("filterPRs = %#v, want all PRs", got)
	}
}

func TestFilterPRsCanHideDrafts(t *testing.T) {
	prs := []model.PullRequest{
		{Number: 1, Draft: true},
		{Number: 2},
		{Number: 3, Draft: true},
	}

	got := filterPRs(prs, true)

	if len(got) != 1 || got[0].Number != 2 {
		t.Fatalf("filterPRs = %#v, want only PR #2", got)
	}
}

func TestSettingsToggleCanHideDrafts(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{
		{Number: 1, Draft: true},
		{Number: 2},
	}, 0)
	m.mode = ModeSettings
	m.prevMode = ModeInbox

	m = pressSpecial(m, tea.KeySpace)

	if !m.hideDrafts {
		t.Fatal("hideDrafts should be enabled")
	}
	if len(m.prs) != 1 || m.prs[0].Number != 2 {
		t.Fatalf("prs = %#v, want only non-draft PR", m.prs)
	}
}

func TestSettingsCanCycleSortMode(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 2}, {Number: 1}}, 0)
	m.mode = ModeSettings
	m.settingsCursor = 1

	m = pressSpecial(m, tea.KeySpace)

	if m.sortMode != app.SortNumber {
		t.Fatalf("sortMode = %s, want number", m.sortMode)
	}
	if m.prs[0].Number != 2 {
		t.Fatalf("prs = %#v, want sorted by number descending", m.prs)
	}
}

func TestSettingsSortModePersistsToConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	m := modelWithPRs([]model.PullRequest{{Number: 2}, {Number: 1}}, 0)
	m.mode = ModeSettings
	m.settingsCursor = 1

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("cycling sort mode should return a save command")
	}
	cmd()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Sort != app.SortNumber.String() {
		t.Fatalf("cfg.Sort = %q, want %q", cfg.Sort, app.SortNumber.String())
	}
}

func TestSettingsHideDraftsPersistsToConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	m := modelWithPRs([]model.PullRequest{{Number: 1, Draft: true}, {Number: 2}}, 0)
	m.mode = ModeSettings

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("toggling draft visibility should return a save command")
	}
	cmd()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !cfg.HideDrafts || !m.hideDrafts {
		t.Fatalf("hide drafts = config:%v model:%v, want true", cfg.HideDrafts, m.hideDrafts)
	}
}

func TestNewLoadsSortModeFromConfig(t *testing.T) {
	zappr := app.New(nil, nil, nil)
	cfg := config.Default()
	cfg.Sort = "number"
	m, err := New(zappr, "org/repo", 0, "", cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.sortMode != app.SortNumber {
		t.Fatalf("sortMode = %s, want number", m.sortMode)
	}
}

func TestNewDefaultsPollInterval(t *testing.T) {
	m, err := New(app.New(nil, nil, nil), "org/repo", 0, "", config.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.pollEvery != 30*time.Second {
		t.Fatalf("pollEvery = %s, want 30s", m.pollEvery)
	}
}

func TestNewLoadsHideDraftsFromConfig(t *testing.T) {
	zappr := app.New(nil, nil, nil)
	cfg := config.Default()
	cfg.HideDrafts = true
	m, err := New(zappr, "org/repo", 0, "", cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !m.hideDrafts {
		t.Fatal("hideDrafts = false, want true")
	}
}

func TestActionRegistryResolvesCustomAliases(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["hello"] = config.ActionConfig{
		Operation: "comment", Keys: []string{"H"}, Commands: []string{"hello"}, Comment: "hello world",
	}

	registry, err := newActionRegistry(cfg.Actions)
	if err != nil {
		t.Fatalf("newActionRegistry: %v", err)
	}
	byKey, ok := registry.actionForKey(actionContextNormal, "H")
	if !ok || byKey.ID != "hello" {
		t.Fatalf("actionForKey(H) = %#v, %v; want hello, true", byKey, ok)
	}
	byCommand, ok := registry.exactCommand("hello")
	if !ok || byCommand.ID != byKey.ID {
		t.Fatalf("exactCommand(hello) = %#v, %v; want hello, true", byCommand, ok)
	}
}

func TestActionRegistryGenericActions(t *testing.T) {
	configured := config.DefaultActions()
	confirm := false
	configured["deploy"] = config.ActionConfig{
		Operation: "comment", Comment: "atlantis plan", Commands: []string{"ap"}, Confirm: &confirm,
	}
	configured["approve-now"] = config.ActionConfig{
		Operation: "approve", Keys: []string{"A"}, Confirm: &confirm,
		Description: "Approve selected PR without confirmation",
	}

	registry, err := newActionRegistry(configured)
	if err != nil {
		t.Fatal(err)
	}
	comment, ok := registry.exactCommand("ap")
	if !ok || comment.Operation != "comment" || comment.Comment != "atlantis plan" || comment.Confirmation != confirmationNone {
		t.Fatalf(":ap = %#v, %v", comment, ok)
	}
	approve, ok := registry.actionForKey(actionContextNormal, "A")
	if !ok || approve.Operation != "approve" || approve.Label != "Approve selected PR without confirmation" {
		t.Fatalf("A = %#v, %v", approve, ok)
	}
	for _, id := range []string{"approve-with-confirmation", "approve-immediately", "write-comment"} {
		if _, ok := registry.actionByID(id); ok {
			t.Errorf("removed action %q is registered", id)
		}
	}
}

func TestGenericActionDescriptions(t *testing.T) {
	for _, test := range []struct {
		action tuiAction
		want   string
	}{
		{tuiAction{Operation: "comment"}, "Write a comment on PR."},
		{tuiAction{Operation: "comment", Comment: "hello", Confirmation: confirmationNone}, `comment "hello"`},
		{tuiAction{Operation: "approve", Confirmation: confirmationMandatory}, "Approve PR. Confirmation required."},
		{tuiAction{Operation: "approve", Label: "Use this", Confirmation: confirmationMandatory}, "Use this"},
	} {
		if got := actionDisplayLabel(test.action); got != test.want {
			t.Errorf("actionDisplayLabel(%#v) = %q, want %q", test.action, got, test.want)
		}
	}
	confirmed := false
	cfg := config.Default()
	cfg.Actions["deploy"] = config.ActionConfig{Operation: "comment", Comment: "hello", Commands: []string{"deploy"}, Confirm: &confirmed}
	cfg.Actions["approve-now"] = config.ActionConfig{Operation: "approve", Commands: []string{"approve-now"}, Confirm: &confirmed, Description: "Use this"}
	m, _ := configuredModel(t, cfg, true)
	m.width = 100
	m.height = 40
	m.mode = ModeCommand
	panel := stripANSI(m.commandPanel())
	for _, want := range []string{`comment "hello"`, "Use this"} {
		if !strings.Contains(panel, want) {
			t.Errorf("palette is missing generated label %q: %q", want, panel)
		}
	}
}

func TestCommandPaletteUsesConciseRequestChangesLabel(t *testing.T) {
	m, _ := configuredModel(t, config.Default(), true)
	m.width = 60
	m.height = 40
	m.mode = ModeCommand

	panel := stripANSI(m.commandPanel())
	if !strings.Contains(panel, ":block  block PR merge [?]") {
		t.Fatalf("palette = %q, want concise block command label", panel)
	}
}

func TestActionRegistryFixedBindings(t *testing.T) {
	registry, err := newActionRegistry(config.DefaultActions())
	if err != nil {
		t.Fatal(err)
	}
	if action, ok := registry.actionForKey(actionContextNormal, "down"); !ok || action.ID != "move-down" {
		t.Fatalf("down = %#v, %v", action, ok)
	}
	if action, ok := registry.actionForKey(actionContextNormal, "tab"); !ok || action.ID != "focus-next" {
		t.Fatalf("tab = %#v, %v", action, ok)
	}
	for _, id := range []string{"next-file", "prev-file"} {
		if _, ok := registry.actionByID(id); ok {
			t.Errorf("removed action %q is registered", id)
		}
	}
}

func TestActionRegistryIgnoresConfiguredFixedAction(t *testing.T) {
	configured := config.DefaultActions()
	configured["move-down"] = config.ActionConfig{Keys: []string{"M"}}

	registry, err := newActionRegistry(configured)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.actionForKey(actionContextNormal, "M"); ok {
		t.Fatal("configured fixed action key M must not be bound")
	}
	if action, ok := registry.actionForKey(actionContextNormal, "down"); !ok || action.ID != "move-down" {
		t.Fatalf("down = %#v, %v; want move-down, true", action, ok)
	}
}

func TestActionRegistryUsesConfiguredAliases(t *testing.T) {
	configured := config.DefaultActions()
	configured["refresh-inbox"] = config.ActionConfig{Keys: []string{"F"}}

	registry, err := newActionRegistry(configured)
	if err != nil {
		t.Fatal(err)
	}
	if action, ok := registry.actionForKey(actionContextNormal, "F"); !ok || action.ID != "refresh-inbox" {
		t.Fatalf("F = %#v, %v", action, ok)
	}
}

func TestActionRegistryRejectsNormalKeyCollision(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["refresh-inbox"] = config.ActionConfig{Keys: []string{"H"}}
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Keys: []string{"H"}, Comment: "hello world"}

	_, err := newActionRegistry(cfg.Actions)
	if err == nil || !strings.Contains(err.Error(), "refresh-inbox") || !strings.Contains(err.Error(), "hello") {
		t.Fatalf("newActionRegistry error = %v, want conflicting action IDs", err)
	}
}

func TestActionRegistryRejectsCommandCollisionIgnoringCase(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Commands: []string{"AI"}, Comment: "hello world"}

	registry, err := newActionRegistry(cfg.Actions)
	if err != nil {
		t.Fatalf("newActionRegistry: %v", err)
	}
	if action, ok := registry.exactCommand("ai"); !ok || action.ID != "hello" {
		t.Fatalf(":ai = %#v, %v; want hello, true", action, ok)
	}
}

func TestActionRegistryIgnoresConfiguredTextEntryControl(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["command-cancel"] = config.ActionConfig{Keys: []string{"enter"}}

	registry, err := newActionRegistry(cfg.Actions)
	if err != nil {
		t.Fatal(err)
	}
	if action, ok := registry.actionForKey(actionContextCommand, "enter"); !ok || action.ID != "command-submit" {
		t.Fatalf("command enter = %#v, %v; want command-submit, true", action, ok)
	}
}

func TestActionRegistryAllowsKeysInDifferentContexts(t *testing.T) {
	cfg := config.Default()

	registry, err := newActionRegistry(cfg.Actions)
	if err != nil {
		t.Fatalf("newActionRegistry: %v", err)
	}
	if action, ok := registry.actionForKey(actionContextNormal, "down"); !ok || action.ID != "move-down" {
		t.Fatalf("normal down = %#v, %v; want move-down, true", action, ok)
	}
	if action, ok := registry.actionForKey(actionContextSettings, "j"); !ok || action.ID != "settings-next" {
		t.Fatalf("settings j = %#v, %v; want settings-next, true", action, ok)
	}
}

func TestGenericActionsDispatchByOperation(t *testing.T) {
	cfg := config.Default()
	confirmed := false
	cfg.Actions["approve"] = config.ActionConfig{Operation: "approve", Keys: []string{"Z"}, Commands: []string{"approve-confirmed"}}
	cfg.Actions["approve-now"] = config.ActionConfig{Operation: "approve", Keys: []string{"I"}, Commands: []string{"approve-immediate"}, Confirm: &confirmed}
	cfg.Actions["static-comment"] = config.ActionConfig{Operation: "comment", Keys: []string{"S"}, Commands: []string{"static-comment"}, Comment: "hello world"}
	cfg.Actions["comment"] = config.ActionConfig{Operation: "comment", Keys: []string{"W"}, Commands: []string{"editor-comment"}}
	m, provider := configuredModel(t, cfg, true)

	// Confirmed approval uses the fixed approval modal and its configured key.
	m = updateKey(m, "Z")
	assertMode(t, m, ModeConfirmApprove)
	m = updateKey(m, "y")
	if provider.approvals != 1 {
		t.Fatalf("approvals = %d, want 1", provider.approvals)
	}

	// Immediate approval dispatches from its command alias.
	m, provider = configuredModel(t, cfg, true)
	m = openCommandAndType(m, "approve-immediate")
	m = updateKey(m, "enter")
	if provider.approvals != 1 {
		t.Fatalf("approvals = %d, want 1", provider.approvals)
	}

	// Static and editor comments both dispatch from generic aliases.
	m = openCommandAndType(m, "static-comment")
	m = updateKey(m, "enter")
	assertMode(t, m, ModeConfirmCommand)
	m = updateKey(m, "y")
	assertCommentBody(t, provider, "hello world")
	m.comment = "stale draft"
	m = updateKey(m, "W")
	assertMode(t, m, ModeComment)
	if m.comment != "" {
		t.Fatalf("comment = %q, want cleared draft", m.comment)
	}
	m = updateKey(m, "esc")
	assertMode(t, m, ModeDetail)
	if len(provider.comments) != 1 {
		t.Fatalf("comments = %#v, want static comment only", provider.comments)
	}
	m = openCommandAndType(m, "editor-comment")
	m = updateKey(m, "enter")
	m = updateKey(m, "o")
	m = updateKey(m, "k")
	m = updateKey(m, "enter")
	if got := strings.Join(provider.comments, ","); got != "hello world,ok" {
		t.Fatalf("comments = %q, want hello world,ok", got)
	}
}

func TestGenericApprovalCancellation(t *testing.T) {
	cfg := config.Default()
	confirmed := false
	cfg.Actions["approve"] = config.ActionConfig{Operation: "approve", Keys: []string{"A"}}
	cfg.Actions["approve-now"] = config.ActionConfig{Operation: "approve", Confirm: &confirmed}
	m, provider := configuredModel(t, cfg, true)

	m = updateKey(m, "A")
	m = updateKey(m, "esc")
	assertMode(t, m, ModeDetail)
	if provider.approvals != 0 {
		t.Fatalf("approvals = %d, want 0 after cancellation", provider.approvals)
	}
}

func fullscreenDiffModel(t *testing.T, cfg config.Config) (Model, *commentRecordingGitHub) {
	t.Helper()
	m, provider := configuredModel(t, cfg, true)
	// Below minMediumWidth so the diff really is full-screen, not just a pane.
	m.width = 60
	m.height = 40
	m.resizeViewports()
	m.diffEnabled = true
	m.rawDiff = "diff --git a/a b/a"
	m.diff = strings.Repeat("diff line "+strings.Repeat("x", 300)+"\n", 200)
	m.diffVP.SetContent(m.diff)
	m.mode = ModeDiff
	m.focus = FocusDiff
	return m, provider
}

func TestFullscreenDiffIgnoresMutatingNormalActions(t *testing.T) {
	cfg := config.Default()
	confirm := false
	cfg.Actions["approve"] = config.ActionConfig{Operation: "approve", Keys: []string{"Z"}, Confirm: &confirm}
	cfg.Actions["comment"] = config.ActionConfig{Operation: "comment", Keys: []string{"W"}}
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Keys: []string{"H"}, Comment: "hello", Confirm: &confirm}
	m, provider := fullscreenDiffModel(t, cfg)

	// Generic actions remain unreachable while the diff is full-screen.
	for _, key := range []string{"Z", "W", "H"} {
		m = updateKey(m, key)
	}

	if provider.approvals != 0 {
		t.Errorf("approvals = %d, want 0 in fullscreen diff", provider.approvals)
	}
	if len(provider.comments) != 0 {
		t.Errorf("comments = %#v, want none in fullscreen diff", provider.comments)
	}
	if m.mode != ModeDiff {
		t.Errorf("mode = %v, want ModeDiff to be preserved", m.mode)
	}
}

func TestFullscreenDiffKeepsScrollingAndNavigation(t *testing.T) {
	cfg := config.Default()

	m, _ := fullscreenDiffModel(t, cfg)
	m = updateKey(m, "down")
	if m.diffVP.YOffset == 0 {
		t.Error("down did not scroll diff down in fullscreen diff")
	}
	m = updateKey(m, "up")
	if m.diffVP.YOffset != 0 {
		t.Errorf("up did not scroll diff up, YOffset = %d", m.diffVP.YOffset)
	}

	m, _ = fullscreenDiffModel(t, cfg)
	m = updateKey(m, "G")
	bottom := m.diffVP.YOffset
	if bottom == 0 {
		t.Error("G did not jump to bottom in fullscreen diff")
	}
	m = updateKey(m, "g")
	if m.diffVP.YOffset != 0 {
		t.Errorf("g did not jump to top, YOffset = %d", m.diffVP.YOffset)
	}

	m, _ = fullscreenDiffModel(t, cfg)
	m = updateKey(m, "pgdown")
	if m.diffVP.YOffset == 0 {
		t.Error("pgdown did not page down in fullscreen diff")
	}
	m = updateKey(m, "pgup")
	if m.diffVP.YOffset != 0 {
		t.Errorf("pgup did not page up, YOffset = %d", m.diffVP.YOffset)
	}

	m, _ = fullscreenDiffModel(t, cfg)
	m = updateKey(m, "right")
	if m.diffXOffset == 0 {
		t.Error("right did not pan diff in fullscreen diff")
	}
	m = updateKey(m, "left")
	if m.diffXOffset != 0 {
		t.Errorf("left did not pan back, diffXOffset = %d", m.diffXOffset)
	}
}

func TestFullscreenDiffCloseStillWorks(t *testing.T) {
	m, _ := fullscreenDiffModel(t, config.Default())
	m = updateKey(m, "esc")
	if m.mode != ModeDetail {
		t.Fatalf("mode = %v, want ModeDetail after diff-close", m.mode)
	}
}

// paneDiffModel builds a layout wide enough that the diff is only one of three
// visible panes, with ModeDiff set the way a diff-pane click sets it.
func paneDiffModel(t *testing.T, cfg config.Config, width int) (Model, *commentRecordingGitHub) {
	t.Helper()
	m, provider := configuredModel(t, cfg, true)
	m.width = width
	m.height = 40
	m.resizeViewports()
	m.diffEnabled = true
	m.rawDiff = "diff --git a/a b/a"
	m.diff = strings.Repeat("diff line\n", 200)
	m.diffVP.SetContent(m.diff)
	m.mode = ModeDiff
	m.focus = FocusDiff
	return m, provider
}

func TestDiffPaneClickKeepsNormalKeysAlive(t *testing.T) {
	for _, width := range []int{120, 80} {
		t.Run(fmt.Sprintf("width%d", width), func(t *testing.T) {
			cfg := config.Default()
			confirm := false
			cfg.Actions["approve"] = config.ActionConfig{Operation: "approve", Keys: []string{"A"}}
			cfg.Actions["approve-now"] = config.ActionConfig{Operation: "approve", Confirm: &confirm}
			cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Keys: []string{"H"}, Comment: "hello", Confirm: &confirm}
			cfg.Actions["comment"] = config.ActionConfig{Operation: "comment", Keys: []string{"W"}}
			m, provider := paneDiffModel(t, cfg, width)

			// Summary-mode switching via "2".
			m2 := updateKey(m, "2")
			if m2.summaryMode != SummaryModeCommits {
				t.Errorf("summaryMode = %v, want SummaryModeCommits after \"2\"", m2.summaryMode)
			}

			// Command palette opening via ":".
			m3 := updateKey(m, ":")
			if m3.mode != ModeCommand {
				t.Errorf("mode = %v, want ModeCommand after \":\"", m3.mode)
			}

			// Focus cycling via "tab".
			m4 := updateKey(m, "tab")
			if m4.focus == FocusDiff {
				t.Error("tab did not change focus away from diff pane")
			}

			// Generic approval confirmation via its configured key.
			m5 := updateKey(m, "A")
			if m5.mode != ModeConfirmApprove {
				t.Errorf("mode = %v, want ModeConfirmApprove after \"A\"", m5.mode)
			}

			// Generic comment composition via an empty comment action.
			m6 := updateKey(m, "W")
			if m6.mode != ModeComment {
				t.Errorf("mode = %v, want ModeComment after \"W\"", m6.mode)
			}

			// A custom unconfirmed comment action is reachable in a pane layout.
			updateKey(m, "H")
			if len(provider.comments) != 1 || provider.comments[0] != "hello" {
				t.Errorf("comments = %#v, want custom action to run in pane layout", provider.comments)
			}
		})
	}
}

func TestConfiguredActionSupportsCommandOnlyAndKeyOnly(t *testing.T) {
	cfg := config.Default()
	confirm := false
	cfg.Actions["command-comment"] = config.ActionConfig{Operation: "comment", Commands: []string{"comment-only"}, Comment: "command", Confirm: &confirm}
	cfg.Actions["key-comment"] = config.ActionConfig{Operation: "comment", Keys: []string{"H"}, Comment: "key", Confirm: &confirm}
	m, provider := configuredModel(t, cfg, true)

	m = openCommandAndType(m, "comment-only")
	m = updateKey(m, "enter")
	m = updateKey(m, "H")

	if got := strings.Join(provider.comments, ","); got != "command,key" {
		t.Fatalf("comments = %q, want command,key", got)
	}
}

func TestConfiguredActionWithoutSelectedPRShowsStatus(t *testing.T) {
	cfg := config.Default()
	confirm := false
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Keys: []string{"H"}, Comment: "hello world", Confirm: &confirm}
	m, provider := configuredModel(t, cfg, false)

	m = updateKey(m, "H")

	if !m.statusActive || m.statusText != "no PR selected" {
		t.Fatalf("status = active:%v text:%q, want active no PR selected", m.statusActive, m.statusText)
	}
	if len(provider.comments) != 0 {
		t.Fatalf("comments = %#v, want none", provider.comments)
	}
}

func TestSummaryTitleUsesFixedAliases(t *testing.T) {
	cfg := config.Default()
	m, _ := configuredModel(t, cfg, true)
	m.summaryMode = SummaryModeFiles

	title := stripANSI(m.summaryTitle())
	if want := "Summary [1 files] [2] [3] [4]"; title != want {
		t.Fatalf("summary title = %q, want %q", title, want)
	}
}

func TestCommandPaletteShowsMatchedAlias(t *testing.T) {
	cfg := config.Default()
	confirm := false
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Commands: []string{"alpha", "zulu"}, Comment: "hi", Confirm: &confirm}
	m, _ := configuredModel(t, cfg, true)
	m.width = 100
	m.height = 40

	m = openCommandAndType(m, "zul")
	panel := stripANSI(m.commandPanel())
	if !strings.Contains(panel, ":zulu") {
		t.Errorf("palette = %q, want matched alias :zulu", panel)
	}
	if strings.Contains(panel, ":alpha") {
		t.Errorf("palette = %q, should not show unmatched alias :alpha", panel)
	}
}

func TestCommandPaletteConfirmsMatchedAlias(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Commands: []string{"alpha", "zulu"}, Comment: "hi"}
	m, _ := configuredModel(t, cfg, true)
	m.width = 100
	m.height = 40

	// Prefix match resolved via cursor selection.
	m = openCommandAndType(m, "zul")
	m = pressSpecial(m, tea.KeyEnter)
	if m.mode != ModeConfirmCommand {
		t.Fatalf("mode = %v, want ModeConfirmCommand", m.mode)
	}
	if panel := stripANSI(m.commandPanel()); !strings.Contains(panel, `Comment "hi" on #1?`) || strings.Contains(panel, "org/repo") || strings.Contains(panel, "Run ") {
		t.Errorf("confirmation = %q, want action description", panel)
	}
}

func TestCommandPaletteConfirmsExactAlias(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Commands: []string{"alpha", "zulu"}, Comment: "hi"}
	m, _ := configuredModel(t, cfg, true)
	m.width = 100
	m.height = 40

	m = openCommandAndType(m, "zulu")
	m = pressSpecial(m, tea.KeyEnter)
	if panel := stripANSI(m.commandPanel()); !strings.Contains(panel, `Comment "hi" on #1?`) || strings.Contains(panel, "org/repo") || strings.Contains(panel, "Run ") {
		t.Errorf("confirmation = %q, want action description", panel)
	}
}

func TestCommandPaletteMatchesDescription(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["atlantis-apply"] = config.ActionConfig{Operation: "comment", Commands: []string{"aa"}, Comment: "atlantis apply"}
	m, _ := configuredModel(t, cfg, true)
	m.width = 100
	m.height = 40

	m = openCommandAndType(m, "apply")
	panel := stripANSI(m.commandPanel())
	if !strings.Contains(panel, `:aa  comment "atlantis apply"`) || strings.Contains(panel, "[confirm]") {
		t.Fatalf("palette = %q", panel)
	}
	m = pressSpecial(m, tea.KeyEnter)
	if m.mode != ModeConfirmCommand {
		t.Fatalf("mode = %v, want ModeConfirmCommand", m.mode)
	}
}

func TestCommandPaletteUsesShortUpdateLabel(t *testing.T) {
	m, _ := configuredModel(t, config.Default(), true)
	m.width = 100
	m.height = 40

	m = openCommandAndType(m, "update")
	panel := stripANSI(m.commandPanel())
	if !strings.Contains(panel, ":update  update branch [?]") || strings.Contains(panel, "base branch") {
		t.Fatalf("palette = %q", panel)
	}
}

func TestConfiguredActionIsShownInHelp(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Keys: []string{"H"}, Commands: []string{"hello"}, Comment: "hello world"}
	m, _ := configuredModel(t, cfg, true)

	panel := stripANSI(m.helpPanel())
	for _, want := range []string{"H", `comment "hello world"`} {
		if !strings.Contains(panel, want) {
			t.Fatalf("help = %q, missing %q", panel, want)
		}
	}
	if strings.Contains(panel, ":hello") || strings.Contains(panel, "Command palette") {
		t.Fatalf("help = %q, should omit commands", panel)
	}
}

func TestHelpPanelShowsCuratedShortcuts(t *testing.T) {
	model, _ := configuredModel(t, config.Default(), true)
	panel := stripANSI(model.helpPanel())

	for _, want := range []string{"Navigate", "↑/↓", "tab/shift+tab", "g/G", "pgdown/space", "Review", "1/2/3/4", "←/→", "next/previous file", "Actions", "open PR"} {
		if !strings.Contains(panel, want) {
			t.Errorf("help = %q, missing %q", panel, want)
		}
	}
}

func TestHelpSearchFiltersAndEscCloses(t *testing.T) {
	m, _ := configuredModel(t, config.Default(), true)
	m.width = 80
	m.height = 20
	m = updateKey(m, "?")
	if m.mode != ModeHelp {
		t.Fatalf("mode = %v, want ModeHelp", m.mode)
	}
	for _, key := range "open" {
		m = updateKey(m, string(key))
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "open PR") || strings.Contains(view, "Command palette") {
		t.Fatalf("help = %q", view)
	}
	m = updateKey(m, "esc")
	if m.mode == ModeHelp {
		t.Fatal("esc should close help")
	}
}

func TestHelpFitsScreenAndScrolls(t *testing.T) {
	m, _ := configuredModel(t, config.Default(), true)
	m.width = 80
	m.height = 30
	m = updateKey(m, "?")
	view := stripANSI(m.renderHelpModal())
	if lines := len(strings.Split(strings.TrimRight(view, "\n"), "\n")); lines > 10 {
		t.Fatalf("help has %d lines, want at most 10", lines)
	}
	if !strings.Contains(view, "│") {
		t.Fatalf("help = %q, want scrollbar", view)
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(stripANSI(line)) > m.width {
			t.Fatalf("help line exceeds width: %q", line)
		}
	}
	m = updateKey(m, "down")
	if m.helpVP.YOffset == 0 {
		t.Fatal("down should scroll help")
	}
}

func TestHelpListsBuiltInActionsOnce(t *testing.T) {
	m, _ := configuredModel(t, config.Default(), true)
	panel := stripANSI(m.helpPanel())
	for _, duplicate := range []string{"Approve PR. Confirmation required.", "Approve PR. No confirmation.", "Write a comment on PR."} {
		if strings.Contains(panel, duplicate) {
			t.Fatalf("help = %q, duplicate action %q", panel, duplicate)
		}
	}
}

func TestHelpPanelUsesResponsiveColumns(t *testing.T) {
	model, _ := configuredModel(t, config.Default(), true)
	model.width = 100
	wide := stripANSI(model.helpPanel())
	if !strings.Contains(wide, "↑/↓") {
		t.Fatalf("wide help = %q, want navigation", wide)
	}

	model.width = 50
	narrow := stripANSI(model.helpPanel())
	for _, line := range strings.Split(narrow, "\n") {
		if lipgloss.Width(line) > 40 {
			t.Fatalf("narrow help line exceeds content width: %q", line)
		}
	}
}

func TestPaletteOmitsKeyOnlyActions(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Keys: []string{"H"}, Comment: "hello world"}
	m, _ := configuredModel(t, cfg, true)
	m.mode = ModeCommand

	if panel := stripANSI(m.commandPanel()); strings.Contains(panel, "hello world") || strings.Contains(panel, ": ") {
		t.Fatalf("palette = %q, should omit key-only actions", panel)
	}
}

func TestKeyOnlyActionConfirmation(t *testing.T) {
	cfg := config.Default()
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Keys: []string{"H"}, Comment: "hello"}
	m, _ := configuredModel(t, cfg, true)
	m.width = 80
	m = updateKey(m, "H")

	panel := stripANSI(m.commandPanel())
	if !strings.Contains(panel, `Comment "hello" on #1?`) || strings.Contains(panel, "org/repo") || strings.Contains(panel, "Run ") {
		t.Fatalf("confirmation = %q", panel)
	}
}

func TestCustomCommentConfirmationEscapesWithoutTruncatingPreview(t *testing.T) {
	cfg := config.Default()
	comment := "first line\n" + strings.Repeat("long ", 40)
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Keys: []string{"H"}, Comment: comment}
	m, _ := configuredModel(t, cfg, true)
	m.width = 80
	m = updateKey(m, "H")

	panel := stripANSI(m.commandPanel())
	if !strings.Contains(panel, `Comment "first line\n`+strings.Repeat("long ", 40)+`" on #1?`) || strings.Contains(panel, "org/repo") {
		t.Fatalf("confirmation preview = %q", panel)
	}
}

func TestModalHintsUseFixedActions(t *testing.T) {
	cfg := config.Default()
	m, _ := configuredModel(t, cfg, true)

	if panel := stripANSI(m.commentPanel()); !strings.Contains(panel, "enter submit  esc cancel") {
		t.Fatalf("comment hint = %q", panel)
	}
	m.pendingAction, _ = m.actions.actionByID("hello")
	m.mode = ModeConfirmCommand
	if panel := stripANSI(m.commandPanel()); !strings.Contains(panel, "y/enter confirm  esc/n cancel") {
		t.Fatalf("confirmation hint = %q", panel)
	}
	if view := stripANSI(m.settingsView()); !strings.Contains(view, "esc/f back  q/ctrl+c quit") {
		t.Fatalf("settings hint = %q", view)
	}
}

func TestRemainingModalAndFallbackHintsUseFixedActions(t *testing.T) {
	cfg := config.Default()
	m, _ := configuredModel(t, cfg, true)

	if panel := stripANSI(m.approvePanel()); !strings.Contains(panel, "y approve  esc/n cancel") {
		t.Fatalf("approve hint = %q", panel)
	}
	if view := stripANSI(m.approveView()); !strings.Contains(view, "y to approve") || !strings.Contains(view, "esc/n to cancel") {
		t.Fatalf("approve fallback = %q", view)
	}
	if view := stripANSI(m.commentView()); !strings.Contains(view, "enter submit  esc cancel") {
		t.Fatalf("comment fallback = %q", view)
	}
	m.mode = ModeCommand
	if panel := stripANSI(m.commandPanel()); !strings.Contains(panel, "enter execute  esc cancel") {
		t.Fatalf("command hint = %q", panel)
	}
	if view := stripANSI(m.commandView()); !strings.Contains(view, "enter execute  esc cancel") {
		t.Fatalf("command fallback = %q", view)
	}
	if panel := stripANSI(m.settingsPanel()); !strings.Contains(panel, "j/down/k/up select") || !strings.Contains(panel, "esc/f close") {
		t.Fatalf("settings hint = %q", panel)
	}
	if panel := stripANSI(m.colorschemePanel()); !strings.Contains(panel, "j/down/k/up move") || !strings.Contains(panel, "esc/q cancel") {
		t.Fatalf("colorscheme hint = %q", panel)
	}
}

func TestSettingsSavePreservesActions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	cfg := config.Default()
	cfg.Actions["hello"] = config.ActionConfig{Operation: "comment", Keys: []string{"H"}, Comment: "hello world"}
	m, _ := configuredModel(t, cfg, true)
	m.settingsCursor = 1
	if cmd := m.cycleSortMode(); cmd != nil {
		cmd()
	}
	m.mode = ModeColorscheme
	m.colorschemeIndex = 1
	_, cmd := m.handleColorschemeKey("enter")
	if cmd == nil {
		t.Fatal("confirming a theme should return a save command")
	}
	for _, command := range cmd().(tea.BatchMsg) {
		command()
	}

	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got.Sort != app.SortNumber.String() || got.Actions["hello"].Comment != "hello world" {
		t.Fatalf("saved config = %#v, want sort and hello action", got)
	}
}

func TestHeaderUsesFixedActions(t *testing.T) {
	cfg := config.Default()
	m, _ := configuredModel(t, cfg, true)
	m.width = 100

	header := stripANSI(m.headerView())
	if !strings.Contains(header, "? help") || !strings.Contains(header, "q/ctrl+c quit") {
		t.Fatalf("header = %q", header)
	}
}

func TestShiftCCopiesPRURL(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		URL:    "https://github.com/org/repo/pull/1",
	}}, 0)
	m.focus = FocusInbox

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	m = next.(Model)

	if cmd == nil {
		t.Fatal("Shift+C should return a command")
	}
	if !m.statusActive {
		t.Fatal("statusActive should be true after Shift+C")
	}
	if m.statusText != "copied URL" {
		t.Fatalf("statusText = %q, want %q", m.statusText, "copied URL")
	}
}

func TestOKeyOpensPRURLInsteadOfSorting(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", URL: "https://github.com/org/repo/pull/1"}}, 0)
	m.sortMode = app.SortUpdated

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = next.(Model)

	if cmd == nil {
		t.Fatal("o key should return open-browser command")
	}
	if m.sortMode != app.SortUpdated {
		t.Fatalf("sortMode = %s, want unchanged", m.sortMode)
	}
}

func TestPRURLFallsBackToGitHubURL(t *testing.T) {
	got := prURL(model.PullRequest{Repo: "org/repo", Number: 7})
	if got != "https://github.com/org/repo/pull/7" {
		t.Fatalf("prURL = %q", got)
	}
}

func TestEmptyInboxClearsStaleDiff(t *testing.T) {
	m := minimalModel()
	m.diffEnabled = true
	m.diff = "stale diff"
	m.rawDiff = "stale raw diff"
	m.diffFiles = []diffFile{{Path: "main.go", Body: "stale"}}
	m.renderedFiles = []diffFile{{Path: "main.go", Body: "stale"}}
	m.allPRs = nil
	m.prs = nil

	m.applyPRSettings()

	if m.diff != "" || m.rawDiff != "" || len(m.diffFiles) != 0 || len(m.renderedFiles) != 0 {
		t.Fatalf("diff state not cleared: %#v", m)
	}
	if got := stripANSI(m.diffPanel(80, 10)); !strings.Contains(got, "select PR") {
		t.Fatalf("diffPanel = %q, want select PR", got)
	}
}

func TestFullFileDiffModeRendersSelectedFileContent(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number:     1,
		Repo:       "org/repo",
		HeadRefOID: "abc123",
		Files:      []model.ChangedFile{{Path: "main.go"}},
	}}, 0)
	m.diffEnabled = true
	m.diffMode = DiffModeFile
	m.diffFiles = []diffFile{{Path: "main.go", Body: "diff --git a/main.go b/main.go\n@@ -1,3 +1,3 @@\n line1\n-old line\n+line2\n line3\n"}}
	m.fileContentKey = m.selectedFileContentKey()
	m.fileContent = "line1\nline2\nline3\n"

	content := m.currentDiffContent()
	plain := stripANSI(content)

	if !strings.Contains(plain, " 1 │ line1") || !strings.Contains(plain, "-  │ old line") || !strings.Contains(plain, "+2 │ line2") || !strings.Contains(plain, " 3 │ line3") {
		t.Fatalf("content = %q, want full file with added and removed line markers", content)
	}
	if strings.Contains(plain, "-2 │") {
		t.Fatalf("deleted line should not render a second line-number column: %q", content)
	}
}

func TestInboxMarksDraftPRs(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Title: "draft", Author: "alice", Draft: true}}, 0)

	view := stripANSI(m.inboxPanel(80, 10))

	if !strings.Contains(view, "[draft]") {
		t.Fatalf("inbox missing draft marker: %q", view)
	}
}

// minimalModel returns a Model with enough state for key-handling tests,
// without requiring a real *app.App.
func minimalModel() Model {
	actions, err := newActionRegistry(config.DefaultActions())
	if err != nil {
		panic(err)
	}
	m := Model{actions: actions}
	m.prs = []model.PullRequest{
		{Number: 1, Repo: "org/repo", Title: "first"},
	}
	m.allPRs = m.prs
	m.selected = 0
	m.mode = ModeInbox
	m.focus = FocusInbox
	m.summaryMode = SummaryModeFiles
	return m
}

func modelWithPRs(prs []model.PullRequest, selected int) Model {
	m := minimalModel()
	m.prs = prs
	m.allPRs = prs
	m.selected = selected
	return m
}

func configuredModel(t *testing.T, cfg config.Config, selected bool) (Model, *commentRecordingGitHub) {
	t.Helper()
	provider := &commentRecordingGitHub{}
	m, err := New(app.New(provider, nil, nil), "org/repo", 0, "", cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.mode = ModeInbox
	m.focus = FocusInbox
	if selected {
		m.prs = []model.PullRequest{{Number: 1, Repo: "org/repo"}}
		m.allPRs = m.prs
	}
	return m, provider
}

func updateKey(m Model, key string) Model {
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	if cmd != nil {
		runCmd(cmd)
	}
	return next.(Model)
}

func runCmd(cmd tea.Cmd) {
	if msg := cmd(); msg != nil {
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, command := range batch {
				runCmd(command)
			}
		}
	}
}

func openCommandAndType(m Model, input string) Model {
	m = updateKey(m, ":")
	for _, key := range input {
		m = updateKey(m, string(key))
	}
	return m
}

func assertMode(t *testing.T, m Model, want Mode) {
	t.Helper()
	if m.mode != want {
		t.Fatalf("mode = %v, want %v", m.mode, want)
	}
}

func assertCommentBody(t *testing.T, provider *commentRecordingGitHub, want string) {
	t.Helper()
	if len(provider.comments) != 1 || provider.comments[0] != want {
		t.Fatalf("comments = %#v, want %#v", provider.comments, []string{want})
	}
}

func pressKey(m Model, key string) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return next.(Model)
}

func pressSpecial(m Model, key tea.KeyType) Model {
	next, _ := m.Update(tea.KeyMsg{Type: key})
	return next.(Model)
}

func clickMouse(m Model, x, y int) Model {
	next, _ := m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	return next.(Model)
}

// TestDiffDisabledByDefault verifies diff is off and autoDiffCmd returns nil.
func TestDiffDisabledByDefault(t *testing.T) {
	m := minimalModel()
	if m.diffEnabled {
		t.Fatal("diffEnabled should be false by default")
	}
	if cmd := m.autoDiffCmd(); cmd != nil {
		t.Fatal("autoDiffCmd should return nil when diffEnabled=false")
	}
}

// TestVKeyTogglesDiffOn verifies pressing v enables diff and stays in Summary focus.
func TestVKeyTogglesDiffOn(t *testing.T) {
	m := minimalModel()
	m.width = 200 // wide, so no narrow-path branch
	m = pressKey(m, "v")
	if !m.diffEnabled {
		t.Fatal("diffEnabled should be true after pressing v")
	}
	if m.focus == FocusDiff {
		t.Fatalf("focus should not be FocusDiff after v (diff scrolls from Summary), got %v", m.focus)
	}
}

// TestVKeyTogglesDiffOff verifies a second v press disables diff.
func TestVKeyTogglesDiffOff(t *testing.T) {
	m := minimalModel()
	m.width = 200
	m = pressKey(m, "v") // on
	m = pressKey(m, "v") // off
	if m.diffEnabled {
		t.Fatal("diffEnabled should be false after pressing v twice")
	}
	if m.focus == FocusDiff {
		t.Fatal("focus should not be FocusDiff after diff is turned off")
	}
}

// TestAutoDiffCmdRespectsEnabled verifies autoDiffCmd returns a command when enabled.
func TestAutoDiffCmdRespectsEnabled(t *testing.T) {
	m := minimalModel()
	m.diffEnabled = true
	if cmd := m.autoDiffCmd(); cmd == nil {
		t.Fatal("autoDiffCmd should return a command when diffEnabled=true and diff not yet loaded")
	}
}

// TestAutoDiffCmdDeduplicates verifies autoDiffCmd returns nil when diff already loaded for same PR.
func TestAutoDiffCmdDeduplicates(t *testing.T) {
	m := minimalModel()
	m.diffEnabled = true
	m.diffLoadKey = "org/repo#1"
	if cmd := m.autoDiffCmd(); cmd != nil {
		t.Fatal("autoDiffCmd should return nil when diffLoadKey matches current PR")
	}
}

func TestMouseClickSelectsInboxPR(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{
		{Number: 1, Repo: "org/repo", Title: "first"},
		{Number: 2, Repo: "org/repo", Title: "second"},
		{Number: 3, Repo: "org/repo", Title: "third"},
	}, 0)
	m.width = 120
	m.height = 30
	m.mode = ModeInbox
	m.focus = FocusInbox
	m.resizeViewports()

	// Wide layout: body starts below the header, pane content starts below the
	// top border. First PR occupies content lines 0-1, separator line 2, second
	// PR starts at content line 3.
	m = clickMouse(m, 2, 6)

	if m.selected != 1 {
		t.Fatalf("selected = %d, want 1", m.selected)
	}
	if m.focus != FocusInbox {
		t.Fatalf("focus = %v, want FocusInbox", m.focus)
	}
	if m.mode != ModeInbox {
		t.Fatalf("mode = %v, want ModeInbox", m.mode)
	}
}

func TestMouseClickInboxSeparatorOnlyFocuses(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{
		{Number: 1, Repo: "org/repo", Title: "first"},
		{Number: 2, Repo: "org/repo", Title: "second"},
	}, 0)
	m.width = 120
	m.height = 30
	m.mode = ModeDetail
	m.focus = FocusSummary
	m.resizeViewports()

	// Separator after first PR is not selectable.
	// With 3-line layout (2 content + 1 sep): contentY=2 is separator for PR 0.
	// region.y=1, so y=4 → contentY=4-1-1=2, line=2, 2%3=2 → separator.
	m = clickMouse(m, 2, 4)

	if m.selected != 0 {
		t.Fatalf("selected = %d, want unchanged 0", m.selected)
	}
	if m.focus != FocusInbox {
		t.Fatalf("focus = %v, want FocusInbox", m.focus)
	}
	if m.mode != ModeInbox {
		t.Fatalf("mode = %v, want ModeInbox", m.mode)
	}
}

func TestMouseClickSummaryFocusesPane(t *testing.T) {
	m := minimalModel()
	m.width = 120
	m.height = 30
	m.mode = ModeInbox
	m.focus = FocusInbox
	m.resizeViewports()

	// In wide inbox-focused layout, summary pane starts after the tall inbox pane.
	m = clickMouse(m, 2, 24)

	if m.focus != FocusSummary {
		t.Fatalf("focus = %v, want FocusSummary", m.focus)
	}
	if m.mode != ModeDetail {
		t.Fatalf("mode = %v, want ModeDetail", m.mode)
	}
}

// TestRefreshEmptyQueueResetsToInbox verifies that when the last PR is merged or
// closed externally and a refresh returns zero PRs, the UI drops back to the
// Inbox view/focus instead of staying stuck in a detail/diff mode for a PR that
// no longer exists (which made pane clicks appear dead).
func TestRefreshEmptyQueueResetsToInbox(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{
		{Number: 1, Repo: "org/repo", Title: "last"},
	}, 0)
	m.width = 120
	m.height = 30
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.resizeViewports()

	next, _ := m.Update(refreshMsg{prs: nil})
	m = next.(Model)

	if len(m.prs) != 0 {
		t.Fatalf("prs len = %d, want 0", len(m.prs))
	}
	if m.mode != ModeInbox {
		t.Fatalf("mode = %v, want ModeInbox after queue emptied", m.mode)
	}
	if m.focus != FocusInbox {
		t.Fatalf("focus = %v, want FocusInbox after queue emptied", m.focus)
	}
}

func TestRefreshPRUpdatesOnlyMatchingPR(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{
		{Number: 1, Repo: "org/repo", Title: "first"},
		{Number: 2, Repo: "org/repo", Title: "second"},
	}, 1)
	m.mode = ModeDetail
	m.focus = FocusSummary

	next, _ := m.Update(refreshPRMsg{pr: model.PullRequest{Number: 2, Repo: "org/repo", Title: "second updated"}})
	m = next.(Model)

	if len(m.prs) != 2 {
		t.Fatalf("prs len = %d, want 2", len(m.prs))
	}
	titles := map[int]string{}
	for _, pr := range m.prs {
		titles[pr.Number] = pr.Title
	}
	if titles[1] != "first" {
		t.Fatalf("first title = %q, want unchanged", titles[1])
	}
	if titles[2] != "second updated" {
		t.Fatalf("second title = %q, want updated", titles[2])
	}
	if pr, ok := m.current(); !ok || pr.Number != 2 {
		t.Fatalf("current = %#v/%v, want PR #2 preserved", pr, ok)
	}
	if m.mode != ModeDetail || m.focus != FocusSummary {
		t.Fatalf("mode/focus = %v/%v, want detail/summary preserved", m.mode, m.focus)
	}
}

func TestRefreshPRRemovesClosedPR(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{
		{Number: 1, Repo: "org/repo", Title: "first"},
		{Number: 2, Repo: "org/repo", Title: "second"},
	}, 1)
	m.mode = ModeDetail
	m.focus = FocusSummary

	next, _ := m.Update(refreshPRMsg{repo: "org/repo", number: 2, err: app.ErrPRNotOpen})
	m = next.(Model)

	if len(m.prs) != 1 || m.prs[0].Number != 1 {
		t.Fatalf("prs = %#v, want only PR #1", m.prs)
	}
	if m.mode != ModeDetail || m.focus != FocusSummary {
		t.Fatalf("mode/focus = %v/%v, want detail/summary preserved", m.mode, m.focus)
	}
}

func TestRefreshPRHeadChangeClearsDiffState(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "first", HeadRefOID: "old"}}, 0)
	m.diffEnabled = true
	m.diff = "old rendered"
	m.rawDiff = "old raw"
	m.diffFiles = []diffFile{{Path: "old.go", Body: "raw"}}
	m.renderedFiles = []diffFile{{Path: "old.go", Body: "rendered"}}
	m.diffMode = DiffModeFile
	m.fileContent = "old content"
	m.fileContentKey = "old-file"
	m.fileIndex = 1
	m.commitIndex = 1
	m.diffLoadKey = "org/repo#1@old"
	m.commitDiffKey = "commit"
	m.renderedCommitDiff = "old commit"
	m.diffXOffset = 5
	m.summaryXOffset = 3
	m.syncDiffVP()

	next, _ := m.Update(refreshPRMsg{pr: model.PullRequest{Number: 1, Repo: "org/repo", Title: "first", HeadRefOID: "new"}})
	m = next.(Model)

	if m.diff != "" || m.rawDiff != "" || len(m.diffFiles) != 0 || len(m.renderedFiles) != 0 {
		t.Fatalf("diff state not cleared: diff=%q raw=%q files=%d rendered=%d", m.diff, m.rawDiff, len(m.diffFiles), len(m.renderedFiles))
	}
	if m.fileContent != "" || m.fileContentKey != "" || m.commitDiffKey != "" || m.renderedCommitDiff != "" {
		t.Fatalf("loaded detail state not cleared")
	}
	if m.diffLoadKey != "org/repo#1@new" {
		t.Fatalf("diffLoadKey = %q, want new head fetch key", m.diffLoadKey)
	}
	if m.diffMode != DiffModeHunks || m.fileIndex != 0 || m.commitIndex != 0 || m.diffXOffset != 0 || m.summaryXOffset != 0 {
		t.Fatalf("view indexes not reset")
	}
}

func TestRefreshHeadChangeClearsDiffState(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "first", HeadRefOID: "old"}}, 0)
	m.diffEnabled = true
	m.diff = "old rendered"
	m.rawDiff = "old raw"
	m.diffFiles = []diffFile{{Path: "old.go", Body: "raw"}}
	m.renderedFiles = []diffFile{{Path: "old.go", Body: "rendered"}}
	m.diffMode = DiffModeFile
	m.fileContent = "old content"
	m.fileContentKey = "old-file"
	m.fileIndex = 1
	m.commitIndex = 1
	m.diffLoadKey = "org/repo#1@old"
	m.commitDiffKey = "commit"
	m.renderedCommitDiff = "old commit"
	m.diffXOffset = 5
	m.summaryXOffset = 3
	m.syncDiffVP()

	next, _ := m.Update(refreshMsg{prs: []model.PullRequest{{Number: 1, Repo: "org/repo", Title: "first", HeadRefOID: "new"}}})
	m = next.(Model)

	if m.diff != "" || m.rawDiff != "" || len(m.diffFiles) != 0 || len(m.renderedFiles) != 0 {
		t.Fatalf("diff state not cleared: diff=%q raw=%q files=%d rendered=%d", m.diff, m.rawDiff, len(m.diffFiles), len(m.renderedFiles))
	}
	if m.fileContent != "" || m.fileContentKey != "" || m.commitDiffKey != "" || m.renderedCommitDiff != "" {
		t.Fatalf("loaded detail state not cleared")
	}
	if m.diffLoadKey != "org/repo#1@new" {
		t.Fatalf("diffLoadKey = %q, want new head fetch key", m.diffLoadKey)
	}
	if m.diffMode != DiffModeHunks || m.fileIndex != 0 || m.commitIndex != 0 || m.diffXOffset != 0 || m.summaryXOffset != 0 {
		t.Fatalf("view indexes not reset")
	}
}

func TestDiffMsgIgnoresStaleHeadResult(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "first", HeadRefOID: "new"}}, 0)
	m.diffEnabled = true
	m.diffLoadKey = "org/repo#1@new"

	next, _ := m.Update(diffMsg{repo: "org/repo", number: 1, key: "org/repo#1@old", raw: "old raw", rendered: "old rendered"})
	m = next.(Model)

	if m.diff != "" || m.rawDiff != "" {
		t.Fatalf("stale diff applied: diff=%q raw=%q", m.diff, m.rawDiff)
	}
}

func TestTickPollsInboxAndSelectedPR(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "first"}}, 0)
	m.loading = false

	next, cmd := m.Update(tickMsg{})
	m = next.(Model)

	if cmd == nil {
		t.Fatal("tick should schedule polling commands")
	}
	if !m.loading {
		t.Fatal("tick should mark model loading")
	}
	if m.message != "polling inbox" {
		t.Fatalf("message = %q, want polling inbox", m.message)
	}
	sequence := cmd()
	commands := reflect.ValueOf(sequence)
	if fmt.Sprintf("%T", sequence) != "tea.sequenceMsg" || commands.Len() != 3 {
		t.Fatalf("tick command = %#v, want sequential inbox, selected PR, and next tick", sequence)
	}
}

// TestMouseClickWideLayoutEmptyQueueChangesFocus verifies that in the wide
// 3-pane layout with an empty queue, clicking the Summary and Diff panes still
// moves focus to that pane (focus change only; no PR selection required).
func TestMouseClickWideLayoutEmptyQueueChangesFocus(t *testing.T) {
	m := minimalModel()
	m.prs = nil
	m.allPRs = nil
	m.width = 120
	m.height = 30
	m.mode = ModeInbox
	m.focus = FocusInbox
	m.resizeViewports()

	// Click the summary pane (lower-left in wide inbox-focused layout).
	m = clickMouse(m, 2, 24)
	if m.focus != FocusSummary {
		t.Fatalf("summary click: focus = %v, want FocusSummary", m.focus)
	}

	// Click the diff pane (right column).
	m = clickMouse(m, 90, 5)
	if m.focus != FocusDiff {
		t.Fatalf("diff click: focus = %v, want FocusDiff", m.focus)
	}
}

// TestSplitDiffFilesRaw verifies that splitDiffFiles correctly splits a raw
// unified diff (with "diff --git" markers) into per-file chunks.
func TestSplitDiffFilesRaw(t *testing.T) {
	raw := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1 +1 @@
-old
+new
diff --git a/bar.go b/bar.go
--- a/bar.go
+++ b/bar.go
@@ -1 +1 @@
-x
+y
`
	files := splitDiffFiles(raw)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Path != "foo.go" {
		t.Errorf("files[0].Path = %q, want %q", files[0].Path, "foo.go")
	}
	if files[1].Path != "bar.go" {
		t.Errorf("files[1].Path = %q, want %q", files[1].Path, "bar.go")
	}
	// Each body must not contain the other file's content.
	if contains(files[0].Body, "bar.go") {
		t.Error("files[0].Body contains bar.go content")
	}
	if contains(files[1].Body, "foo.go") {
		t.Error("files[1].Body contains foo.go content")
	}
}

// TestSplitDiffFilesRenderedDeltaOutput verifies that splitting a
// delta-rendered diff (which rewrites "diff --git" header lines) returns only
// a single chunk — demonstrating WHY we split on the raw diff instead.
// renderDiffFiles must be used for correct per-file rendering.
func TestSplitDiffFilesRenderedDeltaOutput(t *testing.T) {
	// Simulate what delta does: it replaces the "diff --git" line with styled
	// output that no longer starts with "diff --git ".
	deltaRendered := `━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
foo.go
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 1  │-old
 1  │+new
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
bar.go
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 1  │-x
 1  │+y
`
	files := splitDiffFiles(deltaRendered)
	// Delta output has no "diff --git" lines, so everything is one chunk (or zero).
	if len(files) > 1 {
		t.Fatalf("expected 0 or 1 chunks from delta output (no git markers), got %d", len(files))
	}
}

func TestInboxTitleDoesNotChangePaneGeometryWithSelectedContent(t *testing.T) {
	short := model.PullRequest{Number: 1, Repo: "org/repo", Title: "short"}
	long := model.PullRequest{Number: 2, Repo: "org/repo", Title: strings.Repeat("long-title-", 12)}

	base := modelWithPRs([]model.PullRequest{short, long}, 0)
	base.width = 90
	base.height = 24
	base.mode = ModeInbox
	shortView := base.View()

	base.selected = 1
	longView := base.View()

	if got, want := lipWidthOfFirstLine(shortView), lipWidthOfFirstLine(longView); got != want {
		t.Fatalf("pane width changed with selected PR content: %d vs %d", got, want)
	}
	if got, want := len(strings.Split(shortView, "\n")), len(strings.Split(longView, "\n")); got != want {
		t.Fatalf("pane height changed with selected PR content: %d vs %d", got, want)
	}
}

func TestDiffHorizontalScrollMovesXOffset(t *testing.T) {
	m := minimalModel()
	m.width = 120
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("abcdefghijklmnopqrstuvwxyz", 8)
	m.syncDiffVP()

	initial := m.diffPanel(m.width, m.height)
	if !strings.Contains(initial, "abcd") {
		t.Fatalf("initial diff view missing expected prefix: %q", initial)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	afterRight := m.diffPanel(m.width, m.height)
	if afterRight == initial {
		t.Fatal("diff view did not change after right key")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(Model)
	afterLeft := m.diffPanel(m.width, m.height)
	if afterLeft != initial {
		t.Fatalf("diff view not restored after left key")
	}
}

func TestInboxSelectedPRStaysWithinWidth(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number:   1,
		Repo:     "org/repo",
		Title:    "very long title with wide chars 測試 測試 測試",
		Author:   "author",
		CIStatus: "passing",
		Skipped:  true,
	}}, 0)
	m.width = 28
	m.height = 12
	view := m.inboxPanel(m.width, m.height)
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(stripANSI(line)) > m.width-4 {
			t.Fatalf("line over width: %q", line)
		}
	}
	if strings.Contains(view, ">") {
		t.Fatalf("selected inbox row still has cursor: %q", view)
	}
}

func TestInboxTitleDropsZeroPadding(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 12}, {Number: 11}}, 0)
	if got := m.inboxTitle(); got != "Inbox (1/2)" {
		t.Fatalf("inboxTitle = %q, want %q", got, "Inbox (1/2)")
	}
}

func TestInboxScrollKeepsSelectedPRVisible(t *testing.T) {
	prs := make([]model.PullRequest, 8)
	for i := range prs {
		prs[i] = model.PullRequest{Number: i + 1, Repo: "org/repo", Title: strings.Repeat("item-", i+1), Author: "author", CIStatus: "passing"}
	}
	m := modelWithPRs(prs, len(prs)-1)
	m.width = 48
	m.height = 12
	view := m.inboxPanel(m.width, m.height)
	if !strings.Contains(view, prs[len(prs)-1].Title) {
		t.Fatalf("inbox view missing selected PR title %q: %q", prs[len(prs)-1].Title, view)
	}
	if !strings.Contains(view, "CI pass") || !strings.Contains(view, "#8 · author") {
		t.Fatalf("inbox view missing full selected PR details: %q", view)
	}
}

func TestLayoutUsesFullHeight(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author", CIStatus: "passing"}}, 0)
	m.width = 120
	m.height = 20
	view := m.View()
	got := len(strings.Split(strings.TrimRight(view, "\n"), "\n"))
	if got != m.height {
		t.Fatalf("layout height = %d, want %d", got, m.height)
	}
}

func TestLayoutUsesFullWidth(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author", CIStatus: "passing"}}, 0)
	m.width = 120
	m.height = 20
	view := m.View()
	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if got := lipgloss.Width(stripANSI(line)); got != m.width {
			t.Fatalf("line width = %d, want %d: %q", got, m.width, line)
		}
	}
}

func TestDiffVerticalScrollMovesYOffset(t *testing.T) {
	m := minimalModel()
	m.width = 120
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("line\n", 80)
	m.resizeViewports()

	initial := m.diffVP.YOffset
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.diffVP.YOffset <= initial {
		t.Fatalf("diff YOffset did not move down after down key: %d -> %d", initial, m.diffVP.YOffset)
	}

	afterDown := m.diffVP.YOffset
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = next.(Model)
	if m.diffVP.YOffset <= afterDown {
		t.Fatalf("diff YOffset did not move down after pgdown: %d -> %d", afterDown, m.diffVP.YOffset)
	}
}

func TestSummarySelectedFileFlushLeftAndHighlighted(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		Title:  "title",
		Author: "author",
		Files:  []model.ChangedFile{{Path: "a/very/long/path/name.go", Status: "M"}, {Path: "other.go", Status: "A"}},
	}}, 0)
	m.width = 40
	m.height = 14
	m.focus = FocusSummary
	m.fileIndex = 0
	view := m.summaryPanel(m.width, m.height)
	if strings.Contains(view, ">") {
		t.Fatalf("selected summary row still has cursor: %q", view)
	}
	if !strings.Contains(view, "name.go") {
		t.Fatalf("selected file path missing: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(stripANSI(line)) > m.width-4 {
			t.Fatalf("summary line over width: %q", line)
		}
	}
}

func TestTruncateANSIWidthUsesRequestedWidth(t *testing.T) {
	got := lipgloss.Width(stripANSI(truncateANSIWidth("abcdef", 6)))
	if got != 6 {
		t.Fatalf("truncate width = %d, want 6", got)
	}
}

func TestLeftTruncatePath(t *testing.T) {
	cases := []struct {
		path string
		maxW int
		want string
	}{
		// Fits: no truncation.
		{"sa.tf", 20, "sa.tf"},
		// First segment stripped.
		{"terraform/accounts/sa.tf", 20, "…/accounts/sa.tf"},
		// Two segments stripped.
		{"terraform/accounts/emea/eks/sa.tf", 20, "…/emea/eks/sa.tf"},
		// Falls back to last segment only.
		{"terraform/accounts/emea/eks/sa.tf", 10, "…/sa.tf"},
		// Filename itself wider than maxW: hard right-truncate with ellipsis.
		{"verylongfilename.tf", 5, "…e.tf"},
		// Zero width.
		{"terraform/sa.tf", 0, ""},
	}
	for _, tc := range cases {
		got := leftTruncatePath(tc.path, tc.maxW)
		if got != tc.want {
			t.Errorf("leftTruncatePath(%q, %d) = %q, want %q", tc.path, tc.maxW, got, tc.want)
		}
	}
}

func TestLeftColumnHeightsShiftByFocus(t *testing.T) {
	m := minimalModel()
	inboxH, summaryH := m.leftColumnHeights(20)
	if inboxH != 16 || summaryH != 4 {
		t.Fatalf("inbox focus heights = %d/%d, want 16/4", inboxH, summaryH)
	}
	m.focus = FocusSummary
	m.mode = ModeDetail
	inboxH, summaryH = m.leftColumnHeights(20)
	if inboxH != 4 || summaryH != 16 {
		t.Fatalf("summary focus heights = %d/%d, want 4/16", inboxH, summaryH)
	}
}

func TestNumberKeysSwitchSummaryModeGlobally(t *testing.T) {
	m := minimalModel()
	m = pressKey(m, "2")
	if m.summaryMode != SummaryModeCommits {
		t.Fatalf("summaryMode after 2 = %v, want %v", m.summaryMode, SummaryModeCommits)
	}
	m = pressKey(m, "3")
	if m.summaryMode != SummaryModeComments {
		t.Fatalf("summaryMode after 3 = %v, want %v", m.summaryMode, SummaryModeComments)
	}
	m = pressKey(m, "4")
	if m.summaryMode != SummaryModeAtlantis {
		t.Fatalf("summaryMode after 4 = %v, want %v", m.summaryMode, SummaryModeAtlantis)
	}
	m = pressKey(m, "1")
	if m.summaryMode != SummaryModeFiles {
		t.Fatalf("summaryMode after 1 = %v, want %v", m.summaryMode, SummaryModeFiles)
	}
}

func TestCommandOverlayPreservesSelectedInboxColors(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number:   1,
		Repo:     "org/repo",
		Title:    "selected title",
		Author:   "author",
		CIStatus: "passing",
	}}, 0)
	m.width = 100
	m.height = 20
	m.theme = theme.Default()
	m.mode = ModeCommand
	m.focus = FocusInbox
	m.resizeViewports()

	view := m.View()
	want := m.theme.Styles.TitleStrong.Render("selected title")
	if !strings.Contains(view, want) {
		t.Fatalf("selected title colors not preserved; missing %q in %q", want, view)
	}
}

func TestApprovalViewUsesModalOverlayWhenSized(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number:   1,
		Repo:     "org/repo",
		Title:    "selected title",
		Author:   "author",
		CIStatus: "passing",
	}}, 0)
	m.width = 100
	m.height = 20
	m.theme = theme.Default()
	m.mode = ModeConfirmApprove
	m.focus = FocusInbox
	m.resizeViewports()

	view := stripANSI(m.View())
	if !strings.Contains(view, "selected title") || !strings.Contains(view, "Inbox (1/1)") {
		t.Fatalf("approval view should render over inbox layout: %q", view)
	}
	if strings.Contains(view, "Press y to approve") {
		t.Fatalf("approval view should not use legacy full-screen prompt: %q", view)
	}
	if !strings.Contains(view, m.actionAlias("approve-confirm")+" approve  "+m.actionAlias("approve-cancel")+" cancel") {
		t.Fatalf("approval view missing modal controls: %q", view)
	}
}

func TestAtlantisSummaryShowsCondensedPlan(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		Title:  "title",
		Author: "author",
		AtlantisPlan: &model.AtlantisPlan{
			Author:  "atlantis",
			Summary: "Plan: 1 to add, 2 to change, 3 to destroy",
			Add:     1,
			Change:  2,
			Destroy: 3,
			Body:    "noisy raw body",
			Layers: []model.AtlantisLayer{
				{Name: "network/prod", Add: 1, Change: 2, Destroy: 3},
				{Name: "app/dev", NoChanges: true},
			},
		},
	}}, 0)
	m.width = 60
	m.height = 16
	m.summaryMode = SummaryModeAtlantis

	view := stripANSI(m.summaryPanel(m.width, m.height))
	if !strings.Contains(view, "Atlantis plan") || !strings.Contains(view, "+1 ~2 -3") || !strings.Contains(view, "Changes") || !strings.Contains(view, "network/prod") || !strings.Contains(view, "No changes") || !strings.Contains(view, "app/dev") {
		t.Fatalf("missing Atlantis summary: %q", view)
	}
	if strings.Contains(view, "noisy raw body") || strings.Contains(view, "Unknown") {
		t.Fatalf("Atlantis summary should omit raw body and unknown label: %q", view)
	}
}

func TestAtlantisSummaryUsesAffectedLayersForUncountedLayers(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		AtlantisPlan: &model.AtlantisPlan{
			Author:  "atlantis",
			Summary: "Plan: 0 to add, 2 to change, 0 to destroy",
			Change:  2,
			Layers: []model.AtlantisLayer{
				{Name: "terraform/accounts/prod / default", Unknown: true},
			},
		},
	}}, 0)
	m.width = 80
	m.height = 16
	m.summaryMode = SummaryModeAtlantis

	view := stripANSI(m.summaryPanel(m.width, m.height))
	if !strings.Contains(view, "Affected layers") || !strings.Contains(view, "terraform/accounts/prod / default") {
		t.Fatalf("missing affected layers: %q", view)
	}
	if strings.Contains(view, "Unknown") || strings.Contains(view, "Plan Summary") || strings.Contains(view, "unchanged attributes hidden") {
		t.Fatalf("Atlantis summary leaked noisy labels: %q", view)
	}
}

func TestCommentSummaryStripsEmojiShortcodes(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		Title:  "title",
		Author: "author",
		Comments: []model.PRComment{{
			Author: "alice",
			Body:   ":rocket: ship it :white_check_mark:",
		}},
	}}, 0)
	m.width = 60
	m.height = 14
	m.summaryMode = SummaryModeComments

	view := stripANSI(m.summaryPanel(m.width, m.height))
	if strings.Contains(view, ":rocket:") || strings.Contains(view, ":white_check_mark:") {
		t.Fatalf("emoji shortcodes not stripped: %q", view)
	}
	if !strings.Contains(view, "ship it") {
		t.Fatalf("comment text missing: %q", view)
	}
}

func TestCommentSummaryWrapsLongLines(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		Title:  "title",
		Author: "author",
		Comments: []model.PRComment{{
			Author: "atlantis",
			Body:   strings.Repeat("0123456789", 12),
		}},
	}}, 0)
	m.width = 44
	m.height = 14
	m.summaryMode = SummaryModeComments
	view := m.summaryPanel(m.width, m.height)
	lines := strings.Split(view, "\n")
	bodyLines := 0
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(stripANSI(line)) > m.width-4 {
			t.Fatalf("comment summary line over width: %q", line)
		}
		if strings.Contains(line, "0123456789") {
			bodyLines++
		}
	}
	if bodyLines != 1 {
		t.Fatalf("long comment body wrapped into %d lines, want 1: %#v", bodyLines, lines)
	}
}

func TestCommitSummaryUsesAvailableWidth(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{
		Number: 1,
		Repo:   "org/repo",
		Title:  "title",
		Author: "author",
		Commits: []model.PRCommit{{
			OID:      "1234567890abcdef",
			Headline: strings.Repeat("x", 80),
			Author:   "alice",
		}},
	}}, 0)
	m.width = 44
	m.height = 14
	m.summaryMode = SummaryModeCommits
	view := m.summaryPanel(m.width, m.height)
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		t.Fatal("empty commit summary view")
	}
	if got := lipgloss.Width(stripANSI(lines[0])); got < m.width-10 {
		t.Fatalf("commit headline used too little width: %d", got)
	}
}

func TestDiffPanelHonorsPassedWidth(t *testing.T) {
	m := minimalModel()
	m.width = 140
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("abcdefghijklmnopqrstuvwxyz", 8)
	m.syncDiffVP()

	paneW := 44
	view := m.diffPanel(paneW, 10)
	maxLineW := m.diffContentWidth(m.paneInnerWidth(paneW, FocusDiff))
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(stripANSI(line)); got != maxLineW {
			t.Fatalf("diff line width = %d, want %d: %q", got, maxLineW, line)
		}
	}
}

func TestDiffPanelHorizontalScrollShowsLaterContentWithoutWrapping(t *testing.T) {
	m := minimalModel()
	m.width = 120
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", 4)
	m.resizeViewports()

	initial := m.diffPanel(m.diffPaneOuterWidth(), 10)
	if !strings.Contains(initial, "0123456789") {
		t.Fatalf("initial diff missing prefix: %q", initial)
	}

	m = pressSpecial(m, tea.KeyRight)
	afterRight := m.diffPanel(m.diffPaneOuterWidth(), 10)
	if afterRight == initial {
		t.Fatal("diff did not change after horizontal scroll")
	}
	for _, line := range strings.Split(afterRight, "\n") {
		if got := lipgloss.Width(stripANSI(line)); got != m.diffVP.Width {
			t.Fatalf("scrolled diff line width = %d, want %d: %q", got, m.diffVP.Width, line)
		}
	}
}

func TestWideLayoutWithDiffUsesFullWidth(t *testing.T) {
	for _, width := range []int{119, 120, 121} {
		m := modelWithPRs([]model.PullRequest{{
			Number: 1,
			Repo:   "org/repo",
			Title:  "title",
			Author: "author",
		}}, 0)
		m.width = width
		m.height = 20
		m.mode = ModeDiff
		m.focus = FocusDiff
		m.diffEnabled = true
		m.diff = strings.Repeat("line\n", 20)
		m.resizeViewports()

		view := m.View()
		lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
		for _, line := range lines {
			if got := lipgloss.Width(stripANSI(line)); got != m.width {
				t.Fatalf("width %d: line width = %d, want %d: %q", width, got, m.width, line)
			}
		}
	}
}

func TestWideDiffHardTruncatesBeforeRightPadding(t *testing.T) {
	for _, width := range []int{119, 120, 121} {
		m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
		m.width = width
		m.height = 18
		m.mode = ModeDiff
		m.focus = FocusDiff
		m.diffEnabled = true
		m.diff = m.theme.Styles.File.Render("terraform/accounts/emea-production/storage/kafka.tf") + "\n" + strings.Repeat("data.terraform_remote_state.vpc_state.outputs.vpc_id", 4)
		m.resizeViewports()

		paneW := m.diffPaneOuterWidth()
		want := m.diffContentWidth(m.paneInnerWidth(paneW, FocusDiff))
		if m.diffVP.Width != want {
			t.Fatalf("width %d: diff viewport width = %d, want %d", width, m.diffVP.Width, want)
		}
		for _, line := range strings.Split(m.diffPanel(paneW, 8), "\n") {
			if got := lipgloss.Width(stripANSI(line)); got != want {
				t.Fatalf("width %d: diff line width = %d, want %d: %q", width, got, want, line)
			}
		}
	}
}

func TestWideLayoutLongDiffDoesNotWrapOrChangeHeight(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
	m.width = 120
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("data.terraform_remote_state.vpc_state.outputs.vpc_id", 6)
	m.resizeViewports()

	view := m.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) != m.height {
		t.Fatalf("view height = %d, want %d", len(lines), m.height)
	}
	for _, line := range lines {
		if got := lipgloss.Width(stripANSI(line)); got != m.width {
			t.Fatalf("line width = %d, want %d: %q", got, m.width, line)
		}
	}
}

func TestWideLayoutTabbedDiffDoesNotExceedTerminalWidth(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
	m.width = 120
	m.height = 20
	m.mode = ModeDiff
	m.focus = FocusDiff
	m.diffEnabled = true
	m.diff = strings.Repeat("\tdata.terraform_remote_state.vpc_state.outputs.vpc_id", 6)
	m.resizeViewports()

	for _, line := range strings.Split(strings.TrimRight(m.View(), "\n"), "\n") {
		if strings.Contains(line, "\t") {
			t.Fatalf("rendered line still contains tab: %q", line)
		}
		if got := lipgloss.Width(stripANSI(line)); got > m.width {
			t.Fatalf("line width = %d, want <= %d: %q", got, m.width, line)
		}
	}
}

func TestSummaryHeaderShowsActiveModeAndFooterDropsShortcut(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
	m.width = 100
	m.height = 18
	m.mode = ModeDetail
	m.focus = FocusSummary
	m.summaryMode = SummaryModeCommits
	view := m.View()
	if strings.Contains(view, "1/2/3/4 summary") {
		t.Fatalf("footer still shows summary shortcut: %q", view)
	}
	if !strings.Contains(stripANSI(view), "Summary [1] [2 commits] [3] [4]") {
		t.Fatalf("summary header missing mode numbers: %q", view)
	}
}

func TestRKeyRefreshesSelectedPR(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
	m.mode = ModeDetail

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = next.(Model)

	if cmd == nil {
		t.Fatal("r should return selected refresh command")
	}
	if m.statusErr || m.statusText != "refreshing PR…" {
		t.Fatalf("status = (%v, %q), want refreshing PR", m.statusErr, m.statusText)
	}
}

func TestRefreshAllCommandRefreshesInbox(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
	m.mode = ModeCommand
	m.prevMode = ModeDetail
	m.commandInput = "refresh-all"

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	if cmd == nil {
		t.Fatal(":refresh-all should return refresh command")
	}
	if !m.loading || m.message != "refreshing" {
		t.Fatalf("loading/message = %v/%q, want true/refreshing", m.loading, m.message)
	}
	if m.statusText != "refreshing…" {
		t.Fatalf("statusText = %q, want refreshing", m.statusText)
	}
}

// TestStripNonSGRRemovesEraseLineKeepsColor verifies that non-SGR CSI escape
// sequences emitted by delta (e.g. ESC[0K) are stripped while SGR color codes
// are preserved. Leaked ESC[0K caused diff lines to wrap/misalign in the
// terminal because it measures as zero width but erases columns when rendered.
func TestStripNonSGRRemovesEraseLineKeepsColor(t *testing.T) {
	in := "\x1b[31mred\x1b[0m text\x1b[0K end\x1b[2J\x1b[?25h done"
	got := stripNonSGR(in)
	if !strings.Contains(got, "\x1b[31m") || !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("stripNonSGR dropped SGR color codes: %q", got)
	}
	for _, seq := range []string{"\x1b[0K", "\x1b[2J", "\x1b[?25h"} {
		if strings.Contains(got, seq) {
			t.Fatalf("stripNonSGR kept non-SGR CSI %q: %q", seq, got)
		}
	}
	if stripANSI(got) != "red text end done" {
		t.Fatalf("unexpected visible text: %q", stripANSI(got))
	}
}

// TestPaneRendersExactAllocatedWidth guards Bug 2: lipgloss Width() sets the
// content+padding area and adds the border outside, so pane() must size the box
// with width-borderSize, not width-frameSize, or every pane renders 2 cols too
// narrow (leaving dead space at the terminal's right edge).
func TestPaneRendersExactAllocatedWidth(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo", Title: "x"}}, 0)
	m.theme = theme.Default()
	m.width = 120
	m.height = 24
	for _, focus := range []Focus{FocusInbox, FocusDiff} {
		for _, w := range []int{10, 30, 69, 100} {
			box := m.pane("Title", "content line", w, 6, focus)
			top := strings.SplitN(box, "\n", 2)[0]
			if got := lipgloss.Width(stripANSI(top)); got != w {
				t.Fatalf("focus=%v alloc=%d: pane outer width = %d, want %d", focus, w, got, w)
			}
		}
	}
}

// TestWideLayoutBodyFillsTerminalWidth guards Bug 2 end-to-end with the real
// bordered theme: every rendered line (incl. the diff pane right border) must
// span exactly the terminal width, leaving no dead space on the right.
func TestWideLayoutBodyFillsTerminalWidth(t *testing.T) {
	for _, width := range []int{110, 113, 120, 160, 200} {
		m := modelWithPRs([]model.PullRequest{{Number: 7933, Repo: "org/repo", Title: "title", Author: "author"}}, 0)
		m.theme = theme.Default()
		m.width = width
		m.height = 24
		m.mode = ModeDetail
		m.focus = FocusDiff
		m.diffEnabled = true
		m.diff = strings.Repeat("| EBS latency, IOPS saturation, full disk | ebs.md | EC2 |\n", 20)
		m.resizeViewports()
		for i, line := range strings.Split(m.View(), "\n") {
			if got := lipgloss.Width(stripANSI(line)); got != width {
				t.Fatalf("width=%d line %d width = %d, want %d: %q", width, i, got, width, stripANSI(line))
			}
		}
	}
}

// TestDiffContentNoLeakedNonSGREscapes ensures non-SGR CSI escapes are removed
// from delta output at ingest (guards Bug 1).
func TestDiffContentNoLeakedNonSGREscapes(t *testing.T) {
	in := "162\x1b[0K| line one |\n163| line two |\x1b[0K\n"
	if csiNonSGRPattern.MatchString(stripNonSGR(in)) {
		t.Fatalf("non-SGR CSI escape survived stripNonSGR: %q", stripNonSGR(in))
	}
}

func TestColorschemePicker(t *testing.T) {
	m := modelWithPRs([]model.PullRequest{{Number: 1, Repo: "org/repo"}}, 0)

	// Open via the colorscheme action.
	action, _ := m.actions.actionByID("colorscheme")
	next, _ := m.executeAction(action)
	m = next.(Model)
	if m.mode != ModeColorscheme {
		t.Fatalf("mode = %v, want ModeColorscheme", m.mode)
	}

	// j moves cursor down
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.colorschemeIndex != 1 {
		t.Fatalf("colorschemeIndex = %d, want 1 after j", m.colorschemeIndex)
	}

	// esc reverts and closes
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = next.(Model)
	if m.mode == ModeColorscheme {
		t.Fatal("mode should not be ModeColorscheme after esc")
	}
}

func lipWidthOfFirstLine(s string) int {
	line := strings.SplitN(s, "\n", 2)[0]
	return lipgloss.Width(stripANSI(line))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
