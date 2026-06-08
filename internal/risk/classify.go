package risk

import (
	"path/filepath"
	"strings"

	"github.com/senilio/zappr/internal/model"
)

func Classify(pr model.PullRequest) model.Triage {
	reasons := []string{}
	concerns := []string{}
	highRisk := []string{}
	reviewTargets := []string{}

	addReason := func(s string) { reasons = append(reasons, s) }
	addConcern := func(s string) { concerns = append(concerns, s) }

	if pr.Draft {
		addReason("draft pull request")
		return triage(model.RiskBlock, 0.95, pr, reasons, concerns, highRisk, reviewTargets)
	}

	if isFailingCI(pr.CIStatus) {
		addReason("CI is failing")
		return triage(model.RiskBlock, 0.95, pr, reasons, concerns, highRisk, reviewTargets)
	}

	if pr.ChangedFiles == 0 && len(pr.Files) == 0 {
		addConcern("changed files unavailable")
		return triage(model.RiskReview, 0.4, pr, reasons, concerns, highRisk, reviewTargets)
	}

	for _, file := range pr.Files {
		path := normalizePath(file.Path)
		if isHighRiskPath(path) {
			highRisk = append(highRisk, file.Path)
			addReason("high-risk path changed: " + file.Path)
		}
		if isProductionPath(path) {
			reviewTargets = append(reviewTargets, file.Path)
		}
	}

	if len(highRisk) > 0 {
		return triage(model.RiskBlock, 0.9, pr, reasons, concerns, highRisk, reviewTargets)
	}

	if pr.ChangedFiles >= 30 || pr.Size() >= 1000 {
		addReason("large change size")
		return triage(model.RiskBlock, 0.85, pr, reasons, concerns, highRisk, reviewTargets)
	}

	if pr.ChangedFiles >= 10 || pr.Size() >= 200 {
		addReason("moderate change size")
		return triage(model.RiskReview, 0.75, pr, reasons, concerns, highRisk, reviewTargets)
	}

	if onlyLowRiskFiles(pr.Files) {
		addReason("only low-risk files changed")
		return triage(model.RiskSafe, 0.8, pr, reasons, concerns, highRisk, reviewTargets)
	}

	if len(reviewTargets) > 0 {
		addReason("production code changed")
		if !hasTests(pr.Files) {
			addConcern("no test files changed")
		}
		return triage(model.RiskReview, 0.7, pr, reasons, concerns, highRisk, reviewTargets)
	}

	addConcern("change type unclear")
	return triage(model.RiskReview, 0.5, pr, reasons, concerns, highRisk, reviewTargets)
}

func triage(risk model.Risk, confidence float64, pr model.PullRequest, reasons, concerns, highRisk, reviewTargets []string) model.Triage {
	action := "inspect"
	if risk == model.RiskSafe {
		action = "approve"
	}
	if risk == model.RiskBlock {
		action = "request_changes"
	}

	return model.Triage{
		Risk:            risk,
		Confidence:      confidence,
		Summary:         summaryFor(risk, pr),
		Reasons:         dedupe(reasons),
		Concerns:        dedupe(concerns),
		HighRiskFiles:   dedupe(highRisk),
		ReviewTargets:   dedupe(reviewTargets),
		SuggestedAction: action,
		ReviewComment:   "",
	}
}

func summaryFor(risk model.Risk, pr model.PullRequest) string {
	switch risk {
	case model.RiskSafe:
		return "Small low-risk change; quick review likely."
	case model.RiskBlock:
		return "Unsafe for quick approval; manual review required."
	default:
		return "Human inspection recommended."
	}
}

func isFailingCI(status string) bool {
	s := strings.ToLower(status)
	return strings.Contains(s, "fail") || strings.Contains(s, "failure") || strings.Contains(s, "error")
}

func normalizePath(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(path), "./")
}

func isHighRiskPath(path string) bool {
	lower := strings.ToLower(path)
	highRiskParts := []string{
		"auth", "security", "billing", "payment", "payments", "migration", "migrations",
		"terraform", "infra", ".github/workflows", "api/", "openapi", "schema",
	}
	for _, part := range highRiskParts {
		if strings.Contains(lower, part) {
			return true
		}
	}
	base := filepath.Base(lower)
	return base == "go.mod" || base == "go.sum" || base == "package-lock.json" || base == "yarn.lock" || base == "pnpm-lock.yaml"
}

func isProductionPath(path string) bool {
	lower := strings.ToLower(path)
	if isLowRiskPath(lower) || isGeneratedPath(lower) || isTestPath(lower) {
		return false
	}
	ext := filepath.Ext(lower)
	switch ext {
	case ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".rb", ".rs", ".java", ".kt", ".swift", ".cs", ".sql":
		return true
	default:
		return false
	}
}

func onlyLowRiskFiles(files []model.ChangedFile) bool {
	if len(files) == 0 {
		return false
	}
	for _, file := range files {
		path := normalizePath(file.Path)
		if !(isLowRiskPath(path) || isGeneratedPath(path) || isTestPath(path)) {
			return false
		}
	}
	return true
}

func isLowRiskPath(path string) bool {
	lower := strings.ToLower(path)
	base := filepath.Base(lower)
	return strings.HasPrefix(lower, "docs/") || strings.HasPrefix(lower, ".github/") || base == "readme.md" || base == "license" || filepath.Ext(lower) == ".md"
}

func isGeneratedPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "/generated/") || strings.Contains(lower, "/gen/") || strings.HasSuffix(lower, ".pb.go") || strings.HasSuffix(lower, ".gen.go")
}

func isTestPath(path string) bool {
	lower := strings.ToLower(path)
	base := filepath.Base(lower)
	return strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/") || strings.Contains(base, "test") || strings.Contains(base, "spec")
}

func hasTests(files []model.ChangedFile) bool {
	for _, file := range files {
		if isTestPath(file.Path) {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
