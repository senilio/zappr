package model

import "time"

type ChangedFile struct {
	Path      string
	Additions int
	Deletions int
	Status    string
}

type PRCommit struct {
	OID      string
	Headline string
	Author   string
	Date     time.Time
}

type PRComment struct {
	Author    string
	Body      string
	CreatedAt time.Time
	// Kind is empty for conversation comments, "review" for review bodies,
	// and "inline" for comments attached to a file or diff line.
	Kind         string `json:",omitempty"`
	Path         string `json:",omitempty"`
	Line         int    `json:",omitempty"`
	OriginalLine int    `json:",omitempty"`
	Side         string `json:",omitempty"`
}

// Check is one entry of a PR's status check rollup (a CheckRun or a
// StatusContext). State is normalized to SUCCESS, PENDING, FAILURE, ERROR,
// EXPECTED, or "" when unrecognized.
type Check struct {
	Name         string
	Context      string
	WorkflowName string
	State        string
	StartedAt    time.Time
	CompletedAt  time.Time
	URL          string
}

type PullRequest struct {
	Repo           string
	Number         int
	Title          string
	Author         string
	URL            string
	Labels         []string
	Files          []ChangedFile
	Additions      int
	Deletions      int
	ChangedFiles   int
	CIStatus       string
	Checks         []Check `json:"checks,omitempty"`
	ReviewDecision string
	Draft          bool
	AutoMerge      bool
	State          string
	HeadRefOID     string
	UpdatedAt      time.Time
	Commits        []PRCommit
	Comments       []PRComment
	// DetailsLoaded reports whether Commits and Comments were
	// fetched; inbox list refreshes do not include them.
	DetailsLoaded bool `json:"details_loaded,omitempty"`
	// ReviewCommentsLoaded distinguishes complete details from older cached
	// entries that fetched only conversation comments.
	ReviewCommentsLoaded bool `json:"review_comments_loaded,omitempty"`
	Skipped              bool
}
