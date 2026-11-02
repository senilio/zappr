// Package plan indexes supported plaintext reports without interpreting or
// executing infrastructure configuration. All identities are local source keys,
// not proof of an Atlantis run or a commit association.
package plan

import "time"

type Outcome string

const (
	OutcomeUnknown   Outcome = "unknown"
	OutcomeChanges   Outcome = "changes"
	OutcomeNoChanges Outcome = "no-changes"
	OutcomeFailed    Outcome = "failed"
)

type Coverage string

const (
	CoverageUnknown   Coverage = "unknown"
	CoverageAggregate Coverage = "aggregate-only"
	CoverageProjects  Coverage = "projects"
)

type Document struct{ Reports []Report }
type Report struct {
	ID, Revision   string
	Sources        []Source
	Projects       []Project
	Aggregate      *Aggregate
	SourceComplete bool
	Coverage       Coverage
	Outcome        Outcome
	CommitSHA      string
	CommitVerified bool
	Issues         []string
	LatestActivity time.Time
	aggregateText  string
}
type Source struct {
	CommentKey, URL      string
	CreatedAt, UpdatedAt time.Time
}
type Span struct {
	CommentKey string
	Start, End int
}
type SourceLine struct {
	CommentKey string
	Line       int
}
type Project struct {
	ID, Name, Dir, Workspace string
	Outcome                  Outcome
	Evidence                 string
	Totals                   *Totals
	DetailAvailable          bool
	Text                     string
	LineSources              []SourceLine
	Resources                []Resource
	Folds                    []Fold
	Issues                   []string
	footer                   string
	supported                bool
}
type Totals struct{ Add, Change, Destroy int }
type Aggregate struct{ Projects, Changed, NoChanges, Failed int }
type Resource struct {
	ID, Address, Action string
	Start, End          int
	Sources             []Span
}
type NodeKind string

const (
	NodeSummary  NodeKind = "summary"
	NodeProject  NodeKind = "project"
	NodeResource NodeKind = "resource"
	NodeSource   NodeKind = "source"
	NodeReport   NodeKind = "report"
)

type Node struct {
	ID                                          string
	Kind                                        NodeKind
	ReportID, ProjectID, ResourceID, CommentKey string
	Depth                                       int
}
type Fold struct {
	ID, ResourceID, Label string
	Start, End            int
}
type Rendered struct {
	Text        string
	SourceLines []int
}
