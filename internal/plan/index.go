package plan

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/senilio/zappr/internal/model"
)

var resourceHeading = regexp.MustCompile(`^\s*#\s+(.+?)\s+(will be .+|must be replaced.*|has moved to .+)$`)
var terminalTotals = regexp.MustCompile(`^Plan: ([0-9]+) to add, ([0-9]+) to change, ([0-9]+) to destroy\.$`)
var aggregateTotals = regexp.MustCompile(`^([0-9]+) projects, ([0-9]+) with changes, ([0-9]+) with no changes, ([0-9]+) failed$`)
var heredocOpen = regexp.MustCompile(`^<<-?([A-Za-z_][A-Za-z0-9_]*)\s*$`)

// scanState tracks only literal boundaries needed to avoid interpreting values
// as report syntax. It is intentionally not a Terraform expression parser.
type scanState struct {
	heredoc        string
	quoted, escape bool
}

func diffText(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "~") {
		s = strings.TrimSpace(s[1:])
	}
	return s
}
func (s *scanState) payload(line string) bool {
	if s.heredoc != "" {
		// Inside a heredoc, leading +/- characters are literal value text,
		// not diff markers. Only the supported plain delimiter closes it.
		if strings.TrimSpace(line) == s.heredoc {
			s.heredoc = ""
		}
		return true
	}
	wasQuoted := s.quoted
	for i := 0; i < len(line); i++ {
		b := line[i]
		if s.escape {
			s.escape = false
			continue
		}
		if s.quoted && b == '\\' {
			s.escape = true
			continue
		}
		if b == '"' {
			s.quoted = !s.quoted
			continue
		}
		if !s.quoted && b == '<' {
			if m := heredocOpen.FindStringSubmatch(line[i:]); m != nil {
				s.heredoc = m[1]
				break
			}
		}
	}
	return wasQuoted
}
func (s scanState) closed() bool { return s.heredoc == "" && !s.quoted }
func number(s string) int        { n, _ := strconv.Atoi(s); return n }

func indexReport(r *Report) {
	if r.aggregateText != "" {
		indexAggregate(r)
	}
	for i := range r.Projects {
		indexProject(&r.Projects[i], r.SourceComplete)
		indexFolds(&r.Projects[i], r.SourceComplete)
	}
	if r.Aggregate != nil {
		a := r.Aggregate
		valid := len(r.Issues) == 0 && r.SourceComplete
		changed, failed, unchanged, missing := 0, 0, 0, 0
		for _, p := range r.Projects {
			if len(p.Issues) > 0 {
				valid = false
			}
			if a.NoChanges == a.Projects && p.Outcome == OutcomeChanges {
				valid = false
				r.Issues = append(r.Issues, "Aggregate conflicts with project output")
			}
			switch p.Outcome {
			case OutcomeChanges:
				changed++
			case OutcomeFailed:
				failed++
			case OutcomeNoChanges:
				unchanged++
			default:
				if !p.DetailAvailable {
					missing++
				}
			}
		}
		if changed > a.Changed || failed > a.Failed || unchanged > a.NoChanges {
			valid = false
			r.Issues = append(r.Issues, "Aggregate conflicts with project outcomes")
		}
		if !valid {
			r.Outcome = OutcomeUnknown
			return
		}
		switch {
		case a.Failed > 0:
			r.Outcome = OutcomeFailed
		case a.Changed > 0:
			r.Outcome = OutcomeChanges
		case a.NoChanges == a.Projects:
			r.Outcome = OutcomeNoChanges
		}
		if changed == a.Changed && failed == a.Failed && unchanged+missing == a.NoChanges {
			for i := range r.Projects {
				p := &r.Projects[i]
				if !p.DetailAvailable {
					p.Outcome = OutcomeNoChanges
					p.Evidence = "aggregate"
				}
			}
		}
		return
	}
	r.Outcome = OutcomeUnknown
	consistent := r.SourceComplete && len(r.Issues) == 0
	allNoChanges := len(r.Projects) > 0
	for _, p := range r.Projects {
		if p.Outcome == OutcomeFailed {
			r.Outcome = OutcomeFailed
			return
		}
		if p.Outcome != OutcomeNoChanges {
			allNoChanges = false
		}
		if p.Outcome == OutcomeChanges && consistent {
			r.Outcome = OutcomeChanges
		}
	}
	if allNoChanges && consistent {
		r.Outcome = OutcomeNoChanges
	}
}

func indexProject(p *Project, complete bool) {
	if !p.supported {
		return
	}
	lines := strings.Split(p.Text, "\n")
	var state scanState
	noChanges, outputChanges, failed, unsupported, conflict := false, false, false, false, false
	lastEnd := len(lines)
	seen := map[string]bool{}
	acceptTotals := func(line string) {
		m := terminalTotals.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			if strings.HasPrefix(strings.TrimSpace(line), "Plan:") {
				unsupported = true
			}
			return
		}
		t := Totals{number(m[1]), number(m[2]), number(m[3])}
		if p.Totals != nil && *p.Totals != t {
			conflict = true
		} else {
			p.Totals = &t
		}
	}
	for i, line := range lines {
		if state.payload(line) {
			continue
		}
		trim := strings.TrimSpace(line)
		if m := resourceHeading.FindStringSubmatch(line); m != nil {
			if seen[m[1]] {
				unsupported = true
				continue
			}
			seen[m[1]] = true
			if n := len(p.Resources); n > 0 {
				p.Resources[n-1].End = i
			}
			action := nativeAction(m[2])
			if action == "unknown" {
				unsupported = true
			}
			p.Resources = append(p.Resources, Resource{ID: p.ID + "/resource:" + m[1], Address: m[1], Action: action, Start: i, End: len(lines)})
		}
		if terminalTotals.MatchString(trim) || strings.HasPrefix(trim, "No changes.") || trim == "Changes to Outputs:" {
			if i < lastEnd {
				lastEnd = i
			}
		}
		if strings.HasPrefix(trim, "Plan:") {
			acceptTotals(trim)
		}
		if trim == "No changes. Your infrastructure matches the configuration." || trim == "No changes. Infrastructure is up-to-date." {
			noChanges = true
		}
		if trim == "Changes to Outputs:" {
			outputChanges = true
		}
		if strings.HasPrefix(trim, "Error:") || strings.HasPrefix(trim, "Planning failed.") {
			failed = true
		}
	}
	// Footer command fences are transport/help, not additional plan output.
	inFence := false
	for _, line := range strings.Split(p.footer, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			acceptTotals(trim)
		}
	}
	if len(p.Resources) > 0 {
		p.Resources[len(p.Resources)-1].End = lastEnd
	}
	for i := range p.Resources {
		resource := &p.Resources[i]
		if resource.End < resource.Start {
			resource.End = resource.Start + 1
		}
		resource.Sources = sourceSpans(p.LineSources, resource.Start, resource.End)
	}
	if !state.closed() {
		p.Issues = append(p.Issues, "Unterminated or unsupported literal value")
		unsupported = true
	}
	if conflict {
		p.Issues = append(p.Issues, "Conflicting terminal plan totals")
	}
	if unsupported {
		p.Issues = append(p.Issues, "Unsupported or inconsistent plan structure")
	}
	p.Outcome = OutcomeUnknown
	if failed {
		p.Outcome = OutcomeFailed
		p.Evidence = "output"
		return
	}
	if !complete || unsupported || conflict {
		return
	}
	changed := outputChanges
	if p.Totals != nil {
		changed = changed || p.Totals.Add+p.Totals.Change+p.Totals.Destroy > 0
	}
	for _, r := range p.Resources {
		if r.Action != "read" && r.Action != "unknown" {
			changed = true
		}
	}
	if noChanges && changed {
		p.Issues = append(p.Issues, "No-change statement conflicts with planned effects")
		return
	}
	if changed {
		p.Outcome = OutcomeChanges
		p.Evidence = "output"
	} else if noChanges && len(p.Resources) == 0 {
		p.Outcome = OutcomeNoChanges
		p.Evidence = "output"
	}
}

func nativeAction(s string) string {
	for _, p := range []struct{ prefix, action string }{{"will be created", "create"}, {"will be updated in-place", "update"}, {"will be destroyed", "delete"}, {"must be replaced", "replace"}, {"will be replaced", "replace"}, {"will be read during apply", "read"}, {"will be imported", "import"}} {
		if strings.HasPrefix(s, p.prefix) {
			return p.action
		}
	}
	return "unknown"
}
func sourceSpans(lines []SourceLine, start, end int) []Span {
	var spans []Span
	for i := start; i < end && i < len(lines); i++ {
		s := lines[i]
		n := len(spans)
		if n > 0 && spans[n-1].CommentKey == s.CommentKey && spans[n-1].End == s.Line {
			spans[n-1].End++
		} else {
			spans = append(spans, Span{s.CommentKey, s.Line, s.Line + 1})
		}
	}
	return spans
}

func indexAggregate(r *Report) {
	lines := strings.Split(r.aggregateText, "\n")
	var sections []int
	inOutput := false
	for i, line := range lines {
		if line == "```diff" {
			inOutput = true
			continue
		}
		if inOutput {
			if line == "```" && i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "</details>" {
				inOutput = false
			}
			continue
		}
		if projectSection(line) != nil {
			sections = append(sections, i)
		}
		if m := aggregateTotals.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			a := Aggregate{number(m[1]), number(m[2]), number(m[3]), number(m[4])}
			if r.Aggregate != nil && *r.Aggregate != a {
				r.Issues = append(r.Issues, "Conflicting aggregate totals")
			}
			r.Aggregate = &a
		}
	}
	for j, start := range sections {
		end := len(lines)
		if j+1 < len(sections) {
			end = sections[j+1]
		}
		m := projectSection(lines[start])
		p := newProject(r.ID, m[1], m[2], m[3])
		found := -1
		for i, existing := range r.Projects {
			if existing.ID == p.ID {
				found = i
				break
			}
		}
		if found < 0 {
			r.Issues = append(r.Issues, "Project output is not in the advertised roster")
			continue
		}
		if r.Projects[found].DetailAvailable {
			r.Issues = append(r.Issues, "Duplicate project output section")
			continue
		}
		c := model.PRComment{Kind: "", Body: strings.Join(lines[start:end], "\n")}
		_, valid := appendOutput(&p, c)
		if !valid {
			p.Issues = append(p.Issues, "Unsupported project output wrapper")
		}
		for i := range p.LineSources {
			p.LineSources[i].CommentKey = r.Sources[0].CommentKey
			p.LineSources[i].Line += start
		}
		r.Projects[found] = p
		if valid {
			r.Coverage = CoverageProjects
		}
	}
	expected := aggregateHeader.FindStringSubmatch(firstLine(r.aggregateText))
	seen := map[string]bool{}
	for _, p := range r.Projects {
		if seen[p.ID] {
			r.Issues = append(r.Issues, "Duplicate project in roster")
		}
		seen[p.ID] = true
	}
	if r.Aggregate == nil {
		r.Issues = append(r.Issues, "Aggregate result not provided")
		return
	}
	a := r.Aggregate
	if len(expected) < 2 || number(expected[1]) != a.Projects || len(r.Projects) != a.Projects || a.Changed+a.NoChanges+a.Failed != a.Projects {
		r.Issues = append(r.Issues, fmt.Sprintf("Aggregate/roster count mismatch (%d listed)", len(r.Projects)))
	}
}
