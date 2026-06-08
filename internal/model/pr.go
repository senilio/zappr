package model

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

type PullRequest struct {
	Repo         string
	Number       int
	Title        string
	Author       string
	URL          string
	Labels       []string
	Files        []ChangedFile
	Additions    int
	Deletions    int
	ChangedFiles int
	CIStatus     string
	Draft        bool
	Diff         string
	Triage       Triage
	Skipped      bool
}

func (pr PullRequest) Size() int {
	return pr.Additions + pr.Deletions
}
