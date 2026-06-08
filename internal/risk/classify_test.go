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
