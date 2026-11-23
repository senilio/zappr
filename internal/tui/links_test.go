package tui

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/model"
)

func TestContextualCopyURL(t *testing.T) {
	const prURL = "https://git.example.com/o/r/pull/7"
	const fileURL = prURL + "/files#diff-d29189d0d5a5081839d0f4848ce28f62465a7f14bcf336c6f152ee81fae066a6"
	m := modelWithPRs([]model.PullRequest{{
		URL: prURL, Files: []model.ChangedFile{{Path: "dir/a b.go"}},
		Commits:  []model.PRCommit{{OID: "older"}, {OID: "full-oid", Date: time.Now()}},
		Comments: []model.PRComment{{URL: prURL + "#issuecomment-1"}},
	}}, 0)
	m.width, m.diffEnabled = 130, true
	check := func(want string) {
		t.Helper()
		if got := m.contextualCopyURL(); got != want {
			t.Fatalf("focus %v, tab %v: copied %q, want %q", m.focus, m.summaryMode, got, want)
		}
	}
	m.focus = FocusInbox
	check(prURL)
	m.focus, m.summaryMode = FocusSummary, SummaryModeFiles
	check(fileURL)
	m.focus, m.descriptionActive = FocusDiff, true
	check(prURL)
	m.focus, m.summaryMode = FocusSummary, SummaryModeCommits
	check(prURL + "/commits/full-oid")
	m.descriptionActive, m.summaryMode = false, SummaryModeComments
	check(prURL + "#issuecomment-1")
	m.focus = FocusDiff
	check(prURL + "#issuecomment-1")
	m.prs[0].Comments[0].URL = "javascript:bad"
	check(prURL)
	m.prs[0].Comments = nil
	m.prs[0].Reviews = []model.PRReview{{ID: "review", URL: prURL + "#pullrequestreview-1"}}
	check(prURL + "#pullrequestreview-1")
	m.prs[0].ReviewDismissals = []model.PRReviewDismissal{{ID: "event", ReviewID: "review", URL: prURL + "#event-1", CreatedAt: time.Now()}}
	check(prURL + "#event-1")
	m.summaryMode = SummaryModePlan
	check(prURL)
	m.summaryMode = SummaryModeFiles
	check(prURL)
	m.diff = "loaded"
	m.renderedFiles = []diffFile{{Path: "different.txt", Body: "visible"}}
	check(fileDiffURL(prURL+"/files", "different.txt"))
	m.diffMode = DiffModeFile
	m.fileContentKey, m.fileContent = m.selectedFileContentKey(), "contents"
	check(fileURL)
	m.prs = nil
	check("")
}

func TestVisibleDiffTargetUsesViewport(t *testing.T) {
	t.Setenv("PATH", "") // Use deterministic fallback diff rendering.
	const patch = `diff --git a/dir/a b.go b/dir/a b.go
--- a/dir/a b.go
+++ b/dir/a b.go
+old
diff --git "a/old\040name" "b/new\040name"
similarity index 80%
rename from "old\040name"
rename to "new\040name"
+very long second line for viewport wrapping that will not fit entirely in the diff pane even at a width of ninety columns
diff --git a/gone file b/gone file
deleted file mode 100644
--- a/gone file
+++ /dev/null
-gone
`
	files := splitDiffFiles(patch)
	if len(files) != 3 || files[0].Path != "dir/a b.go" || files[1].Path != "new name" || files[2].Path != "gone file" {
		t.Fatalf("diff paths: %+v", files)
	}
	rendered, starts, err := renderDiffSections(context.Background(), patch, 16, false, false)
	if err != nil || len(starts) != 3 {
		t.Fatalf("renderDiffSections: %v, %+v", err, starts)
	}
	for _, side := range []bool{false, true} {
		m := modelWithPRs([]model.PullRequest{{URL: "https://github.com/o/r/pull/7", Files: []model.ChangedFile{{Path: "unrelated"}}}}, 0)
		m.width, m.height, m.focus, m.summaryMode = 90, 16, FocusDiff, SummaryModeCommits
		m.cfg.Wrap.Diff, m.diffEnabled, m.sideBySide = true, true, side
		m.diff, m.renderedCommitDiff, m.rawCommitDiff, m.commitDiffKey = rendered, rendered, patch, "full-oid"
		m.commitFileStarts = starts
		m.syncDiffVP()
		if side {
			m.sideDiffKey, m.sideDiffContent, m.sideFileStarts = m.sideLayoutKey(), rendered, starts
			m.syncDiffVP()
		}
		layout := m.diffPane(m.diffVP.Width())
		found := false
		for row := 1; row < len(layout.renderedRows); row++ {
			r := layout.renderedRows[row]
			if r >= starts[1].row && r < starts[2].row && layout.renderedRows[row-1] == r {
				m.diffVP.SetHeight(1)
				m.diffVP.SetYOffset(row)
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("side=%t: no wrapped row in second file", side)
		}
		want := fileDiffURL("https://github.com/o/r/pull/7/commits/full-oid", "new name")
		if got := m.contextualCopyURL(); got != want {
			t.Fatalf("side=%t offset=%d URL=%q, want %q", side, m.diffVP.YOffset(), got, want)
		}
	}
}

func TestCopyActionRoutesWithoutClipboard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX fake clipboard command")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("COPIED_URL", filepath.Join(dir, "clipboard"))
	for _, name := range []string{"pbcopy", "xclip"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n/bin/cat > \"$COPIED_URL\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	m := nativeCommentModel(t)
	m.focus = FocusSummary
	m.prs[0].Comments[0].URL = "https://github.com/o/r/pull/7#issuecomment-101"
	for _, name := range []string{"Summary", "compact Activity", "compact Diff"} {
		t.Run(name, func(t *testing.T) {
			view := m
			want := "#issuecomment-101"
			if name != "Summary" {
				view.width = 60
				view.diffEnabled = false
				if name == "compact Diff" {
					view.summaryMode = SummaryModeFiles
					view.prs = append([]model.PullRequest(nil), m.prs...)
					view.prs[0].Files = []model.ChangedFile{{Path: "visible.go"}}
					view.diff = "visible diff"
					view.renderedFiles = []diffFile{{Path: "visible.go", Body: "visible diff"}}
					want = "#diff-" + fmt.Sprintf("%x", sha256.Sum256([]byte("visible.go")))
				}
				view = pressKey(view, "v")
				if !view.isFullscreenDiff() {
					t.Fatal("did not enter compact full-screen pane")
				}
			}
			_, cmd := view.Update(tea.KeyPressMsg{Text: "C"})
			if cmd == nil {
				t.Fatal("C did not schedule clipboard copy")
			}
			msg := cmd()
			copied, err := os.ReadFile(os.Getenv("COPIED_URL"))
			if err != nil || !strings.HasSuffix(string(copied), want) {
				t.Fatalf("clipboard result %q, %v, msg=%#v", copied, err, msg)
			}
		})
	}
}
