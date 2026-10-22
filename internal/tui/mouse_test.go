package tui

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/senilio/zappr/internal/model"
)

func wheel(button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{Action: tea.MouseActionPress, Button: button}
}

func focusedViewport(m *Model) *viewport.Model {
	switch m.focus {
	case FocusSummary:
		return &m.summaryVP
	case FocusDiff:
		return &m.diffVP
	default:
		return &m.inboxVP
	}
}

func TestMouseWheelDiffAfterIdenticalReload(t *testing.T) {
	m := scrollModel(t, 1)
	m.diffEnabled = true
	pr := m.prs[0]
	loaded := diffMsg{repo: pr.Repo, number: pr.Number, key: prRefreshKey(pr), rendered: strings.Repeat("diff line\n", 100)}
	next, _ := m.Update(loaded)
	m = next.(Model)
	region := m.paneRegions()[0]
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: region.x + 2, Y: region.y + 1})
	m = next.(Model)
	if m.diff != "" {
		t.Fatal("click did not clear the loaded diff")
	}
	next, _ = m.Update(loaded)
	m = next.(Model)
	for _, region = range m.paneRegions() {
		if region.focus == FocusDiff {
			break
		}
	}
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: region.x + 2, Y: region.y + 1})
	m = next.(Model)
	msg := wheel(tea.MouseButtonWheelDown)
	if FilterMouseWheel(m, msg) == nil {
		t.Fatalf("reloaded diff wheel was filtered: viewport has %d rows", m.diffVP.TotalLineCount())
	}
	before := m.View()
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.diffVP.YOffset == 0 || m.View() == before {
		t.Fatal("reloaded diff did not visibly scroll")
	}
}

func TestMouseWheelSummaryUsesVisibleBounds(t *testing.T) {
	m := scrollModel(t, 100)
	for i := range 100 {
		m.prs[0].Files = append(m.prs[0].Files, model.ChangedFile{Path: fmt.Sprintf("file%d.go", i)})
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.focus != FocusSummary || m.mode != ModeDetail {
		t.Fatal("tab did not focus Summary")
	}
	var region paneRegion
	for _, region = range m.paneRegions() {
		if region.focus == FocusSummary {
			break
		}
	}
	visibleHeight := max(1, region.h-paneFrame)
	if m.summaryVP.Height != visibleHeight {
		t.Errorf("stored Summary height = %d, visible height = %d", m.summaryVP.Height, visibleHeight)
	}
	m.summaryVP.SetYOffset(max(0, m.summaryVP.TotalLineCount()-visibleHeight))
	before := m.summaryPanel(region.w, region.h)
	msg := wheel(tea.MouseButtonWheelDown)
	if FilterMouseWheel(m, msg) != nil {
		t.Error("downward wheel at visible Summary bottom was not filtered")
	}
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.summaryPanel(region.w, region.h) != before {
		t.Fatal("test was not at the visible Summary bottom")
	}
	// Completed actions return to Inbox mode without changing focus. The
	// changed left-column split must reach the filter before its next event.
	next, _ = m.Update(actionMsg{})
	m = next.(Model)
	for _, region = range m.paneRegions() {
		if region.focus == FocusSummary {
			break
		}
	}
	visibleHeight = max(1, region.h-paneFrame)
	if m.summaryVP.Height != visibleHeight {
		t.Fatalf("Summary height after mode change = %d, visible height = %d", m.summaryVP.Height, visibleHeight)
	}
	for range 100 {
		if FilterMouseWheel(m, msg) == nil {
			break
		}
		next, _ = m.Update(msg)
		m = next.(Model)
	}
	if m.summaryVP.YOffset != max(0, m.summaryVP.TotalLineCount()-visibleHeight) {
		t.Fatal("filter stopped scrolling before the visible Summary bottom")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'a'}}, {Type: tea.KeyEsc}} {
		next, _ = m.Update(key)
		m = next.(Model)
	}
	_, summaryH := m.leftColumnHeights(m.height - chromeLines)
	before = m.summaryPanel(region.w, summaryH)
	next, _ = m.Update(wheel(tea.MouseButtonWheelUp))
	m = next.(Model)
	if m.summaryPanel(region.w, summaryH) == before {
		t.Fatal("first upward wheel after Summary expansion did not visibly scroll")
	}
}

func TestMouseWheelFilterBoundaries(t *testing.T) {
	for _, focus := range []Focus{FocusInbox, FocusSummary, FocusDiff} {
		for _, count := range []int{0, 1, 100} {
			t.Run(fmt.Sprintf("focus%d/items%d", focus, count), func(t *testing.T) {
				m := scrollModel(t, count)
				m.focus, m.inboxManualScroll = focus, true
				vp := focusedViewport(&m)
				vp.SetContent(strings.Repeat("line\n", count))
				if got := FilterMouseWheel(m, wheel(tea.MouseButtonWheelUp)); got != nil {
					t.Error("upward wheel at top was not filtered")
				}
				if count == 100 && FilterMouseWheel(m, wheel(tea.MouseButtonWheelDown)) == nil {
					t.Error("valid downward movement was filtered")
				}
				vp.GotoBottom()
				if got := FilterMouseWheel(m, wheel(tea.MouseButtonWheelDown)); got != nil {
					t.Error("downward wheel at bottom was not filtered")
				}
				if count == 100 && FilterMouseWheel(m, wheel(tea.MouseButtonWheelUp)) == nil {
					t.Error("valid upward movement was filtered")
				}
				if count <= 1 && FilterMouseWheel(m, wheel(tea.MouseButtonWheelUp)) != nil {
					t.Error("short pane wheel was not filtered")
				}
			})
		}
	}
}

func TestMouseWheelFilterPreservesFirstManualScroll(t *testing.T) {
	m := scrollModel(t, 100)
	msg := wheel(tea.MouseButtonWheelUp)
	if FilterMouseWheel(m, msg) == nil {
		t.Fatal("first wheel event must enter manual scrolling")
	}
	next, _ := m.Update(msg)
	m = next.(Model)
	if !m.inboxManualScroll {
		t.Fatal("first boundary event did not enter manual scrolling")
	}
	if FilterMouseWheel(m, msg) != nil {
		t.Fatal("subsequent ineffective wheel event was not filtered")
	}
}

func TestMouseWheelFilterHorizontal(t *testing.T) {
	m := scrollModel(t, 1)
	m.focus, m.diffEnabled = FocusDiff, true
	m.diff = strings.Repeat("long diff line ", 40) + "\nsecond line"
	m.syncDiffVP()
	for _, shift := range []bool{false, true} {
		left, right := wheel(tea.MouseButtonWheelLeft), wheel(tea.MouseButtonWheelRight)
		if shift {
			left, right = wheel(tea.MouseButtonWheelUp), wheel(tea.MouseButtonWheelDown)
			left.Shift, right.Shift = true, true
		}
		m.diffVP.SetXOffset(0)
		if FilterMouseWheel(m, left) != nil {
			t.Error("ineffective left scroll was not filtered")
		}
		if FilterMouseWheel(m, right) == nil {
			t.Error("valid horizontal scroll at vertical top was filtered")
		}
		m.diffVP.SetXOffset(20)
		before := m.diffVP.HorizontalScrollPercent()
		if FilterMouseWheel(m, left) == nil {
			t.Error("valid left scroll was filtered")
		}
		if m.diffVP.HorizontalScrollPercent() != before {
			t.Error("filter mutated the live viewport")
		}
		m.diffVP.SetXOffset(1 << 20)
		if FilterMouseWheel(m, right) != nil {
			t.Error("ineffective right scroll was not filtered")
		}
		m.cfg.Wrap.Diff = true
		if FilterMouseWheel(m, left) != nil {
			t.Error("horizontal diff scroll allowed while wrapping")
		}
		m.cfg.Wrap.Diff = false
	}
}

func TestMouseWheelFilterDisabledAndRelease(t *testing.T) {
	m := scrollModel(t, 100)
	m.inboxManualScroll = true
	m.inboxVP.MouseWheelEnabled = false
	if FilterMouseWheel(m, wheel(tea.MouseButtonWheelDown)) != nil {
		t.Error("disabled wheel event was not filtered")
	}
	m.inboxVP.MouseWheelEnabled = true
	msg := wheel(tea.MouseButtonWheelDown)
	msg.Action = tea.MouseActionRelease
	if FilterMouseWheel(m, msg) != nil {
		t.Error("ignored release was not filtered")
	}
}

func TestMouseWheelFilterLeavesOtherInputUntouched(t *testing.T) {
	m := scrollModel(t, 1)
	m.inboxManualScroll = true
	for _, msg := range []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}, tea.MouseMsg{Action: tea.MouseActionMotion}, tea.WindowSizeMsg{Width: 80, Height: 24}} {
		if FilterMouseWheel(m, msg) == nil {
			t.Errorf("non-wheel input %T was filtered", msg)
		}
	}
	for _, mode := range []Mode{ModeHelp, ModeComment, ModeSettings, ModeCommand, ModeConfirmCommand, ModeConfirmApprove, ModeColorscheme} {
		m.mode = mode
		if FilterMouseWheel(m, wheel(tea.MouseButtonWheelUp)) == nil {
			t.Errorf("modal input behavior changed in mode %d", mode)
		}
	}
	if FilterMouseWheel(filterTestProgram{}, wheel(tea.MouseButtonWheelUp)) == nil {
		t.Error("unknown model was filtered")
	}
}

func TestMouseWheelFilterUsesFreshInboxAfterRefresh(t *testing.T) {
	m := scrollModel(t, 100)
	m.inboxManualScroll = true
	m.inboxVP.GotoBottom()
	prs := append([]model.PullRequest(nil), m.prs...)
	prs = append(prs, model.PullRequest{Repo: "org/repo", Number: 101, Title: "new PR"})
	next, _ := m.Update(refreshMsg{prs: prs})
	m = next.(Model)
	if FilterMouseWheel(m, wheel(tea.MouseButtonWheelDown)) == nil {
		t.Fatal("new rows were not scrollable after refresh")
	}
}

type filterTestProgram struct {
	Model
	wheels, views *int
}

func (m filterTestProgram) Init() tea.Cmd { return nil }
func (m filterTestProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if mouse, ok := msg.(tea.MouseMsg); ok && tea.MouseEvent(mouse).IsWheel() {
		*m.wheels++
	}
	next, cmd := m.Model.Update(msg)
	m.Model = next.(Model)
	return m, cmd
}
func (m filterTestProgram) View() string { *m.views++; return m.Model.View() }

// Deliver complete terminal sequences so read-buffer boundaries do not affect
// how many synthetic wheel messages Bubble Tea decodes.
type wheelBurstReader struct{ remaining int }

func (r *wheelBurstReader) Read(p []byte) (int, error) {
	if r.remaining > 0 {
		r.remaining--
		return copy(p, "\x1b[<64;2;2M"), nil
	}
	if r.remaining == 0 {
		r.remaining--
		return copy(p, "q"), nil
	}
	return 0, io.EOF
}

func TestMouseWheelBurstSkipsUpdateAndView(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("filter=%t", enabled), func(t *testing.T) {
			m := scrollModel(t, 100)
			m.inboxManualScroll = true
			wheels, views := 0, 0
			options := []tea.ProgramOption{tea.WithInput(&wheelBurstReader{remaining: 200}), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignals()}
			if enabled {
				options = append(options, tea.WithFilter(func(current tea.Model, msg tea.Msg) tea.Msg {
					return FilterMouseWheel(current.(filterTestProgram).Model, msg)
				}))
			}
			p := tea.NewProgram(filterTestProgram{m, &wheels, &views}, options...)
			if _, err := p.Run(); err != nil {
				t.Fatal(err)
			}
			if enabled {
				if wheels != 0 || views > 5 {
					t.Fatalf("ineffective burst reached Update/View: %d wheel updates, %d views", wheels, views)
				}
			} else if wheels != 200 {
				t.Fatalf("decoded %d wheel events, want 200", wheels)
			}
		})
	}
}

func BenchmarkMouseWheelBoundaryFilter(b *testing.B) {
	for _, count := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("PRs%d", count), func(b *testing.B) {
			m := scrollModel(b, count)
			m.inboxManualScroll = true
			current, msg := tea.Model(m), wheel(tea.MouseButtonWheelUp)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				FilterMouseWheel(current, msg)
			}
		})
	}
}
