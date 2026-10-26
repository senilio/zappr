package tui

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"github.com/charmbracelet/colorprofile"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

// Entries are immutable once published. Model copies can share them without
// changing earlier states; only the latest entry for each pane is retained.
type paneLayout struct {
	content  string
	spans    []rowSpan
	sources  []int
	lines    []string
	viewport viewport.Model
}

func newPaneLayout(content string, width int) *paneLayout {
	vp := viewport.New(viewport.WithWidth(width))
	vp.SetContent(content)
	return &paneLayout{content: content, viewport: vp}
}

type layoutTheme struct {
	palette uitheme.Palette
	profile colorprofile.Profile
	noColor bool
}

func (m Model) layoutTheme() layoutTheme {
	return layoutTheme{m.theme.Palette, m.theme.Profile, m.theme.NoColor}
}

type inboxItem struct {
	number                    int
	title, author, ci, review string
	draft, autoMerge          bool
}

func inboxRenderItem(pr model.PullRequest) inboxItem {
	return inboxItem{pr.Number, pr.Title, pr.Author, pr.CIStatus, pr.ReviewDecision, pr.Draft, pr.AutoMerge}
}

type inboxLayoutKey struct {
	width, selected int
	wrap            bool
	theme           layoutTheme
}

type inboxLayoutCache struct {
	key    inboxLayoutKey
	items  []inboxItem
	layout *paneLayout
}

func (m *Model) inboxPane(width int) *paneLayout {
	key := inboxLayoutKey{width, m.selected, m.cfg.Wrap.Inbox, m.layoutTheme()}
	if cached := m.inboxLayoutCache; cached != nil && cached.key == key && len(cached.items) == len(m.prs) {
		matches := true
		for i, pr := range m.prs {
			if cached.items[i] != inboxRenderItem(pr) {
				matches = false
				break
			}
		}
		if matches {
			return cached.layout
		}
	}
	content, spans := m.buildInboxContent(width)
	layout := newPaneLayout(content, width)
	layout.spans = spans
	items := make([]inboxItem, len(m.prs))
	for i, pr := range m.prs {
		items[i] = inboxRenderItem(pr)
	}
	m.inboxLayoutCache = &inboxLayoutCache{key, items, layout}
	return layout
}

func (m *Model) inboxContent(width int) (string, []rowSpan) {
	layout := m.inboxPane(width)
	return layout.content, layout.spans
}

type summaryLayoutKey struct {
	width, fileIndex, commitIndex, commentIndex, xOffset int
	mode                                                 SummaryMode
	wrap                                                 bool
	theme                                                layoutTheme
	second                                               int64
}

type summaryLayoutCache struct {
	key      summaryLayoutKey
	files    []model.ChangedFile
	commits  []model.PRCommit
	comments []model.PRComment
	layout   *paneLayout
}

func (m *Model) summaryPane(pr model.PullRequest, width int) *paneLayout {
	key := summaryLayoutKey{width: width, fileIndex: m.fileIndex, commitIndex: m.commitIndex, commentIndex: m.commentIndex, xOffset: m.summaryXOffset, mode: m.summaryMode, wrap: m.cfg.Wrap.Summary, theme: m.layoutTheme()}
	var files []model.ChangedFile
	var commits []model.PRCommit
	var comments []model.PRComment
	switch m.summaryMode {
	case SummaryModeFiles:
		files = pr.Files
	case SummaryModeCommits:
		commits = pr.Commits
		key.second = time.Now().Unix()
	case SummaryModeComments:
		comments = pr.Comments
		key.second = time.Now().Unix()
	}
	if cached := m.summaryLayoutCache; cached != nil && cached.key == key && slices.Equal(cached.files, files) && slices.Equal(cached.commits, commits) && slices.Equal(cached.comments, comments) {
		return cached.layout
	}
	content, spans := m.buildSummaryLayout(pr, width)
	layout := newPaneLayout(m.summaryViewportContent(content, width), width)
	layout.content, layout.spans = content, spans
	m.summaryLayoutCache = &summaryLayoutCache{key, slices.Clone(files), slices.Clone(commits), slices.Clone(comments), layout}
	return layout
}

func (m *Model) summaryLayout(pr model.PullRequest, width int) (string, []rowSpan) {
	layout := m.summaryPane(pr, width)
	return layout.content, layout.spans
}

type diffLayoutKey struct {
	content          string
	width            int
	wrap, sideBySide bool
}

type diffLayoutCache struct {
	key      diffLayoutKey
	layout   *paneLayout
	maxWidth int
}

func (m *Model) diffPane(width int) *paneLayout {
	key := diffLayoutKey{m.currentDiffContent(), width, m.cfg.Wrap.Diff, m.sideBySide}
	if cached := m.diffLayoutCache; cached != nil && cached.key == key {
		return cached.layout
	}
	content, sources := m.buildDiffLayout(key.content, width)
	layout := newPaneLayout(content, width)
	layout.sources, layout.lines = sources, strings.Split(content, "\n")
	m.diffLayoutCache = &diffLayoutCache{key, layout, longestLineWidth(key.content)}
	return layout
}

func (m *Model) diffLayout(width int) (string, []int) {
	layout := m.diffPane(width)
	return layout.content, layout.sources
}

type fileLayoutCache struct {
	content, raw string
	theme        layoutTheme
	rendered     string
}

type sideKeyCache struct {
	raw   string
	width int
	wrap  bool
	key   string
}

func (m *Model) sideLayoutKey() string {
	raw := m.sideLayoutRaw()
	if raw == "" {
		return ""
	}
	if cached := m.sideKeyCache; cached != nil && cached.raw == raw && cached.width == m.diffVP.Width() && cached.wrap == m.cfg.Wrap.Diff {
		return cached.key
	}
	key := fmt.Sprintf("%x:%d:%t", sha256.Sum256([]byte(raw)), m.diffVP.Width(), m.cfg.Wrap.Diff)
	m.sideKeyCache = &sideKeyCache{raw, m.diffVP.Width(), m.cfg.Wrap.Diff, key}
	return key
}
