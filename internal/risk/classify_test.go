package risk

import (
	"testing"

	"github.com/senilio/zappr/internal/model"
)

func TestClassifyDocsOnlySafe(t *testing.T) {
	pr := model.PullRequest{ChangedFiles: 1, Files: []model.ChangedFile{{Path: "docs/usage.md"}}}
	got := Classify(pr)
	if got.Risk != model.RiskSafe {
		t.Fatalf("risk = %s, want %s", got.Risk, model.RiskSafe)
	}
}

func TestClassifyFailingCIBlocks(t *testing.T) {
	pr := model.PullRequest{CIStatus: "FAILURE", ChangedFiles: 1, Files: []model.ChangedFile{{Path: "docs/usage.md"}}}
	got := Classify(pr)
	if got.Risk != model.RiskBlock {
		t.Fatalf("risk = %s, want %s", got.Risk, model.RiskBlock)
	}
}

func TestClassifyAuthBlocks(t *testing.T) {
	pr := model.PullRequest{ChangedFiles: 1, Files: []model.ChangedFile{{Path: "internal/auth/session.go"}}}
	got := Classify(pr)
	if got.Risk != model.RiskBlock {
		t.Fatalf("risk = %s, want %s", got.Risk, model.RiskBlock)
	}
}

func TestClassifyProductionReview(t *testing.T) {
	pr := model.PullRequest{ChangedFiles: 1, Files: []model.ChangedFile{{Path: "internal/app/service.go"}}}
	got := Classify(pr)
	if got.Risk != model.RiskReview {
		t.Fatalf("risk = %s, want %s", got.Risk, model.RiskReview)
	}
}

func TestClassifyLargeChangeBlocks(t *testing.T) {
	pr := model.PullRequest{ChangedFiles: 31, Files: []model.ChangedFile{{Path: "internal/app/service.go"}}}
	got := Classify(pr)
	if got.Risk != model.RiskBlock {
		t.Fatalf("risk = %s, want %s", got.Risk, model.RiskBlock)
	}
}

func TestTrustedAuthorDowngradesReviewToSafe(t *testing.T) {
	pr := model.PullRequest{Author: "alice", ChangedFiles: 1, Files: []model.ChangedFile{{Path: "internal/app/service.go"}}}
	if base := Classify(pr); base.Risk != model.RiskReview {
		t.Fatalf("baseline risk = %s, want review", base.Risk)
	}
	got := ClassifyWithTrust(pr, map[string]bool{"alice": true})
	if got.Risk != model.RiskSafe {
		t.Fatalf("trusted risk = %s, want safe", got.Risk)
	}
	if !containsString(got.Reasons, "trusted author: alice") {
		t.Fatalf("missing trusted reason: %v", got.Reasons)
	}
}

func TestTrustedAuthorMatchCaseInsensitive(t *testing.T) {
	pr := model.PullRequest{Author: "Alice", ChangedFiles: 1, Files: []model.ChangedFile{{Path: "internal/app/service.go"}}}
	got := ClassifyWithTrust(pr, map[string]bool{"alice": true})
	if got.Risk != model.RiskSafe {
		t.Fatalf("trusted risk = %s, want safe", got.Risk)
	}
}

func TestTrustedAuthorRaisesSafeConfidence(t *testing.T) {
	pr := model.PullRequest{Author: "alice", ChangedFiles: 1, Files: []model.ChangedFile{{Path: "docs/usage.md"}}}
	base := Classify(pr)
	got := ClassifyWithTrust(pr, map[string]bool{"alice": true})
	if got.Risk != model.RiskSafe {
		t.Fatalf("risk = %s, want safe", got.Risk)
	}
	if !(got.Confidence > base.Confidence) {
		t.Fatalf("confidence not raised: base=%v got=%v", base.Confidence, got.Confidence)
	}
}

func TestTrustedAuthorNeverOverridesBlock(t *testing.T) {
	trusted := map[string]bool{"alice": true}
	cases := map[string]model.PullRequest{
		"failing CI":     {Author: "alice", CIStatus: "FAILURE", ChangedFiles: 1, Files: []model.ChangedFile{{Path: "docs/usage.md"}}},
		"high-risk path": {Author: "alice", ChangedFiles: 1, Files: []model.ChangedFile{{Path: "internal/auth/session.go"}}},
		"large change":   {Author: "alice", ChangedFiles: 31, Files: []model.ChangedFile{{Path: "internal/app/service.go"}}},
		"draft":          {Author: "alice", Draft: true, ChangedFiles: 1, Files: []model.ChangedFile{{Path: "docs/usage.md"}}},
	}
	for name, pr := range cases {
		got := ClassifyWithTrust(pr, trusted)
		if got.Risk != model.RiskBlock {
			t.Fatalf("%s: risk = %s, want block", name, got.Risk)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
