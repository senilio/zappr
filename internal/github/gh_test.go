package github

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/model"
)

func TestParseTime(t *testing.T) {
	got := parseTime("2024-01-02T03:04:05Z")
	want := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parseTime = %s, want %s", got, want)
	}
}

func TestParseTimeInvalid(t *testing.T) {
	if !parseTime("nope").IsZero() {
		t.Fatal("invalid timestamp should return zero time")
	}
}

func TestListPRsUsesSingleListCallWithoutCommitsOrComments(t *testing.T) {
	dir := t.TempDir()
	callsPath := filepath.Join(dir, "calls")
	bin := filepath.Join(dir, "gh")
	response := `[{"number":7,"title":"Draft","isDraft":true,"autoMergeRequest":{"enabledAt":"2026-10-07T10:00:00Z"},"files":[{"path":"old.go","additions":0,"deletions":4,"changeType":"DELETED"}]}]`
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + testShellQuote(callsPath) + "\nprintf '%s' " + testShellQuote(response) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	prs, complete, err := (&GHProvider{Bin: bin}).ListPRs(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if !complete {
		t.Fatal("complete = false, want true")
	}
	if len(prs) != 1 || !prs[0].Draft || !prs[0].AutoMerge || len(prs[0].Files) != 1 {
		t.Fatalf("PRs not fully decoded: %#v", prs)
	}
	if prs[0].Files[0].Status != "removed" {
		t.Fatalf("file status = %q, want removed", prs[0].Files[0].Status)
	}

	data, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read calls: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	fields := "number,title,author,url,labels,additions,deletions,changedFiles,isDraft,autoMergeRequest,state,headRefOid,statusCheckRollup,reviewDecision,files,updatedAt"
	want := []string{
		"pr list --repo owner/repo --state open --search sort:updated-desc --limit 60 --json " + fields,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestApproveOmitsBodyWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "gh")
	script := "#!/bin/sh\nprintf '%s\n' \"$@\" > " + testShellQuote(argsPath) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	provider := &GHProvider{Bin: bin}
	if err := provider.Approve(context.Background(), "owner/repo", 123, ""); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	data, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"pr", "review", "123", "--repo", "owner/repo", "--approve"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestMergePREnablesAutoMerge(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "gh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + testShellQuote(argsPath) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	provider := &GHProvider{Bin: bin}
	if err := provider.MergePR(context.Background(), "owner/repo", 123); err != nil {
		t.Fatalf("MergePR: %v", err)
	}

	data, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"pr", "merge", "123", "--repo", "owner/repo", "--auto", "--squash", "--delete-branch"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func testShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func TestToggleDraft(t *testing.T) {
	for _, draft := range []bool{true, false} {
		t.Run(fmt.Sprint(draft), func(t *testing.T) {
			dir := t.TempDir()
			argsPath := filepath.Join(dir, "args")
			bin := filepath.Join(dir, "gh")
			script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + testShellQuote(argsPath) + "\n"
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := (&GHProvider{Bin: bin}).ToggleDraft(context.Background(), "owner/repo", 123, draft); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatal(err)
			}
			want := "pr ready 123 --repo owner/repo"
			if draft {
				want = "pr ready 123 --repo owner/repo --undo"
			}
			if got := strings.Join(strings.Fields(string(data)), " "); got != want {
				t.Fatalf("args = %q, want %q", got, want)
			}
		})
	}
}

func TestToggleAutoMerge(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			dir := t.TempDir()
			argsPath := filepath.Join(dir, "args")
			bin := filepath.Join(dir, "gh")
			script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + testShellQuote(argsPath) + "\n"
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := (&GHProvider{Bin: bin}).ToggleAutoMerge(context.Background(), "owner/repo", 123, enabled); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatal(err)
			}
			want := "pr merge 123 --repo owner/repo --disable-auto"
			if enabled {
				want = "pr merge 123 --repo owner/repo --auto --squash --delete-branch"
			}
			if got := strings.Join(strings.Fields(string(data)), " "); got != want {
				t.Fatalf("args = %q, want %q", got, want)
			}
		})
	}
}

func TestGetPRAutoMergeState(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "gh")
			request := "null"
			if enabled {
				request = `{"enabledAt":"2026-10-07T10:00:00Z"}`
			}
			script := "#!/bin/sh\nif [ \"$1\" = api ]; then printf '[[]]'; exit; fi\ncase \"$*\" in *autoMergeRequest*) ;; *) exit 1 ;; esac\nprintf '%s' " + testShellQuote(`{"number":123,"autoMergeRequest":`+request+`}`) + "\n"
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			pr, err := (&GHProvider{Bin: bin}).GetPR(context.Background(), "owner/repo", 123)
			if err != nil {
				t.Fatal(err)
			}
			if pr.AutoMerge != enabled {
				t.Fatalf("AutoMerge = %v, want %v", pr.AutoMerge, enabled)
			}
		})
	}
}

func TestGetPRPreservesBotConversationComments(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "gh")
	response := `{"number":7,"comments":[{"author":{"login":"bot"},"body":"Plan: 1 to add, 2 to change, 0 to destroy.","createdAt":"2026-01-02T03:04:05Z"}]}`
	script := "#!/bin/sh\nif [ \"$1\" = api ]; then printf '[[]]'; exit; fi\nprintf '%s' " + testShellQuote(response) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	pr, err := (&GHProvider{Bin: bin}).GetPR(context.Background(), "owner/repo", 7)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	want := []model.PRComment{{Author: "bot", Body: "Plan: 1 to add, 2 to change, 0 to destroy.", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}}
	if !pr.DetailsLoaded || !reflect.DeepEqual(pr.Comments, want) {
		t.Fatalf("conversation comments not preserved: %#v", pr)
	}
}

func TestGetPRIncludesReviewBodiesAndPaginatedInlineComments(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "gh")
	callsPath := filepath.Join(dir, "calls")
	response := `{"number":7,"comments":[{"author":{"login":"bot"},"body":"Review started","createdAt":"2026-10-14T12:40:07Z"}],"reviews":[{"author":{"login":"bot"},"body":"Code Review summary","submittedAt":"2026-10-14T12:40:53Z","state":"COMMENTED"},{"body":"  ","state":"APPROVED"},{"body":"Draft review","state":"PENDING"}]}`
	pages := `[[{"user":{"login":"bot[bot]"},"body":"Inline finding","path":"roles.tf","line":291,"original_line":290,"side":"RIGHT","created_at":"2026-10-14T12:40:53Z"}],[{"user":{"login":"alice"},"body":"Outdated reply","path":"old.tf","line":null,"original_line":10,"side":"LEFT","in_reply_to_id":123,"created_at":"2026-10-14T12:41:00Z"},{"user":{"login":"bob"},"body":"File comment","path":"file.tf","line":null,"original_line":null,"subject_type":"file","created_at":"2026-10-14T12:42:00Z"}]]`
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + testShellQuote(callsPath) + "\ncase \"$1\" in\npr) printf '%s' " + testShellQuote(response) + ";;\napi) printf '%s' " + testShellQuote(pages) + ";;\n*) exit 1;;\nesac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	pr, err := (&GHProvider{Bin: bin}).GetPR(context.Background(), "owner/repo", 7)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if len(pr.Comments) != 5 {
		t.Fatalf("comments = %#v, want conversation, review body, and all three inline comments", pr.Comments)
	}
	if !pr.DetailsLoaded || !pr.ReviewCommentsLoaded {
		t.Fatal("complete details must be marked loaded")
	}
	if c := pr.Comments[1]; c.Kind != "review" || c.Body != "Code Review summary" || c.Author != "bot" || !c.CreatedAt.Equal(parseTime("2026-10-14T12:40:53Z")) {
		t.Fatalf("review body = %#v", c)
	}
	if c := pr.Comments[2]; c.Kind != "inline" || c.Path != "roles.tf" || c.Line != 291 || c.OriginalLine != 290 || c.Side != "RIGHT" || c.Author != "bot[bot]" || c.Body != "Inline finding" || !c.CreatedAt.Equal(parseTime("2026-10-14T12:40:53Z")) {
		t.Fatalf("inline comment = %#v", c)
	}
	if c := pr.Comments[3]; c.Line != 0 || c.OriginalLine != 10 || c.Side != "LEFT" || c.Body != "Outdated reply" {
		t.Fatalf("outdated reply = %#v", c)
	}
	if c := pr.Comments[4]; c.Line != 0 || c.OriginalLine != 0 || c.Path != "file.tf" {
		t.Fatalf("file comment = %#v", c)
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], ",reviews,") || lines[1] != "api repos/owner/repo/pulls/7/comments?per_page=100 --paginate --slurp" {
		t.Fatalf("gh calls = %q, want reviews field and paginated inline endpoint", calls)
	}
}

func TestGetPRSurfacesInlineCommentErrors(t *testing.T) {
	for _, tc := range []struct{ name, command, want string }{
		{"network", "printf 'rate limit exceeded' >&2; exit 1", "rate limit exceeded"},
		{"invalid JSON", "printf 'invalid'", "parse gh review comments"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "gh")
			script := "#!/bin/sh\nif [ \"$1\" = pr ]; then printf '{\"number\":7}'; else " + tc.command + "; fi\n"
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			pr, err := (&GHProvider{Bin: bin}).GetPR(context.Background(), "owner/repo", 7)
			if err == nil || !strings.Contains(err.Error(), tc.want) || pr.DetailsLoaded {
				t.Fatalf("GetPR = %#v, %v, want surfaced error %q and no loaded details", pr, err, tc.want)
			}
		})
	}
}

func TestParseChecksFromRollup(t *testing.T) {
	raw := []byte(`[
		{"__typename":"CheckRun","name":"Check tflint","workflowName":"check-tflint","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-10-29T16:25:39Z","completedAt":"2026-10-29T16:27:17Z","detailsUrl":"https://example/run"},
		{"__typename":"StatusContext","context":"atlantis/plan: sso-sso","state":"SUCCESS","startedAt":"2026-10-29T16:31:14Z","targetUrl":"https://atlantis.example/jobs/abc"}
	]`)
	checks := parseChecks(raw)
	if len(checks) != 2 {
		t.Fatalf("parseChecks returned %d checks, want 2: %#v", len(checks), checks)
	}
	run := checks[0]
	if run.Name != "Check tflint" || run.WorkflowName != "check-tflint" || run.State != "SUCCESS" || run.Context != "" || run.URL != "https://example/run" || run.StartedAt.IsZero() || run.CompletedAt.IsZero() {
		t.Fatalf("CheckRun = %#v", run)
	}
	status := checks[1]
	if status.Name != "atlantis/plan: sso-sso" || status.Context != "atlantis/plan: sso-sso" || status.State != "SUCCESS" || status.URL != "https://atlantis.example/jobs/abc" || status.StartedAt.IsZero() {
		t.Fatalf("StatusContext = %#v", status)
	}
	for _, empty := range [][]byte{nil, []byte("null")} {
		got := parseChecks(empty)
		if got == nil || len(got) != 0 {
			t.Fatalf("parseChecks(%q) = %#v, want non-nil empty slice", empty, got)
		}
	}
}

func TestParseChecksNormalizesStates(t *testing.T) {
	raw := []byte(`[
		{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"SKIPPED"},
		{"__typename":"CheckRun","name":"b","status":"COMPLETED","conclusion":""},
		{"__typename":"CheckRun","name":"c","status":"IN_PROGRESS","conclusion":""},
		{"__typename":"CheckRun","name":"d","status":"COMPLETED","conclusion":"TIMED_OUT"},
		{"__typename":"CheckRun","name":"e","status":"COMPLETED","conclusion":"WEIRD"},
		{"__typename":"StatusContext","context":"f","state":"EXPECTED"},
		{"__typename":"StatusContext","context":"g","state":"ERROR"},
		{"__typename":"StatusContext","context":"h","state":"pending"},
		{"__typename":"StatusContext","context":"i","state":"FAILURE"}
	]`)
	want := []string{"SUCCESS", "SUCCESS", "PENDING", "FAILURE", "", "EXPECTED", "ERROR", "PENDING", "FAILURE"}
	checks := parseChecks(raw)
	if len(checks) != len(want) {
		t.Fatalf("parseChecks returned %d checks, want %d", len(checks), len(want))
	}
	for i, w := range want {
		if checks[i].State != w {
			t.Errorf("check %q state = %q, want %q", checks[i].Name, checks[i].State, w)
		}
	}
}
