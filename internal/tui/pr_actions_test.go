package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/senilio/zappr/internal/config"
	"github.com/senilio/zappr/internal/model"
)

func keyUpdateResult(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func runConfirmAction(m Model) (Model, tea.Cmd) {
	return keyUpdate(openCommandAndType(m, "confirm-action"), "enter")
}

func queuedPlanModel(t *testing.T, confirm bool) (Model, *commentRecordingGitHub, model.PullRequest, model.PullRequest) {
	t.Helper()
	cfg := atlantisConfig()
	cfg.Actions["atlantis-plan"] = config.ActionConfig{
		Operation: "comment", Keys: []string{"P"}, Commands: []string{"ap"}, Comment: "atlantis plan", Confirm: &confirm,
	}
	a := model.PullRequest{
		Repo: "org/repo", Number: 9765, State: "OPEN", HeadRefOID: "old",
		Title:         "An ordinary PR with a title long enough to wrap in a narrow pane",
		DetailsLoaded: true, ReviewersLoaded: true, ReviewCommentsLoaded: true,
		ConversationCommentsLoaded: true, ReviewActivityLoaded: true,
	}
	b := a
	b.Number, b.Title = 9764, "Second PR"
	m, provider := atlantisGuardModel(t, cfg, a)
	m.prs, m.allPRs = []model.PullRequest{a, b}, []model.PullRequest{a, b}
	m.diffEnabled = false
	provider.getPR = a
	provider.listedPRs = m.allPRs
	return m, provider, a, b
}

func TestPRActionShowsRefreshingAndQueuedOnTheRow(t *testing.T) {
	m, _, a, _ := queuedPlanModel(t, false)
	m, refresh := keyUpdate(m, "r")
	m, plan := keyUpdate(m, "P")
	row := stripANSI(m.inboxMetaRow(a, true, 70))
	if refresh == nil || plan != nil || m.statusActive || !strings.Contains(row, "refreshing") || !strings.Contains(row, "plan queued") {
		t.Fatalf("refresh=%v, plan started=%v, header=%q, row=%q", refresh != nil, plan != nil, m.statusText, row)
	}
}

func TestPRActionRefreshThenPlanSurvivesNavigation(t *testing.T) {
	m, provider, a, b := queuedPlanModel(t, false)
	m, refreshA := keyUpdate(m, "r")
	m, plan := keyUpdate(m, "P")
	if plan != nil {
		t.Fatal("plan started before the refresh finished")
	}
	m = pressSpecial(m, tea.KeyDown)
	m, refreshB := keyUpdate(m, "r")
	provider.getPR = b
	m = deliver(m, commandResult[prActionResult](t, refreshB))
	m = typeComment(pressKey(m, "c"), "draft on second PR")
	provider.getPR = a
	m, check := keyUpdateResult(m, commandResult[prActionResult](t, refreshA))
	checked := commandResult[prActionResult](t, check)
	m, post := keyUpdateResult(m, checked)
	m, duplicate := keyUpdateResult(m, checked)
	if duplicate != nil {
		t.Fatal("duplicate verification issued another post")
	}
	posted := commandResult[prActionResult](t, post)
	m = deliver(m, posted)
	if _, duplicate := keyUpdateResult(m, posted); duplicate != nil {
		t.Fatal("duplicate post completion advanced the queue")
	}
	if want := []string{"get:9764", "get:9765", "verify:9765:true", "comment:9765:atlantis plan"}; !reflect.DeepEqual(provider.prCalls, want) {
		t.Fatalf("calls = %v; want %v", provider.prCalls, want)
	}
	if current, _ := m.current(); current.Number != b.Number || m.mode != ModeComment || m.comment != "draft on second PR" || m.statusActive {
		t.Fatalf("completion changed view: PR=%d, mode=%v, input=%q, status=%q", current.Number, m.mode, m.comment, m.statusText)
	}
	if _, ok := m.recentPlans[recentPlanKey{repo: a.Repo, number: a.Number, head: a.HeadRefOID}]; !ok {
		t.Fatal("successful post was not recorded at its verified head")
	}
}

func TestPRActionConfirmationIsScoped(t *testing.T) {
	m, provider, a, b := queuedPlanModel(t, false)
	provider.getPR.Checks = []model.Check{{Context: "atlantis/plan", State: "PENDING"}}
	m, verify := keyUpdate(m, "P")
	m, post := keyUpdateResult(m, commandResult[prActionResult](t, verify))
	if post != nil || m.mode != ModeInbox || !strings.Contains(stripANSI(m.inboxMetaRow(a, true, 70)), "confirm plan") {
		t.Fatal("guard took over the view or did not park on its row")
	}
	m = pressSpecial(m, tea.KeyDown)
	m, _ = runConfirmAction(m)
	if m.mode == ModeConfirmAtlantis {
		t.Fatalf("confirmation for #%d opened from #%d", a.Number, b.Number)
	}
	m, refreshB := keyUpdate(m, "r")
	provider.getPR = b
	m = deliver(m, commandResult[prActionResult](t, refreshB))
	m = pressSpecial(m, tea.KeyUp)
	m, _ = runConfirmAction(m)
	assertMode(t, m, ModeConfirmAtlantis)
	for _, want := range []string{"atlantis plan on #9765?", "old", "still running", "send anyway"} {
		if !strings.Contains(atlantisDialogText(m), want) {
			t.Errorf("dialog %q does not contain %q", atlantisDialogText(m), want)
		}
	}
	m, wait := keyUpdate(m, "r") // A modal must not treat inputs as normal shortcuts.
	if wait != nil {
		t.Fatal("dialog routed r to the normal context")
	}
	m, next := keyUpdate(m, "esc")
	if next != nil || m.mode != ModeInbox || len(provider.comments) != 0 {
		t.Fatalf("cancel changed view or posted: mode=%v, comments=%v", m.mode, provider.comments)
	}
}

func TestPRActionConsentRequiresFreshOpenHead(t *testing.T) {
	for _, change := range []string{"none", "head", "closure", "unknown state", "missing head", "new pending reason"} {
		t.Run(change, func(t *testing.T) {
			m, provider, a, _ := queuedPlanModel(t, true)
			provider.evidenceErr = errors.New("rate limit")
			m, check := keyUpdate(m, "P")
			m = deliver(m, commandResult[prActionResult](t, check))
			m, _ = runConfirmAction(m)
			assertMode(t, m, ModeConfirmAtlantis)
			if !strings.Contains(atlantisDialogText(m), "rate limit") {
				t.Fatalf("missing evidence error: %q", atlantisDialogText(m))
			}
			switch change {
			case "head":
				provider.getPR.HeadRefOID = "new"
			case "closure":
				provider.getPR.State = "CLOSED"
			case "unknown state":
				provider.getPR.State = ""
			case "missing head":
				provider.getPR.HeadRefOID = ""
			case "new pending reason":
				provider.getPR.Checks = []model.Check{{Context: "atlantis/plan", State: "PENDING"}}
			}
			m, recheck := keyUpdate(m, "y")
			if len(provider.comments) != 0 {
				t.Fatal("confirmation initiated a post without rechecking")
			}
			m, post := keyUpdateResult(m, commandResult[prActionResult](t, recheck))
			if change == "none" {
				m = deliver(m, commandResult[prActionResult](t, post))
				assertCommentBody(t, provider, "atlantis plan")
				return
			}
			if post != nil || len(provider.comments) != 0 {
				t.Fatalf("%s allowed posting: %v, %v", change, post != nil, provider.comments)
			}
			if change == "closure" {
				if len(m.prs) != 1 || m.prs[0].Number == a.Number {
					t.Fatal("closed target still present")
				}
				return
			}
			m, _ = runConfirmAction(m)
			if change == "head" || change == "new pending reason" {
				assertMode(t, m, ModeConfirmAtlantis)
				want := "still running"
				if change == "head" {
					want = "new"
				}
				if !strings.Contains(atlantisDialogText(m), want) {
					t.Fatalf("fresh conflict not shown: %q", atlantisDialogText(m))
				}
			} else if m.mode == ModeConfirmAtlantis {
				t.Fatalf("unknown target can be overridden: %s", change)
			}
		})
	}
}

func TestPRActionClosureInvalidatesInFlightPrecheck(t *testing.T) {
	for _, removal := range []string{"local close", "complete list", "incomplete list", "filtered draft"} {
		t.Run(removal, func(t *testing.T) {
			m, provider, a, b := queuedPlanModel(t, false)
			if removal == "filtered draft" {
				a.Draft = true
				provider.getPR, m.allPRs[0], m.prs[0] = a, a, a
				m.hideDrafts = false
			}
			m, check := keyUpdate(m, "P")
			success := commandResult[prActionResult](t, check)
			m = pressSpecial(m, tea.KeyDown)
			m, refreshB := keyUpdate(m, "r")
			provider.getPR = b
			pendingB := commandResult[prActionResult](t, refreshB)
			if removal == "local close" {
				m = deliver(m, commandResult[closePRMsg](t, closePRCmd(m.app, a, "", 0, 0)))
			} else {
				provider.listedPRs = []model.PullRequest{b}
				if removal == "incomplete list" {
					provider.listIncomplete = true
				}
				if removal == "filtered draft" {
					// Change local visibility independently of the raw listing.
					m.hideDrafts = true
					m.applyPRSettings()
					if m.prVersions[keyForPR(a)].closedRevision != 0 {
						t.Fatal("draft visibility registered a closure")
					}
				} else {
					var cmd tea.Cmd
					m, cmd = keyUpdate(m, "R")
					m = deliver(m, commandResult[refreshMsg](t, cmd))
				}
			}
			m = deliver(m, pendingB)
			m, post := keyUpdateResult(m, success)
			closed := removal == "local close" || removal == "complete list"
			if closed {
				if post != nil || len(m.prs) != 1 || len(provider.comments) != 0 || m.prs[0].Number != b.Number {
					t.Fatalf("late verified result acted after removal: post=%v rows=%v comments=%v", post != nil, m.prs, provider.comments)
				}
			} else {
				m = deliver(m, commandResult[prActionResult](t, post))
				assertCommentBody(t, provider, "atlantis plan")
				if removal == "filtered draft" && len(m.prs) != 1 {
					t.Fatal("posting brought back a hidden draft")
				}
			}
		})
	}
}

func TestPRActionRepeatedEvidenceErrorStillShowsNewUnlock(t *testing.T) {
	m, provider, pr, _ := queuedPlanModel(t, false)
	m = typeComment(pressKey(m, "c"), "atlantis apply")
	provider.evidenceErr = errors.New("parse gh conversation comments: missing or invalid native provenance")
	m, verify := keyUpdate(m, "enter")
	m = deliver(m, commandResult[prActionResult](t, verify))
	m, _ = runConfirmAction(m)
	assertMode(t, m, ModeConfirmAtlantis)
	provider.getPR.Comments = []model.PRComment{{
		Author: "atlantis[bot]", Body: "All Atlantis locks for this PR have been unlocked and plans discarded", CreatedAt: time.Now(),
	}}
	m, recheck := keyUpdate(m, "y")
	m, post := keyUpdateResult(m, commandResult[prActionResult](t, recheck))
	if post != nil || m.prActionPills(pr)[0].label != "confirm apply" {
		t.Fatal("same evidence error masked the new unlock")
	}
	m, _ = runConfirmAction(m)
	if text := atlantisDialogText(m); !strings.Contains(text, "discarded by an unlock") {
		t.Fatalf("new confirmation didn't disclose the unlock: %q", text)
	}
}

func TestPRActionConfirmationKeepsWarningsVisibleWithLongComment(t *testing.T) {
	m, provider, _, _ := queuedPlanModel(t, false)
	m = deliver(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = pressKey(m, "c")
	const head = "0123456789012345678901234567890123456789"
	provider.getPR.HeadRefOID = head
	body := "atlantis apply\n" + strings.Repeat("additional comment line\n", 30) + "last comment line"
	m = deliver(m, tea.PasteMsg{Content: body})
	m, check := keyUpdate(m, "enter")
	m = deliver(m, commandResult[prActionResult](t, check))
	m, _ = runConfirmAction(m)
	for _, text := range []string{"atlantis apply on #9765?", head, "no Atlantis plan", "pgdown"} {
		if !strings.Contains(stripANSI(m.View().Content), text) {
			t.Errorf("%q not visible in the prompt", text)
		}
	}
	if blocked, cmd := keyUpdate(m, "y"); cmd != nil || blocked.mode != ModeConfirmAtlantis {
		t.Fatal("confirmation bypassed the remaining comment pages")
	}
	m = deliver(m, tea.WindowSizeMsg{Width: 50, Height: 7})
	if blocked, cmd := keyUpdate(m, "enter"); cmd != nil || blocked.mode != ModeConfirmAtlantis {
		t.Fatal("confirmation proceeded with no space to show the warnings")
	}
	m = deliver(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	for range 4 {
		m = pressSpecial(m, tea.KeyPgDown)
	}
	visible := stripANSI(m.View().Content)
	for _, text := range []string{head, "no Atlantis plan", "last comment line", "send anyway"} {
		if !strings.Contains(visible, text) {
			t.Errorf("%q not visible on the last page", text)
		}
	}
	if _, recheck := keyUpdate(m, "y"); recheck == nil {
		t.Fatal("fully visible confirmation did not recheck the target")
	}
}

func TestPRActionPrecheckLoadsSelectedDiffWhenHeadChanges(t *testing.T) {
	m, provider, pr, _ := queuedPlanModel(t, true)
	m.diffEnabled = true
	m.diff, m.rawDiff, m.diffLoadKey = "old diff", "old raw diff", prRefreshKey(pr)
	provider.getPR.HeadRefOID = "new"
	m, check := keyUpdate(m, "P")
	m, next := keyUpdateResult(m, commandResult[prActionResult](t, check))
	if m.diff != "" || m.prActionPills(pr)[0].label != "confirm plan" {
		t.Fatal("head change did not clear the old diff and park the plan")
	}
	if next == nil {
		t.Fatal("guard parked with no request to replace the selected diff")
	}
	diff, ok := next().(diffMsg)
	if !ok || diff.key != prRefreshKey(provider.getPR) {
		t.Fatalf("completion should request only the new diff, got %+v", diff)
	}
}

func TestPRActionCancelAndUnlockKeepConfiguredConfirmation(t *testing.T) {
	for _, name := range []string{"cancel", "unlock"} {
		t.Run(name, func(t *testing.T) {
			pr := model.PullRequest{Repo: "org/repo", Number: 1, State: "OPEN", HeadRefOID: "old"}
			cfg := atlantisConfig()
			body := "ATLANTIS " + name
			cfg.Actions["atlantis-"+name] = config.ActionConfig{Operation: "comment", Keys: []string{"P"}, Comment: body}
			m, provider := atlantisGuardModel(t, cfg, pr)
			provider.getPR = pr
			m, precheck := keyUpdate(m, "P")
			m = deliver(m, commandResult[prActionResult](t, precheck))
			if m.mode != ModeInbox || m.prActionPills(pr)[0].label != "confirm "+name {
				t.Fatalf("%s did not park for ordinary confirmation", name)
			}
			m, _ = runConfirmAction(m)
			if text := atlantisDialogText(m); !strings.Contains(text, " confirm ") || strings.Contains(text, "send anyway") {
				t.Fatalf("ordinary confirmation = %q", text)
			}
			m, recheck := keyUpdate(m, "y")
			runAtlantis(t, m, recheck)
			if want := []string{"verify:1:false", "verify:1:false", "comment:1:" + body}; !reflect.DeepEqual(provider.prCalls, want) {
				t.Fatalf("calls = %v, want %v", provider.prCalls, want)
			}
		})
	}
}

func TestPRActionPostFailureIsLocal(t *testing.T) {
	m, provider, a, _ := queuedPlanModel(t, false)
	provider.commentErr = errors.New("GitHub refused\x1b[2J\ncomment")
	m, check := keyUpdate(m, "P")
	m, post := keyUpdateResult(m, commandResult[prActionResult](t, check))
	result := commandResult[prActionResult](t, post)
	m, followup := keyUpdateResult(m, result)
	row := stripANSI(m.inboxMetaRow(a, true, 100))
	if followup != nil || m.statusActive || len(provider.comments) != 1 || len(m.recentPlans) != 0 ||
		!strings.Contains(row, "plan send failed: GitHub refused comment") || strings.ContainsAny(row, "\x1b\n") {
		t.Fatalf("post error was not local or was retried: followup=%v header=%q row=%q posts=%v", followup != nil, m.statusText, row, provider.comments)
	}
}

func TestPRActionPillsTrackQueueWithoutChangingRowSpans(t *testing.T) {
	m, provider, a, _ := queuedPlanModel(t, false)
	a.Checks = []model.Check{{Context: "atlantis/plan", State: "SUCCESS"}}
	provider.getPR = a
	m.allPRs[0], m.prs[0] = a, a
	m.cfg.Wrap.Inbox = true
	const width = 42
	_, initialSpans := m.inboxContent(width)
	m, refresh := keyUpdate(m, "r")
	m, _ = keyUpdate(m, "P")
	assertLayout := func(text string) {
		t.Helper()
		content, spans := m.inboxContent(width)
		if !reflect.DeepEqual(spans, initialSpans) || !strings.Contains(stripANSI(content), text) {
			t.Fatalf("layout = %q, spans = %v, want %q and %v", content, spans, text, initialSpans)
		}
		for _, line := range strings.Split(content, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("row exceeds width %d: %q", width, line)
			}
		}
	}
	assertLayout("plan queued")
	m, check := keyUpdateResult(m, commandResult[prActionResult](t, refresh))
	assertLayout("checking plan")
	m, post := keyUpdateResult(m, commandResult[prActionResult](t, check))
	assertLayout("sending plan")
	m = deliver(m, commandResult[prActionResult](t, post))
	assertLayout("plan sent")
	if row := stripANSI(m.inboxMetaRow(a, true, 70)); !strings.Contains(row, "plan ok") || !strings.Contains(row, "plan sent") {
		t.Fatalf("local and remote status not distinguished: %q", row)
	}
}
