package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const conversationItem = `{"id":101,"user":{"login":"deploy-bot[bot]"},"body":"plan","html_url":"https://github.com/example/infra/issues/7#issuecomment-101","created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:06Z"}`

func conversationProvider(t *testing.T, command string) *GHProvider {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "gh")
	script := "#!/bin/sh\ncase \"$*\" in\n*issues/7/comments*) " + command + ";;\n*pulls/7/comments*) printf '[[]]';;\n*) printf '{\"number\":7}';;\nesac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &GHProvider{Bin: bin}
}

func TestGetPRConversationPaginationAndProvenance(t *testing.T) {
	second := strings.ReplaceAll(conversationItem, "101", "102")
	third := strings.ReplaceAll(conversationItem, "101", "103")
	p := conversationProvider(t, "printf '%s' "+testShellQuote("[["+conversationItem+","+second+"],["+third+"]]"))
	pr, err := p.GetPR(context.Background(), "example/infra", 7)
	if err != nil {
		t.Fatal(err)
	}
	if !pr.ConversationCommentsLoaded || len(pr.Comments) != 3 {
		t.Fatal(pr)
	}
	for i, id := range []string{"101", "102", "103"} {
		c := pr.Comments[i]
		if c.ID != id || c.URL == "" || c.UpdatedAt.IsZero() || !c.UpdatedAt.After(c.CreatedAt) {
			t.Fatal(c)
		}
	}
}

func TestGetPRConversationFailuresAreNotComplete(t *testing.T) {
	for _, command := range []string{"printf 'rate limit' >&2; exit 1", "printf '[[],broken]'", "printf 'null'"} {
		pr, err := conversationProvider(t, command).GetPR(context.Background(), "example/infra", 7)
		if err == nil || pr.ConversationCommentsLoaded {
			t.Fatalf("%s: %#v %v", command, pr, err)
		}
	}
}

func TestGetPRConversationMissingProvenance(t *testing.T) {
	for _, item := range []string{
		strings.Replace(conversationItem, `"id":101,`, "", 1),
		strings.Replace(conversationItem, `"html_url":"https://github.com/example/infra/issues/7#issuecomment-101",`, "", 1),
		strings.Replace(conversationItem, "2026-01-02T03:04:05Z", "invalid", 1),
	} {
		pr, err := conversationProvider(t, "printf '%s' "+testShellQuote("[["+item+"]]")).GetPR(context.Background(), "example/infra", 7)
		if err == nil || pr.ConversationCommentsLoaded {
			t.Fatal(pr, err)
		}
	}
}
