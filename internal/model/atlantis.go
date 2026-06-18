package model

import (
	"fmt"
	"regexp"
	"strings"
)

var atlantisPlanSummaryPattern = regexp.MustCompile(`(?i)Plan:\s*(\d+)\s+to\s+add,\s*(\d+)\s+to\s+change,\s*(\d+)\s+to\s+destroy`)
var atlantisLayerDirWorkspacePattern = regexp.MustCompile("(?i)(?:dir|directory|path):\\s*`?([^`,\\s]+)`?.*workspace:\\s*`?([^`,\\s]+)`?")
var atlantisLayerNamedPattern = regexp.MustCompile("(?i)(?:project|layer|workspace|dir|directory|path):\\s*`?([^`,\\s]+)`?")

// LatestAtlantisPlan returns the newest comment that looks like Atlantis plan
// output, with a small structured summary for display and AI review input.
func LatestAtlantisPlan(comments []PRComment) *AtlantisPlan {
	for i := len(comments) - 1; i >= 0; i-- {
		comment := comments[i]
		if !looksLikeAtlantisPlan(comment) {
			continue
		}
		plan := AtlantisPlan{
			Author:    comment.Author,
			Body:      comment.Body,
			CreatedAt: comment.CreatedAt,
			Layers:    AtlantisPlanLayers(comment.Body),
		}
		if match := atlantisPlanSummaryPattern.FindStringSubmatch(comment.Body); len(match) == 4 {
			plan.Add = atoi(match[1])
			plan.Change = atoi(match[2])
			plan.Destroy = atoi(match[3])
			plan.Summary = fmt.Sprintf("Plan: %d to add, %d to change, %d to destroy", plan.Add, plan.Change, plan.Destroy)
			return &plan
		}
		lower := strings.ToLower(comment.Body)
		if strings.Contains(lower, "no changes") || strings.Contains(lower, "no changes. infrastructure is up-to-date") {
			plan.NoChanges = true
			plan.Summary = "No changes"
			return &plan
		}
		plan.Summary = "Atlantis plan output found"
		return &plan
	}
	return nil
}

func AtlantisPlanLayers(body string) []AtlantisLayer {
	var layers []AtlantisLayer
	current := AtlantisLayer{Name: "overall"}
	hasCurrent := false

	flush := func() {
		if !hasCurrent {
			return
		}
		if current.Name == "" {
			current.Name = "unknown"
		}
		layers = append(layers, current)
		current = AtlantisLayer{Name: "overall"}
		hasCurrent = false
	}

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(stripMarkdown(line))
		if trimmed == "" {
			continue
		}
		if name := atlantisLayerName(trimmed); name != "" {
			flush()
			current = AtlantisLayer{Name: name}
			hasCurrent = true
			continue
		}
		lower := strings.ToLower(trimmed)
		if match := atlantisPlanSummaryPattern.FindStringSubmatch(trimmed); len(match) == 4 {
			current.Add = atoi(match[1])
			current.Change = atoi(match[2])
			current.Destroy = atoi(match[3])
			current.NoChanges = current.Add == 0 && current.Change == 0 && current.Destroy == 0
			hasCurrent = true
			continue
		}
		if strings.Contains(lower, "no changes") || strings.Contains(lower, "infrastructure is up-to-date") {
			current.NoChanges = true
			hasCurrent = true
			continue
		}
		if strings.Contains(lower, "terraform will perform") || strings.Contains(lower, "resources will be") {
			current.Unknown = true
			hasCurrent = true
		}
	}
	flush()
	return dedupeAtlantisLayers(layers)
}

func atlantisLayerName(line string) string {
	if isAtlantisNoiseLayerName(line) {
		return ""
	}
	if match := atlantisLayerDirWorkspacePattern.FindStringSubmatch(line); len(match) == 3 {
		return match[1] + " / " + match[2]
	}
	if match := atlantisLayerNamedPattern.FindStringSubmatch(line); len(match) == 2 {
		return match[1]
	}
	if strings.HasPrefix(line, "#") {
		name := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if name != "" && len(name) <= 80 && !isAtlantisNoiseLayerName(name) {
			return name
		}
	}
	return ""
}

func isAtlantisNoiseLayerName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" || lower == "plan summary" {
		return true
	}
	return strings.HasPrefix(lower, "(") && strings.Contains(lower, "unchanged") && strings.Contains(lower, "hidden")
}

func stripMarkdown(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`*_ ")
	return strings.TrimSpace(s)
}

func dedupeAtlantisLayers(layers []AtlantisLayer) []AtlantisLayer {
	seen := make(map[string]int)
	var out []AtlantisLayer
	for _, layer := range layers {
		idx, ok := seen[layer.Name]
		if !ok {
			seen[layer.Name] = len(out)
			out = append(out, layer)
			continue
		}
		merged := out[idx]
		merged.Add += layer.Add
		merged.Change += layer.Change
		merged.Destroy += layer.Destroy
		merged.NoChanges = merged.NoChanges || layer.NoChanges
		merged.Unknown = merged.Unknown || layer.Unknown
		out[idx] = merged
	}
	return out
}

func looksLikeAtlantisPlan(comment PRComment) bool {
	lowerAuthor := strings.ToLower(comment.Author)
	lowerBody := strings.ToLower(comment.Body)
	if strings.Contains(lowerAuthor, "atlantis") && (strings.Contains(lowerBody, "plan:") || strings.Contains(lowerBody, "terraform will perform") || strings.Contains(lowerBody, "no changes")) {
		return true
	}
	return strings.Contains(lowerBody, "atlantis plan") && (strings.Contains(lowerBody, "plan:") || strings.Contains(lowerBody, "terraform will perform") || strings.Contains(lowerBody, "no changes"))
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}
