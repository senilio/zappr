package tui

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"
)

func webURL(raw string) *url.URL {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil
	}
	return u
}

func appendURLPath(base, segment string) string {
	u := webURL(base)
	if u == nil {
		return ""
	}
	escaped := strings.TrimRight(u.EscapedPath(), "/") + "/" + url.PathEscape(segment)
	path, err := url.PathUnescape(escaped)
	if err != nil {
		return ""
	}
	u.Path, u.RawPath = path, escaped
	u.Fragment, u.RawFragment, u.RawQuery, u.ForceQuery = "", "", "", false
	return u.String()
}

func fileDiffURL(base, path string) string {
	u := webURL(base)
	if u == nil || path == "" {
		return ""
	}
	u.Fragment = fmt.Sprintf("diff-%x", sha256.Sum256([]byte(path)))
	u.RawFragment = ""
	return u.String()
}

func (m *Model) contextualCopyURL() string {
	pr, ok := m.current()
	if !ok {
		return ""
	}
	base := prURL(pr)
	if webURL(base) == nil {
		return ""
	}
	var target string
	switch m.focus {
	case FocusSummary:
		switch m.summaryMode {
		case SummaryModeFiles:
			if m.fileIndex >= 0 && m.fileIndex < len(pr.Files) {
				target = fileDiffURL(appendURLPath(base, "files"), pr.Files[m.fileIndex].Path)
			}
		case SummaryModeCommits:
			commits := newestCommits(pr.Commits)
			if m.commitIndex >= 0 && m.commitIndex < len(commits) && commits[m.commitIndex].OID != "" {
				target = commitPageURL(base, commits[m.commitIndex].OID)
			}
		case SummaryModeComments:
			target = m.activityCopyURL()
		}
	case FocusDiff:
		switch {
		case m.descriptionShown(), m.summaryMode == SummaryModePlan:
			return base
		case m.summaryMode == SummaryModeComments:
			target = m.activityCopyURL()
		default:
			if path, oid, ok := m.visibleDiffTarget(); ok {
				fileBase := appendURLPath(base, "files")
				if oid != "" {
					fileBase = commitPageURL(base, oid)
				}
				target = fileDiffURL(fileBase, path)
			}
		}
	}
	if target != "" {
		return target
	}
	return base
}

func commitPageURL(base, oid string) string {
	return appendURLPath(appendURLPath(base, "commits"), oid)
}

func (m Model) activityCopyURL() string {
	if entry, ok := m.selectedActivity(); ok {
		if u := webURL(entry.URL); u != nil {
			return u.String()
		}
	}
	return ""
}

func (m *Model) visibleDiffTarget() (path, commitOID string, ok bool) {
	if !m.diffEnabled || m.readerContentActive() || m.diffPlaceholder() != "" {
		return "", "", false
	}
	if m.diffMode == DiffModeFile {
		pr, ok := m.current()
		if ok && m.fileIndex >= 0 && m.fileIndex < len(pr.Files) {
			path = pr.Files[m.fileIndex].Path
		}
		return path, "", path != ""
	}
	starts := m.diffFileStarts
	if m.summaryMode == SummaryModeCommits {
		starts, commitOID = m.commitFileStarts, m.commitDiffKey
	}
	if m.activeSideDiff() {
		starts = m.sideFileStarts
	} else if m.summaryMode != SummaryModeCommits {
		if file, ok := m.displayedPRDiffFile(); ok {
			return file.Path, "", file.Path != ""
		}
	}
	if len(starts) == 0 {
		return "", "", false
	}
	width := m.diffContentWidth(m.paneInnerWidth(m.diffPaneOuterWidth(), FocusDiff))
	layout := m.diffPane(width)
	row := m.diffVP.YOffset()
	if row < 0 || row >= len(layout.renderedRows) {
		return "", "", false
	}
	renderedRow := layout.renderedRows[row]
	for i := len(starts) - 1; i >= 0; i-- {
		if renderedRow >= starts[i].row {
			return starts[i].path, commitOID, starts[i].path != ""
		}
	}
	return "", "", false
}
