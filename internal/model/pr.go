package model

import "time"

type Risk string

const (
	RiskSafe   Risk = "safe"
	RiskReview Risk = "review"
	RiskBlock  Risk = "block"
)

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
	ReviewDecision string
	Draft          bool
	AutoMerge      bool
	State          string
	HeadRefOID     string
	UpdatedAt      time.Time
	Diff           string
	Commits        []PRCommit
	Comments       []PRComment
	// DetailsLoaded reports whether Commits and Comments were
	// fetched; inbox list refreshes do not include them.
	DetailsLoaded bool `json:"details_loaded,omitempty"`
	Triage        Triage
	Skipped       bool
}

func (pr PullRequest) Size() int {
	return pr.Additions + pr.Deletions
}
