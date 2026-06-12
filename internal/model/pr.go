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
	UpdatedAt      time.Time
	Diff           string
	Commits        []PRCommit
	Comments       []PRComment
	Triage         Triage
	Skipped        bool
}

func (pr PullRequest) Size() int {
	return pr.Additions + pr.Deletions
}
