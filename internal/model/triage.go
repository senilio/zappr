package model

type Triage struct {
	Risk            Risk     `json:"risk"`
	Confidence      float64  `json:"confidence"`
	Summary         string   `json:"summary"`
	Reasons         []string `json:"reasons"`
	Concerns        []string `json:"concerns"`
	HighRiskFiles   []string `json:"high_risk_files"`
	ReviewTargets   []string `json:"review_targets"`
	SuggestedAction string   `json:"suggested_action"`
	ReviewComment   string   `json:"review_comment"`
	AI              *AITriage
}

type AITriage struct {
	Summary        string   `json:"summary"`
	RiskNotes      []string `json:"risk_notes"`
	SuggestedFocus []string `json:"suggested_focus"`
	CommentDrafts  []string `json:"comment_drafts"`
	Error          string   `json:"error,omitempty"`
}
