package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/senilio/zappr/internal/model"
)

func summaryScrollModel(mode SummaryMode, wrap bool) Model {
	pr := model.PullRequest{}
	for i := range 10 {
		pr.Files = append(pr.Files, model.ChangedFile{Path: fmt.Sprintf("dir%d/file%d.go", i/2, i)})
		pr.Commits = append(pr.Commits, model.PRCommit{
			OID: fmt.Sprintf("%07d", i), Headline: strings.Repeat("x", 36*(i%3+1)), Author: "author",
		})
	}
	m := modelWithPRs([]model.PullRequest{pr}, 0)
	m.width, m.height = 40, 30
	m.focus, m.summaryMode, m.cfg.Wrap.Summary = FocusSummary, mode, wrap
	m.resizeViewports()
	return m
}

func TestSummarySelectionScrollsWithContextAndStopsAtEnds(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mode   SummaryMode
		wrap   bool
		height int
	}{
		{"files", SummaryModeFiles, false, 8},
		{"files with wrapping enabled", SummaryModeFiles, true, 8},
		{"commits", SummaryModeCommits, false, 10},
		{"wrapped commits", SummaryModeCommits, true, 26},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := summaryScrollModel(tt.mode, tt.wrap)
			m.summaryVP.Height = tt.height
			check := func(index int, down bool) {
				t.Helper()
				_, spans := m.summaryLayout(m.prs[0], m.summaryVP.Width)
				offset := m.summaryVP.YOffset
				span := spans[index]
				if span.start < offset || span.end > offset+tt.height {
					t.Fatalf("selection %d hidden: span=%+v offset=%d", index, span, offset)
				}
				if down && spans[min(index+2, len(spans)-1)].end > offset+tt.height {
					t.Fatalf("selection %d: following items not fully visible at offset %d", index, offset)
				}
				if !down && spans[max(0, index-2)].start < offset {
					t.Fatalf("selection %d: preceding items not fully visible at offset %d", index, offset)
				}
				if index == 0 && offset != 0 {
					t.Fatalf("top offset=%d, want 0", offset)
				}
				if index == len(spans)-1 && offset != spans[index].end-tt.height {
					t.Fatalf("bottom offset=%d, want %d", offset, spans[index].end-tt.height)
				}
				m.syncSummaryVPScroll()
				if m.summaryVP.YOffset != offset {
					t.Fatalf("selection %d: repeated sync changed offset", index)
				}
			}
			for index := range 10 {
				if index > 0 {
					next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
					m = next.(Model)
				}
				check(index, true)
			}
			for index := 8; index >= 0; index-- {
				next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
				m = next.(Model)
				check(index, false)
			}
		})
	}
}

func TestSummaryScrollMarginKeepsSelectionVisibleInSmallPane(t *testing.T) {
	for _, mode := range []SummaryMode{SummaryModeFiles, SummaryModeCommits} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			m := summaryScrollModel(mode, true)
			m.fileIndex, m.commitIndex = 4, 4
			m.syncSummaryVP()
			_, spans := m.summaryLayout(m.prs[0], m.summaryVP.Width)
			span := spans[4]
			m.summaryVP.Height = span.end - span.start + 2
			for _, start := range []int{0, span.start} {
				m.summaryVP.SetYOffset(start)
				m.syncSummaryVPScroll()
				if got, want := m.summaryVP.YOffset, span.start-1; got != want {
					t.Fatalf("initial offset %d: offset=%d, want %d for one row of context on each side", start, got, want)
				}
			}
		})
	}
}

func TestTallSummaryCommitStartsAtHeadlineAndAllowsPaging(t *testing.T) {
	m := summaryScrollModel(SummaryModeCommits, true)
	m.commitIndex = 4
	m.summaryVP.Height = 2
	m.syncSummaryVP()
	m.syncSummaryVPScroll()
	_, spans := m.summaryLayout(m.prs[0], m.summaryVP.Width)
	if got := m.summaryVP.YOffset; got != spans[4].start {
		t.Fatalf("tall commit offset=%d, want headline at %d", got, spans[4].start)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = next.(Model)
	offset := m.summaryVP.YOffset
	_ = m.summaryPanel(m.summaryVP.Width+paneHFrame, m.summaryVP.Height+paneFrame)
	if offset <= spans[4].start || m.summaryVP.YOffset != offset {
		t.Fatalf("manual paging did not retain offset: %d", m.summaryVP.YOffset)
	}
}
