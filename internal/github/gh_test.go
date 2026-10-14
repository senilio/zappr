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
			script := "#!/bin/sh\ncase \"$*\" in *autoMergeRequest*) ;; *) exit 1 ;; esac\nprintf '%s' " + testShellQuote(`{"number":123,"autoMergeRequest":`+request+`}`) + "\n"
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
	script := "#!/bin/sh\nprintf '%s' " + testShellQuote(response) + "\n"
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
