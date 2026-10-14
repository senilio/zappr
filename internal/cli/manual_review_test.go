package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/cache"
	"github.com/senilio/zappr/internal/github"
	"github.com/senilio/zappr/internal/model"
)

func TestReviewCommandIsRemoved(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte("#!/bin/sh\necho unexpected GitHub invocation >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write isolated gh: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := Run([]string{"review", "owner/repo#123"}); err == nil || !strings.Contains(err.Error(), `unknown command "review"`) {
		t.Fatalf("review command error = %v, want unknown command", err)
	}
	if strings.Contains(usageText(), "zappr review") {
		t.Fatal("usage still advertises the removed review command")
	}
}

func TestApproveRunsWithoutFetchingPRDetails(t *testing.T) {
	approvalErr := errors.New("approval denied")
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"success", nil},
		{"provider error", approvalErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &manualApprovalProvider{err: tc.err}
			zappr := &app.App{GitHub: provider, Cache: cache.NewMemoryStore()}
			if err := runApprove(zappr, []string{"owner/repo#123"}); !errors.Is(err, tc.err) {
				t.Fatalf("approve error = %v, want %v", err, tc.err)
			}
			if provider.fetches != 0 || provider.approvals != 1 || provider.repo != "owner/repo" || provider.number != 123 || provider.body != "" {
				t.Fatalf("approval calls = %#v, want one direct manual approval", provider)
			}
		})
	}
}

func TestApproveRejectsInvalidReferenceWithoutCallingProvider(t *testing.T) {
	provider := &manualApprovalProvider{}
	zappr := &app.App{GitHub: provider, Cache: cache.NewMemoryStore()}
	if err := runApprove(zappr, []string{"owner/repo#0"}); err == nil {
		t.Fatal("invalid PR reference accepted")
	}
	if provider.approvals != 0 || provider.fetches != 0 {
		t.Fatalf("invalid reference called provider: %#v", provider)
	}
}

type manualApprovalProvider struct {
	github.Provider
	err       error
	fetches   int
	approvals int
	repo      string
	number    int
	body      string
}

func (p *manualApprovalProvider) GetPR(context.Context, string, int) (model.PullRequest, error) {
	p.fetches++
	return model.PullRequest{}, errors.New("PR details unavailable")
}

func (p *manualApprovalProvider) GetDiff(context.Context, string, int) (string, error) {
	p.fetches++
	return "", errors.New("diff unavailable")
}

func (p *manualApprovalProvider) Approve(_ context.Context, repo string, number int, body string) error {
	p.approvals++
	p.repo, p.number, p.body = repo, number, body
	return p.err
}
