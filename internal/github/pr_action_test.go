package github

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/senilio/zappr/internal/model"
)

func TestCheckPRActionSeparatesTargetAndEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, view, api, state, head, evidence string
		targetErr, known                       bool
		checks                                 int
	}{
		{"same head", `{"number":7,"state":"OPEN","headRefOid":"old","statusCheckRollup":[]}`, "failure", "OPEN", "old", "rate limit", false, true, 0},
		{"changed head", `{"number":7,"state":"OPEN","headRefOid":"new","statusCheckRollup":[{"__typename":"StatusContext","context":"atlantis/plan","state":"PENDING"}]}`, "failure", "OPEN", "new", "rate limit", false, true, 1},
		{"closed", `{"number":7,"state":"CLOSED"}`, "failure", "CLOSED", "", "", false, false, 0},
		{"known empty", `{"number":7,"state":"OPEN","headRefOid":"old","statusCheckRollup":[]}`, "empty", "OPEN", "old", "", false, true, 0},
		{"missing rollup", `{"number":7,"state":"OPEN","headRefOid":"old"}`, "empty", "OPEN", "old", "check rollup", false, false, 0},
		{"malformed rollup", `{"number":7,"state":"OPEN","headRefOid":"old","statusCheckRollup":[{},{"__typename":"StatusContext","context":"atlantis/plan","state":"PENDING"}]}`, "empty", "OPEN", "old", "check rollup", false, false, 1},
		{"unknown state", `{"number":7,"headRefOid":"old"}`, "failure", "", "", "", true, false, 0},
		{"missing head", `{"number":7,"state":"OPEN"}`, "failure", "", "", "", true, false, 0},
		{"wrong PR", `{"number":8,"state":"OPEN","headRefOid":"old"}`, "failure", "", "", "", true, false, 0},
		{"wrong repo", `{"number":7,"state":"OPEN","headRefOid":"old","url":"https://github.com/other/repo/pull/7"}`, "failure", "", "", "", true, false, 0},
		{"fetch failed", "", "failure", "", "", "", true, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "gh")
			log := filepath.Join(dir, "log")
			api := "printf 'rate limit' >&2; exit 1"
			if tc.api == "empty" {
				api = "printf '[[]]'"
			}
			view := "printf '%s' " + testShellQuote(tc.view)
			if tc.view == "" {
				view = "printf 'view failed' >&2; exit 1"
			}
			script := "#!/bin/sh\n" +
				"case \"$*\" in\n" +
				"  'api repos/owner/repo/issues/7/comments?per_page=100 --paginate --slurp') echo comments >>" + testShellQuote(log) + "; " + api + " ;;\n" +
				"  'pr view 7 --repo owner/repo --json '*) echo view >>" + testShellQuote(log) + "; " + view + " ;;\n" +
				"  *) exit 2 ;;\nesac\n"
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			check, err := (&GHProvider{Bin: bin}).CheckPRAction(t.Context(), "owner/repo", 7, true)
			if (err != nil) != tc.targetErr || check.PR.State != tc.state || check.PR.HeadRefOID != tc.head {
				t.Fatalf("target = %+v, err=%v", check.PR, err)
			}
			if check.ChecksKnown != tc.known || len(check.PR.Checks) != tc.checks ||
				(tc.evidence == "" && check.EvidenceErr != nil) ||
				(tc.evidence != "" && (check.EvidenceErr == nil || !strings.Contains(check.EvidenceErr.Error(), tc.evidence))) {
				t.Fatalf("evidence = checks %+v known=%v err=%v", check.PR.Checks, check.ChecksKnown, check.EvidenceErr)
			}
			trace, err := os.ReadFile(log)
			if err != nil || string(trace) != "comments\nview\n" {
				t.Fatalf("call order = %q, %v", trace, err)
			}
		})
	}
}

func TestCheckPRActionPassiveReadReportsClosureWithoutDetails(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "gh")
	script := `#!/bin/sh
case "$1 $2" in
  "pr view") printf '%s' '{"number":7,"state":"CLOSED"}' ;;
  *) printf 'details unavailable' >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	pr, err := (&GHProvider{Bin: bin}).GetPR(t.Context(), "owner/repo", 7)
	if err != nil || pr.State != "CLOSED" || pr.Number != 7 {
		t.Fatalf("GetPR = %+v, %v; want a known closure", pr, err)
	}
}

func TestCheckPRActionKeepsReadableGuardEvidence(t *testing.T) {
	const unlock = `{"id":1,"html_url":"https://github.com/owner/repo/pull/7#issuecomment-1","user":{"login":"atlantis[bot]"},"body":"All Atlantis locks for this PR have been unlocked and plans discarded","created_at":"2026-11-23T08:00:00Z","updated_at":"2026-11-23T08:00:00Z"}`
	for _, response := range []string{
		"[[" + unlock + ",{}]]",
		"[[{}, " + unlock + "]]",
		`[[{"id":"invalid"},` + unlock + `]]`,
		"[null,[" + unlock + "]]",
	} {
		t.Run(response, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "gh")
			script := "#!/bin/sh\n" +
				"case \"$1\" in\n" +
				"  api) printf '%s' " + testShellQuote(response) + " ;;\n" +
				`  pr) printf '%s' '{"number":7,"state":"OPEN","headRefOid":"old","statusCheckRollup":[]}' ;;` + "\nesac\n"
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			check, err := (&GHProvider{Bin: bin}).CheckPRAction(t.Context(), "owner/repo", 7, true)
			atlantis := model.DeriveAtlantis(check.PR.Checks, check.PR.Comments...)
			if err != nil || check.EvidenceErr == nil || check.PR.ConversationCommentsLoaded || atlantis.Plan != model.PlanDiscarded {
				t.Fatalf("want readable unlock and incomplete evidence, got %v, %v, %+v", err, check.EvidenceErr, check.PR)
			}
		})
	}
}

func TestListPRsDoesNotTreatNullAsACompleteList(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, complete, err := (&GHProvider{Bin: bin}).ListPRs(t.Context(), "owner/repo"); err == nil || complete {
		t.Fatalf("null response: complete=%v, err=%v", complete, err)
	}
}

func TestCommentFailureReportsCauseWithoutEchoingBody(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf 'rate limit' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	const body = "atlantis plan -p example"
	err := (&GHProvider{Bin: bin}).Comment(t.Context(), "owner/repo", 7, body)
	if err == nil || err.Error() != "gh pr comment: rate limit" {
		t.Fatalf("Comment error = %v", err)
	}
}
