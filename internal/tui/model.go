package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/config"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

type Mode int

const (
	ModeInbox Mode = iota
	ModeDetail
	ModeDiff
	ModeConfirmApprove
	ModeComment
	ModeHelp
	ModeSettings
	ModeCommand
	ModeConfirmCommand
	ModeColorscheme
	ModeConfirmAtlantis
)

type Model struct {
	app               *app.App
	repo              string
	allPRs            []model.PullRequest
	prs               []model.PullRequest
	selected          int
	sortMode          app.SortMode
	hideDrafts        bool
	settingsCursor    int
	colorschemeIndex  int // selected index in colorscheme picker
	focus             Focus
	mode              Mode
	error             string
	message           string
	diff              string
	rawDiff           string
	diffFiles         []diffFile // raw per-file bodies (for file switching + V pager)
	renderedFiles     []diffFile // rendered (delta/fallback) per-file bodies
	diffFileStarts    []diffFileStart
	diffMode          DiffMode
	fileContent       string
	fileContentKey    string
	fileIndex         int
	commitIndex       int
	commentIndex      int
	sideBySide        bool
	comment           string
	diffXOffset       int
	summaryXOffset    int
	lastInboxRefresh  time.Time
	inboxRefreshing   bool
	inboxRefreshErr   error
	pollEvery         time.Duration
	pollGeneration    uint64
	inboxRequestID    uint64
	prVersions        map[app.PRKey]prVersion
	width             int
	height            int
	theme             uitheme.Theme
	cfg               config.Config
	settingsSaver     *settingsSaver
	settingsPending   int
	quitPending       bool
	inboxManualScroll bool
	actions           actionRegistry
	prevMode          Mode
	inboxVP           viewport.Model
	summaryVP         viewport.Model
	diffVP            viewport.Model
	helpVP            viewport.Model

	inboxLayoutCache   *inboxLayoutCache
	summaryLayoutCache *summaryLayoutCache
	diffLayoutCache    *diffLayoutCache
	fileLayoutCache    *fileLayoutCache
	sideKeyCache       *sideKeyCache
	inboxVPLayout      *paneLayout
	summaryVPLayout    *paneLayout
	diffVPLayout       *paneLayout

	statusActive       bool
	statusPending      bool
	statusText         string
	statusErr          bool
	statusToken        int
	loadToken          uint64
	diffRequest        uint64
	fileRequest        uint64
	commitDiffRequest  uint64
	diffLoadKey        string
	diffLoadErr        string
	fileLoadErr        string
	fileLoadErrKey     string
	detailsLoadKey     string
	commitDiffKey      string
	commitDiffErr      string
	renderedCommitDiff string
	rawCommitDiff      string
	commitFileStarts   []diffFileStart
	sideDiffContent    string
	sideFileStarts     []diffFileStart
	sideDiffKey        string
	sideDiffRequested  string
	sideDiffFailed     string
	diffEnabled        bool
	// descriptionActive shows the PR description in the large pane. The
	// toggle-description action sets it; navigating to another PR or Summary
	// item clears it.
	descriptionActive bool
	// parkedDiffOffset holds the large-pane scroll position from before the
	// description was opened.
	parkedDiffOffset  paneOffset
	summaryMode       SummaryMode
	planState         planReaderState
	readerIntentToken uint64
	commentState      commentViewState

	commandInput       string // typed input after ":"
	helpInput          string
	pendingAction      tuiAction         // action awaiting confirmation
	pendingPR          model.PullRequest // auto-merge confirmation target and original state
	modalTarget        prIntent
	invalidatedPRModal Mode
	commandCursor      int // selected index in command palette list

	prActions           map[app.PRKey]*prActionQueue
	prActionToken       uint64
	prConfirmationToken uint64
	prConfirmationPage  int
	recentPlans         map[recentPlanKey]time.Time
}

type SummaryMode int

type DiffMode int

const (
	DiffModeHunks DiffMode = iota
	DiffModeFile
)

func (m DiffMode) String() string {
	switch m {
	case DiffModeFile:
		return "file"
	default:
		return "diff"
	}
}

const (
	SummaryModeFiles SummaryMode = iota
	SummaryModeCommits
	SummaryModeComments
	SummaryModePlan
)

func (m SummaryMode) String() string {
	switch m {
	case SummaryModeCommits:
		return "commits"
	case SummaryModeComments:
		return "activity"
	case SummaryModePlan:
		return "plan"
	default:
		return "files"
	}
}

type Focus int

const (
	FocusInbox Focus = iota
	FocusSummary
	FocusDiff
)

func New(zappr *app.App, repo string, pollEvery time.Duration, themeName string, cfg config.Config) (Model, error) {
	actions, err := newActionRegistry(cfg.Actions)
	if err != nil {
		return Model{}, fmt.Errorf("configure TUI actions: %w", err)
	}
	if pollEvery == 0 {
		pollEvery = 30 * time.Second
	}
	vp := viewport.New()
	ivp := viewport.New()
	dvp := viewport.New()
	hvp := viewport.New()
	m := Model{app: zappr, repo: repo, pollEvery: pollEvery, sortMode: app.ParseSortMode(cfg.Sort), hideDrafts: cfg.HideDrafts, theme: uitheme.DefaultWithName(themeName), cfg: cfg, actions: actions, settingsSaver: newSettingsSaver(cfg), inboxVP: ivp, summaryVP: vp, diffVP: dvp, helpVP: hvp, summaryMode: SummaryModeFiles, diffEnabled: true, recentPlans: map[recentPlanKey]time.Time{}}
	if zappr != nil {
		m.inboxRequestID = zappr.BeginInboxRefresh(repo)
		m.inboxRefreshing = true
		if zappr.Cache != nil {
			m.allPRs = zappr.InboxRepo(repo, m.sortMode)
			m.applyPRSettings()
		}
	}
	return m, nil
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.inboxRefreshCmd(), tickCmd(m.pollEvery, m.pollGeneration), uiTickCmd(), tea.RequestBackgroundColor, configWatchTickCmd())
}

type refreshMsg struct {
	id      uint64
	updates []app.PRUpdate
	err     error
}

type refreshPRMsg struct {
	update      app.PRUpdate
	err         error
	requestKey  app.PRKey
	requestHead string
}

type diffMsg struct {
	owner         int
	request       uint64
	repo          string
	number        int
	raw           string
	rendered      string
	files         []diffFile
	renderedFiles []diffFile
	fileStarts    []diffFileStart
	key           string
	err           error
}

type actionMsg struct {
	owner   int
	message string
	err     error
	// refreshPR is refetched after a successful review so the reviewers
	// row reflects the new review.
	refreshPR *model.PullRequest
}

type closePRMsg struct {
	owner  int
	key    app.PRKey
	update app.PRUpdate
	err    error
}

type pagerMsg struct {
	message string
	err     error
}

type clipboardMsg struct{ err error }

type commitDiffMsg struct {
	request    uint64
	sha        string
	raw        string
	rendered   string
	fileStarts []diffFileStart
	err        error
}

type fileContentMsg struct {
	owner   int
	request uint64
	key     string
	content string
	err     error
}

type tickMsg struct{ generation uint64 }

type uiTickMsg struct{}

func uiTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return uiTickMsg{}
	})
}

type statusDismissMsg struct {
	token int
}

func dismissStatusCmd(token int, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return statusDismissMsg{token: token}
	})
}

// startStatus gives the command displaying this text exclusive ownership of
// its completion, even if another command fetches the same PR or file.
func (m *Model) startStatus(text string) int {
	m.statusActive = true
	m.statusPending = true
	m.statusErr = false
	m.statusText = text
	m.statusToken++
	return m.statusToken
}

func (m Model) ownsStatus(owner int) bool {
	return owner != 0 && m.statusPending && m.statusToken == owner
}

func (m *Model) clearStatus() {
	m.statusActive, m.statusPending, m.statusErr, m.statusText = false, false, false, ""
}

func (m *Model) completeStatus(owner int, text string, isErr bool) tea.Cmd {
	if !m.ownsStatus(owner) {
		return nil
	}
	if text == "" {
		m.clearStatus()
		return nil
	}
	return m.finishStatus(text, isErr)
}

// finishStatus updates the popup to a final message and schedules auto-dismiss.
func (m *Model) finishStatus(text string, isErr bool) tea.Cmd {
	m.statusActive = true
	m.statusPending = false
	m.statusErr = isErr
	m.statusText = text
	m.statusToken++
	return dismissStatusCmd(m.statusToken, 1200*time.Millisecond)
}

func (m *Model) startInboxRefresh() tea.Cmd {
	if m.app == nil {
		return nil
	}
	m.inboxRequestID = m.app.BeginInboxRefresh(m.repo)
	m.inboxRefreshing = true
	return m.inboxRefreshCmd()
}

func (m Model) inboxRefreshCmd() tea.Cmd {
	a, repo, id := m.app, m.repo, m.inboxRequestID
	if a == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		updates, err := a.RefreshInbox(ctx, repo, id)
		return refreshMsg{id: id, updates: updates, err: err}
	}
}

func refreshPRCmd(a *app.App, pr model.PullRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		key := keyForPR(pr)
		update, err := a.RefreshPRUpdate(ctx, key)
		return refreshPRMsg{update: update, err: err, requestKey: key, requestHead: pr.HeadRefOID}
	}
}

func tickCmd(interval time.Duration, generation uint64) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return tickMsg{generation: generation}
	})
}

func diffCmd(app *app.App, pr model.PullRequest, width, owner int, request uint64) tea.Cmd {
	return func() tea.Msg {
		key := prRefreshKey(pr)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		diff, err := app.GitHub.GetDiff(ctx, pr.Repo, pr.Number)
		if err != nil {
			return diffMsg{owner: owner, request: request, repo: pr.Repo, number: pr.Number, key: key, err: err}
		}
		// Split on the raw diff so that the "diff --git" markers are intact.
		// Rendering (delta) rewrites those headers so splitting rendered output
		// would collapse all files into a single chunk.
		files := splitDiffFiles(diff)
		renderedFiles := renderDiffFiles(ctx, files, width, false)
		rendered, starts := joinRenderedDiffFiles(renderedFiles)
		if rendered == "" {
			rendered, err = renderDiff(ctx, diff, width, false)
		}
		return diffMsg{owner: owner, request: request, repo: pr.Repo, number: pr.Number, key: key, raw: diff, rendered: rendered, files: files, renderedFiles: renderedFiles, fileStarts: starts, err: err}
	}
}

func commitDiffCmd(ghProvider interface {
	GetCommitDiff(ctx context.Context, repo string, sha string) (string, error)
}, repo string, sha string, width int, request uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		raw, err := ghProvider.GetCommitDiff(ctx, repo, sha)
		if err != nil {
			return commitDiffMsg{request: request, sha: sha, err: err}
		}
		rendered, starts, err := renderDiffSections(ctx, raw, width, false, false)
		return commitDiffMsg{request: request, sha: sha, raw: raw, rendered: rendered, fileStarts: starts, err: err}
	}
}

func fileContentCmd(ghProvider interface {
	GetFileContent(ctx context.Context, repo string, path string, ref string) (string, error)
}, repo, path, ref, key string, owner int, request uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		content, err := ghProvider.GetFileContent(ctx, repo, path, ref)
		return fileContentMsg{owner: owner, request: request, key: key, content: content, err: err}
	}
}

func prWriteCmd(a *app.App, pr model.PullRequest, closedRevision uint64, result actionMsg, write func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		msg := result
		msg.err = a.CheckPRIntent(keyForPR(pr), closedRevision)
		if msg.err == nil {
			msg.err = write(ctx)
		}
		return msg
	}
}

func approveCmd(a *app.App, pr model.PullRequest, owner int, closedRevision uint64) tea.Cmd {
	msg := actionMsg{owner: owner, message: fmt.Sprintf("approved %s#%d", pr.Repo, pr.Number), refreshPR: &pr}
	return prWriteCmd(a, pr, closedRevision, msg, func(ctx context.Context) error {
		return a.Approve(ctx, pr.Repo, pr.Number, "")
	})
}

func commentCmd(a *app.App, pr model.PullRequest, body string, owner int, closedRevision uint64) tea.Cmd {
	msg := actionMsg{owner: owner, message: fmt.Sprintf("commented on %s#%d", pr.Repo, pr.Number)}
	return prWriteCmd(a, pr, closedRevision, msg, func(ctx context.Context) error {
		return a.Comment(ctx, pr.Repo, pr.Number, body)
	})
}

func toggleDraftCmd(a *app.App, pr model.PullRequest, owner int, closedRevision uint64) tea.Cmd {
	newDraft := !pr.Draft
	action := "marked ready"
	if newDraft {
		action = "marked draft"
	}
	msg := actionMsg{owner: owner, message: fmt.Sprintf("%s %s#%d", action, pr.Repo, pr.Number)}
	return prWriteCmd(a, pr, closedRevision, msg, func(ctx context.Context) error {
		return a.ToggleDraft(ctx, pr.Repo, pr.Number, newDraft)
	})
}

func toggleAutoMergeCmd(a *app.App, pr model.PullRequest, owner int, closedRevision uint64) tea.Cmd {
	enabled := !pr.AutoMerge
	action := "auto-merge disabled for"
	if enabled {
		action = "auto-merge requested for"
	}
	msg := actionMsg{owner: owner, message: fmt.Sprintf("%s %s#%d", action, pr.Repo, pr.Number)}
	return prWriteCmd(a, pr, closedRevision, msg, func(ctx context.Context) error {
		return a.ToggleAutoMerge(ctx, pr.Repo, pr.Number, enabled)
	})
}

func closePRCmd(a *app.App, pr model.PullRequest, comment string, owner int, closedRevision uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		key := keyForPR(pr)
		if err := a.CheckPRIntent(key, closedRevision); err != nil {
			return closePRMsg{owner: owner, key: key, err: err}
		}
		update, err := a.ClosePRUpdate(ctx, key, comment)
		return closePRMsg{owner: owner, key: key, update: update, err: err}
	}
}

func updateBranchCmd(a *app.App, pr model.PullRequest, owner int, closedRevision uint64) tea.Cmd {
	msg := actionMsg{owner: owner, message: fmt.Sprintf("updated branch for %s#%d", pr.Repo, pr.Number)}
	return prWriteCmd(a, pr, closedRevision, msg, func(ctx context.Context) error {
		return a.UpdateBranch(ctx, pr.Repo, pr.Number)
	})
}

func requestChangesCmd(a *app.App, pr model.PullRequest, body string, owner int, closedRevision uint64) tea.Cmd {
	msg := actionMsg{owner: owner, message: fmt.Sprintf("requested changes on %s#%d", pr.Repo, pr.Number), refreshPR: &pr}
	return prWriteCmd(a, pr, closedRevision, msg, func(ctx context.Context) error {
		return a.RequestChanges(ctx, pr.Repo, pr.Number, body)
	})
}

func mergePRCmd(a *app.App, pr model.PullRequest, owner int, closedRevision uint64) tea.Cmd {
	msg := actionMsg{owner: owner, message: fmt.Sprintf("merge requested for %s#%d", pr.Repo, pr.Number)}
	return prWriteCmd(a, pr, closedRevision, msg, func(ctx context.Context) error {
		return a.MergePR(ctx, pr.Repo, pr.Number)
	})
}

func (m Model) current() (model.PullRequest, bool) {
	if len(m.prs) == 0 || m.selected < 0 || m.selected >= len(m.prs) {
		return model.PullRequest{}, false
	}
	return m.prs[m.selected], true
}

func trimDiff(diff string) string {
	const max = 12000
	if len(diff) <= max {
		return diff
	}
	return diff[:max] + "\n\n... diff truncated in TUI ..."
}

type diffFile struct {
	Path string
	Body string
}

func splitDiffFiles(diff string) []diffFile {
	var files []diffFile
	var current *diffFile
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				current.Path = patchFilePath(current.Body)
				files = append(files, *current)
			}
			current = &diffFile{}
		}
		if current != nil {
			current.Body += line + "\n"
		}
	}
	if current != nil {
		current.Path = patchFilePath(current.Body)
		files = append(files, *current)
	}
	return files
}

func patchFilePath(body string) string {
	header, rest, _ := strings.Cut(body, "\n")
	var before, after string
	for line := range strings.SplitSeq(rest, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"), strings.HasPrefix(line, "GIT binary patch"):
			return patchHeaderPath(header, before, after)
		case strings.HasPrefix(line, "rename to "):
			return patchPath(strings.TrimPrefix(line, "rename to "))
		case strings.HasPrefix(line, "copy to "):
			return patchPath(strings.TrimPrefix(line, "copy to "))
		case strings.HasPrefix(line, "--- "):
			before, _, _ = strings.Cut(strings.TrimPrefix(line, "--- "), "\t")
		case strings.HasPrefix(line, "+++ "):
			after, _, _ = strings.Cut(strings.TrimPrefix(line, "+++ "), "\t")
		}
	}
	return patchHeaderPath(header, before, after)
}

func patchHeaderPath(header, before, after string) string {
	if after != "" && after != "/dev/null" {
		return strings.TrimPrefix(patchPath(after), "b/")
	}
	if before != "" && before != "/dev/null" {
		return strings.TrimPrefix(patchPath(before), "a/")
	}
	// Metadata-only changes may have no ---/+++ lines. Match the duplicated
	// path rather than splitting it on spaces (which Git can leave unquoted).
	pair := strings.TrimPrefix(header, "diff --git ")
	for i := range len(pair) {
		if pair[i] != ' ' {
			continue
		}
		a, aOK := strings.CutPrefix(patchPath(pair[:i]), "a/")
		b, bOK := strings.CutPrefix(patchPath(pair[i+1:]), "b/")
		if aOK && bOK && a == b {
			return b
		}
	}
	return ""
}

func patchPath(path string) string {
	if strings.HasPrefix(path, `"`) {
		path, _ = strconv.Unquote(path)
	}
	return path
}

type diffFileStart struct {
	row  int
	path string
}

func joinRenderedDiffFiles(files []diffFile) (string, []diffFileStart) {
	var b strings.Builder
	var starts []diffFileStart
	row := 0
	for _, f := range files {
		body := strings.TrimRight(f.Body, "\n")
		if body == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
			row++
		}
		starts = append(starts, diffFileStart{row: row, path: f.Path})
		b.WriteString(body)
		row += strings.Count(body, "\n")
	}
	return b.String(), starts
}

func renderDiffSections(ctx context.Context, raw string, width int, side, wrap bool) (string, []diffFileStart, error) {
	files := splitDiffFiles(raw)
	for i := range files {
		rendered, err := renderDiff(ctx, files[i].Body, width, side, wrap)
		if err != nil {
			return "", nil, err
		}
		files[i].Body = rendered
	}
	rendered, starts := joinRenderedDiffFiles(files)
	if rendered != "" {
		return rendered, starts, nil
	}
	rendered, err := renderDiff(ctx, raw, width, side, wrap)
	return rendered, nil, err
}

func renderDiff(ctx context.Context, diff string, width int, sideBySide bool, wrap ...bool) (string, error) {
	if _, err := exec.LookPath("delta"); err == nil {
		args := []string{"--paging=never", "--line-numbers", "--file-decoration-style=none", "--hunk-header-decoration-style=none", "--hunk-header-style=omit"}
		if len(wrap) > 0 && wrap[0] {
			args = append(args, "--wrap-max-lines=unlimited")
		} else {
			args = append(args, "--wrap-max-lines=0")
		}
		if width > 0 {
			args = append(args, fmt.Sprintf("--width=%d", width))
		}
		if sideBySide {
			args = append(args, "--side-by-side")
		}
		cmd := exec.CommandContext(ctx, "delta", args...)
		cmd.Stdin = strings.NewReader(diff)
		var out bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			// delta emits non-SGR CSI sequences (e.g. ESC[0K) that measure as
			// zero width but are executed by the terminal, corrupting layout.
			// Strip them at ingest while preserving color codes.
			// Also trim leading/trailing blank lines that delta adds around
			// each file header to avoid empty rows at the pane border.
			cleaned := stripNonSGR(out.String())
			cleaned = strings.TrimRight(cleaned, "\n")
			cleaned = strings.TrimLeft(cleaned, "\n")
			return cleaned, nil
		}
	}
	return fallbackDiff(diff), nil
}

// renderDiffFiles renders each raw diffFile individually so that per-file
// bodies can be displayed when the user navigates between files. Rendering the
// full diff as a single string and then splitting the result is unreliable
// because tools like delta rewrite the "diff --git" header lines, making the
// split markers invisible.
func renderDiffFiles(ctx context.Context, files []diffFile, width int, sideBySide bool) []diffFile {
	out := make([]diffFile, len(files))
	for i, f := range files {
		rendered, err := renderDiff(ctx, f.Body, width, sideBySide)
		if err != nil {
			rendered = fallbackDiff(f.Body)
		}
		out[i] = diffFile{Path: f.Path, Body: rendered}
	}
	return out
}

func fallbackDiff(diff string) string {
	var b strings.Builder
	for _, line := range strings.Split(diff, "\n") {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func longestLineWidth(content string) int {
	width := 0
	for _, line := range strings.Split(content, "\n") {
		if w := ansi.StringWidth(line); w > width {
			width = w
		}
	}
	return width
}
