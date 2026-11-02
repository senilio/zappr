package plan

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/senilio/zappr/internal/model"
)

const continued = "Continued plan output from previous comment."
const continuationWarning = "**Warning**: Output length greater than max comment size. Continued in next comment."

var projectHeader = regexp.MustCompile("^Ran Plan for project: `([^`]+)` dir: `([^`]+)` workspace: `([^`]+)`\\s*$")
var aggregateProjectHeader = regexp.MustCompile("^### (?:[0-9]+\\. )?project: `([^`]+)` dir: `([^`]+)` workspace: `([^`]+)`\\s*$")
var aggregateHeader = regexp.MustCompile(`^Ran Plan for ([0-9]+) projects:\s*$`)
var rosterEntry = regexp.MustCompile("^[0-9]+\\. project: `([^`]+)` dir: `([^`]+)` workspace: `([^`]+)`\\s*$")

func projectSection(line string) []string {
	line = strings.TrimSpace(line)
	if match := projectHeader.FindStringSubmatch(line); match != nil {
		return match
	}
	return aggregateProjectHeader.FindStringSubmatch(line)
}

func conversation(comments []model.PRComment) []model.PRComment {
	var out []model.PRComment
	for _, c := range comments {
		if c.Kind == "" || c.Kind == "comment" {
			out = append(out, c)
		}
	}
	return out
}

func Fingerprint(comments []model.PRComment, conversationComplete bool) string {
	b, _ := json.Marshal(struct {
		Comments []model.PRComment
		Complete bool
	}{conversation(comments), conversationComplete})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func Parse(comments []model.PRComment, conversationComplete bool) Document {
	ordered := conversation(comments)
	slices.SortStableFunc(ordered, func(a, b model.PRComment) int { return a.CreatedAt.Compare(b.CreatedAt) })
	var doc Document
	// A continuation names neither its project nor its part, so it is only
	// attributable when no other root is also waiting for one.
	awaiting := 0
	for i := 0; i < len(ordered); i++ {
		c := ordered[i]
		first := firstLine(c.Body)
		match := projectHeader.FindStringSubmatch(first)
		aggregate := aggregateHeader.MatchString(first)
		orphan := first == continued
		if match == nil && !aggregate && !orphan {
			continue
		}
		r := Report{ID: c.Key() + ":0", SourceComplete: conversationComplete && nativeSource(c), Outcome: OutcomeUnknown, Coverage: CoverageProjects}
		chain := []model.PRComment{c}
		addSource(&r, c)
		if !conversationComplete {
			r.Issues = append(r.Issues, "Conversation retrieval is incomplete or awaiting refresh")
		}
		if !nativeSource(c) {
			r.Issues = append(r.Issues, "Native comment provenance unavailable; fragment assembly disabled")
		}
		if aggregate {
			r.Coverage = CoverageAggregate
			r.aggregateText = normalize(c.Body)
			for _, line := range strings.Split(r.aggregateText, "\n") {
				if projectSection(line) != nil || strings.TrimSpace(line) == "---" || strings.HasPrefix(strings.TrimSpace(line), "### Plan Summary") {
					break
				}
				if m := rosterEntry.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
					r.Projects = append(r.Projects, newProject(r.ID, m[1], m[2], m[3]))
				}
			}
			if strings.Contains(r.aggregateText, continuationWarning) {
				r.SourceComplete = false
				r.Issues = append(r.Issues, "Report continues in another comment; multi-project continuations are not assembled")
				awaiting++
			}
		} else {
			p := newProject(r.ID, "Unidentified project", "", "")
			if match != nil {
				p = newProject(r.ID, match[1], match[2], match[3])
			}
			needs, valid := appendOutput(&p, c)
			if !valid {
				r.SourceComplete = false
				r.Issues = append(r.Issues, "Unsupported output wrapper; inspect original source")
			}
			if orphan {
				r.SourceComplete = false
				r.Coverage = CoverageUnknown
				r.Issues = append(r.Issues, "Orphan continuation; project/run association unknown")
			}
			if needs && !orphan && awaiting > 0 {
				r.SourceComplete = false
				r.Issues = append(r.Issues, "Another report is also awaiting a continuation; fragments cannot be attributed")
			}
			for needs && !orphan && r.SourceComplete {
				if i+1 >= len(ordered) || firstLine(ordered[i+1].Body) != continued || ordered[i+1].Author != c.Author || c.Author == "" || !nativeSource(ordered[i+1]) {
					break
				}
				i++
				next := ordered[i]
				chain = append(chain, next)
				addSource(&r, next)
				needs, valid = appendOutput(&p, next)
				if !valid {
					r.SourceComplete = false
					r.Issues = append(r.Issues, "Unsupported continuation wrapper")
				}
			}
			if needs {
				r.SourceComplete = false
				r.Issues = append(r.Issues, "Continuation missing or ambiguous; received fragments are not a complete report")
				if !orphan {
					awaiting++
				}
			}
			r.Projects = []Project{p}
		}
		r.Revision = Fingerprint(chain, conversationComplete)
		indexReport(&r)
		doc.Reports = append(doc.Reports, r)
	}
	slices.SortStableFunc(doc.Reports, func(a, b Report) int { return b.LatestActivity.Compare(a.LatestActivity) })
	return doc
}

func nativeSource(c model.PRComment) bool {
	return c.ID != "" && c.URL != "" && !c.CreatedAt.IsZero() && !c.UpdatedAt.IsZero()
}
func normalize(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
func firstLine(s string) string {
	for _, line := range strings.Split(normalize(s), "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
func newProject(reportID, name, dir, workspace string) Project {
	b, _ := json.Marshal([]string{name, dir, workspace})
	return Project{ID: fmt.Sprintf("%s/project:%x", reportID, sha256.Sum256(b)), Name: name, Dir: dir, Workspace: workspace, Outcome: OutcomeUnknown}
}
func addSource(r *Report, c model.PRComment) {
	r.Sources = append(r.Sources, Source{CommentKey: c.Key(), URL: c.URL, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt})
	activity := c.UpdatedAt
	if activity.IsZero() {
		activity = c.CreatedAt
	}
	if activity.After(r.LatestActivity) {
		r.LatestActivity = activity
	}
}

// Only recognized outer transport is stripped. Indented code fences in a plan
// value are payload, not wrapper boundaries.
func appendOutput(p *Project, c model.PRComment) (needs, valid bool) {
	lines := strings.Split(normalize(c.Body), "\n")
	start, end := -1, -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "<details><summary>Show Output</summary>" {
			j := i + 1
			for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
				j++
			}
			if j < len(lines) && strings.TrimRight(lines[j], " \t") == "```diff" {
				start = j + 1
				break
			}
		}
	}
	if start >= 0 {
		for i := len(lines) - 1; i > start; i-- {
			if strings.TrimSpace(lines[i]) != "</details>" {
				continue
			}
			j := i - 1
			for j >= start && strings.TrimSpace(lines[j]) == "" {
				j--
			}
			if j >= start && strings.TrimRight(lines[j], " \t") == "```" {
				end = j
				p.footer += "\n" + strings.Join(lines[i+1:], "\n")
				break
			}
		}
	}
	valid = start >= 0 && end >= start
	if !valid {
		start = 0
		end = len(lines)
		p.footer = ""
	}
	var output []string
	if len(p.LineSources) > 0 {
		output = strings.Split(p.Text, "\n")
	}
	for i := start; i < end; i++ {
		output = append(output, lines[i])
		p.LineSources = append(p.LineSources, SourceLine{CommentKey: c.Key(), Line: i})
	}
	p.Text = strings.Join(output, "\n")
	p.DetailAvailable = true
	p.supported = valid
	for _, line := range strings.Split(p.footer, "\n") {
		if strings.TrimSpace(line) == continuationWarning {
			needs = true
		}
	}
	// The current fragment, not a previous fragment's footer, controls the chain.
	if valid {
		needs = false
		for _, line := range lines[end+1:] {
			if strings.TrimSpace(line) == continuationWarning {
				needs = true
			}
		}
	}
	return needs, valid
}
