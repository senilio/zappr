package github

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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

func TestListPRsFetchesCompletePRsWithoutDetailCalls(t *testing.T) {
	dir := t.TempDir()
	callsPath := filepath.Join(dir, "calls")
	bin := filepath.Join(dir, "gh")
	response := `[{"number":7,"title":"Draft","isDraft":true,"files":[{"path":"old.go","additions":0,"deletions":4,"changeType":"DELETED"}],"commits":[{"oid":"abc","messageHeadline":"change","committedDate":"2024-01-02T03:04:05Z","authors":[{"user":{"login":"octocat"}}]}],"comments":[{"author":{"login":"octocat"},"body":"ready","createdAt":"2024-01-02T03:04:05Z"}]}]`
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
	if len(prs) != 2 || !prs[0].Draft || len(prs[0].Files) != 1 || len(prs[0].Commits) != 1 || len(prs[0].Comments) != 1 {
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
	fields := "number,title,author,url,labels,additions,deletions,changedFiles,isDraft,state,headRefOid,statusCheckRollup,reviewDecision,files,commits,comments,updatedAt"
	want := []string{
		"pr list --repo owner/repo --state open --search draft:false sort:updated-desc --limit 30 --json " + fields,
		"pr list --repo owner/repo --state open --search draft:true sort:updated-desc --limit 30 --json " + fields,
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

func testShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
