package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/model"
)

func TestInboxSelectionScrollsWithContextAndStopsAtEnds(t *testing.T) {
	for _, tt := range []struct {
		name   string
		wrap   bool
		height int
		want   []int
		wantUp []int
	}{
		{
			name: "truncated", height: 18,
			want:   []int{0, 0, 0, 3, 6, 9, 12, 15, 15, 15},
			wantUp: []int{15, 15, 12, 9, 6, 3, 0, 0, 0},
		},
		{
			name: "wrapped", wrap: true, height: 26,
			want:   []int{0, 0, 0, 1, 4, 8, 13, 16, 16, 16},
			wantUp: []int{16, 16, 15, 12, 7, 3, 0, 0, 0},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			prs := make([]model.PullRequest, 10)
			for i := range prs {
				prs[i] = model.PullRequest{Number: i + 1, Title: strings.Repeat("x", 20*(i%3+1))}
			}
			m := modelWithPRs(prs, 0)
			m.width, m.height, m.cfg.Wrap.Inbox = 24, tt.height, tt.wrap
			m.resizeViewports()
			for selected, want := range tt.want {
				if selected > 0 {
					next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
					m = next.(Model)
				}
				if got := m.inboxVP.YOffset(); got != want {
					t.Fatalf("selection %d: offset=%d, want %d", selected, got, want)
				}
				if got := m.inboxOffset(m.inboxVP.Width(), m.inboxVP.Height()); got != want {
					t.Fatalf("selection %d: render offset=%d, want %d", selected, got, want)
				}
				_, spans := m.inboxContent(m.inboxVP.Width())
				if spans[min(selected+2, len(spans)-1)].end > want+m.inboxVP.Height() {
					t.Fatalf("selection %d: following PRs not fully visible", selected)
				}
			}
			for _, want := range tt.wantUp {
				next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
				m = next.(Model)
				if got := m.inboxVP.YOffset(); got != want {
					t.Fatalf("moving up to selection %d: offset=%d, want %d", m.selected, got, want)
				}
				if got := m.inboxOffset(m.inboxVP.Width(), m.inboxVP.Height()); got != want {
					t.Fatalf("moving up to selection %d: render offset=%d, want %d", m.selected, got, want)
				}
				_, spans := m.inboxContent(m.inboxVP.Width())
				if spans[max(0, m.selected-2)].start < want {
					t.Fatalf("selection %d: preceding PRs not fully visible", m.selected)
				}
			}
		})
	}
}

func TestInboxScrollMarginKeepsSelectionVisibleInSmallPane(t *testing.T) {
	m := modelWithPRs(make([]model.PullRequest, 10), 4)
	m.width, m.height = 24, 8
	m.resizeViewports()
	_, spans := m.inboxContent(m.inboxVP.Width())
	span := spans[m.selected]
	offset := m.inboxVP.YOffset()
	if offset <= span.end-m.inboxVP.Height() {
		t.Fatalf("small pane has no look-ahead margin: offset=%d", offset)
	}
	if span.start < offset || span.end > offset+m.inboxVP.Height() {
		t.Fatalf("margin hid selection: span=%+v offset=%d height=%d", span, offset, m.inboxVP.Height())
	}
	// Approaching the top edge must reserve space above without hiding the PR.
	m.inboxVP.SetYOffset(span.start)
	m.syncInboxVP()
	offset = m.inboxVP.YOffset()
	if offset >= span.start {
		t.Fatalf("small pane has no preceding context: offset=%d", offset)
	}
	if span.start < offset || span.end > offset+m.inboxVP.Height() {
		t.Fatalf("top margin hid selection: span=%+v offset=%d height=%d", span, offset, m.inboxVP.Height())
	}
}
