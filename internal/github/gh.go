package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/senilio/zappr/internal/model"
)

type GHProvider struct {
	Bin string
}

const prListLimit = 60

func NewGHProvider() *GHProvider {
	return &GHProvider{Bin: "gh"}
}

func (p *GHProvider) ListPRs(ctx context.Context, repo string) ([]model.PullRequest, bool, error) {
	// Commits and comments are omitted: their nested connections dominate the
	// GraphQL cost of a list query. GetPR fetches them for the selected PR.
	args := []string{"pr", "list", "--repo", repo, "--state", "open", "--search", "sort:updated-desc", "--limit", strconv.Itoa(prListLimit), "--json", "number,title,author,url,labels,additions,deletions,changedFiles,isDraft,autoMergeRequest,state,headRefOid,statusCheckRollup,reviewDecision,mergeStateStatus,files,updatedAt"}
	out, err := p.run(ctx, args...)
	if err != nil {
		return nil, false, err
	}

	var raw []ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, false, fmt.Errorf("parse gh pr list: %w", err)
	}

	prs := make([]model.PullRequest, 0, len(raw))
	for _, item := range raw {
		prs = append(prs, item.toModel(repo))
	}
	return prs, len(raw) < prListLimit, nil
}

func (p *GHProvider) GetPR(ctx context.Context, repo string, number int) (model.PullRequest, error) {
	out, err := p.run(ctx, "pr", "view", strconv.Itoa(number), "--repo", repo, "--json", "number,title,author,url,labels,additions,deletions,changedFiles,isDraft,autoMergeRequest,state,headRefOid,statusCheckRollup,reviewDecision,files,commits,comments,reviews,latestReviews,reviewRequests,mergeStateStatus,updatedAt")
	if err != nil {
		return model.PullRequest{}, err
	}
	var raw ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return model.PullRequest{}, fmt.Errorf("parse gh pr view: %w", err)
	}
	pr := raw.toModel(repo)
	for _, review := range raw.Reviews {
		if strings.TrimSpace(review.Body) == "" || review.State == "PENDING" {
			continue
		}
		pr.Comments = append(pr.Comments, model.PRComment{
			Author: review.Author.Login, Body: review.Body,
			CreatedAt: parseTime(review.SubmittedAt), Kind: "review",
		})
	}
	out, err = p.run(ctx, "api", "repos/"+repo+"/pulls/"+strconv.Itoa(number)+"/comments?per_page=100", "--paginate", "--slurp")
	if err != nil {
		return model.PullRequest{}, err
	}
	var pages [][]ghReviewComment
	if err := json.Unmarshal(out, &pages); err != nil {
		return model.PullRequest{}, fmt.Errorf("parse gh review comments: %w", err)
	}
	for _, page := range pages {
		for _, comment := range page {
			pr.Comments = append(pr.Comments, model.PRComment{
				Author: comment.User.Login, Body: comment.Body,
				CreatedAt: parseTime(comment.CreatedAt), Kind: "inline",
				Path: comment.Path, Line: comment.Line,
				OriginalLine: comment.OriginalLine, Side: comment.Side,
			})
		}
	}
	pr.Reviewers = reviewersFrom(raw.LatestReviews, raw.ReviewRequests)
	pr.ReviewersLoaded = true
	pr.DetailsLoaded = true
	pr.ReviewCommentsLoaded = true
	return pr, nil
}

func (p *GHProvider) GetDiff(ctx context.Context, repo string, number int) (string, error) {
	out, err := p.run(ctx, "pr", "diff", strconv.Itoa(number), "--repo", repo)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (p *GHProvider) GetCommitDiff(ctx context.Context, repo string, sha string) (string, error) {
	out, err := p.run(ctx, "api", "/repos/"+repo+"/commits/"+sha,
		"--header", "Accept: application/vnd.github.diff")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (p *GHProvider) GetFileContent(ctx context.Context, repo string, path string, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("file ref is required")
	}
	apiPath := "/repos/" + repo + "/contents/" + escapeContentPath(path) + "?ref=" + url.QueryEscape(ref)
	out, err := p.run(ctx, "api", apiPath, "--header", "Accept: application/vnd.github.raw")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func escapeContentPath(path string) string {
	parts := strings.Split(path, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func (p *GHProvider) Approve(ctx context.Context, repo string, number int, body string) error {
	args := []string{"pr", "review", strconv.Itoa(number), "--repo", repo, "--approve"}
	if strings.TrimSpace(body) != "" {
		args = append(args, "--body", body)
	}
	_, err := p.run(ctx, args...)
	return err
}

func (p *GHProvider) Comment(ctx context.Context, repo string, number int, body string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("comment body is required")
	}
	_, err := p.run(ctx, "pr", "comment", strconv.Itoa(number), "--repo", repo, "--body", body)
	return err
}

func (p *GHProvider) ToggleDraft(ctx context.Context, repo string, number int, isDraft bool) error {
	if isDraft {
		_, err := p.run(ctx, "pr", "ready", strconv.Itoa(number), "--repo", repo, "--undo")
		return err
	}
	_, err := p.run(ctx, "pr", "ready", strconv.Itoa(number), "--repo", repo)
	return err
}

func (p *GHProvider) ClosePR(ctx context.Context, repo string, number int, comment string) error {
	if strings.TrimSpace(comment) != "" {
		if err := p.Comment(ctx, repo, number, comment); err != nil {
			return err
		}
	}
	_, err := p.run(ctx, "pr", "close", strconv.Itoa(number), "--repo", repo)
	return err
}

func (p *GHProvider) UpdateBranch(ctx context.Context, repo string, number int) error {
	_, err := p.run(ctx, "pr", "update-branch", strconv.Itoa(number), "--repo", repo)
	return err
}

func (p *GHProvider) RequestChanges(ctx context.Context, repo string, number int, body string) error {
	args := []string{"pr", "review", strconv.Itoa(number), "--repo", repo, "--request-changes"}
	if strings.TrimSpace(body) != "" {
		args = append(args, "--body", body)
	} else {
		args = append(args, "--body", "Changes requested.")
	}
	_, err := p.run(ctx, args...)
	return err
}

func (p *GHProvider) MergePR(ctx context.Context, repo string, number int) error {
	_, err := p.run(ctx, "pr", "merge", strconv.Itoa(number), "--repo", repo, "--auto", "--squash", "--delete-branch")
	return err
}

func (p *GHProvider) ToggleAutoMerge(ctx context.Context, repo string, number int, enabled bool) error {
	if enabled {
		return p.MergePR(ctx, repo, number)
	}
	_, err := p.run(ctx, "pr", "merge", strconv.Itoa(number), "--repo", repo, "--disable-auto")
	return err
}

func (p *GHProvider) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, p.Bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}

type ghPR struct {
	Number            int               `json:"number"`
	Title             string            `json:"title"`
	Author            ghAuthor          `json:"author"`
	URL               string            `json:"url"`
	Labels            []ghLabel         `json:"labels"`
	Additions         int               `json:"additions"`
	Deletions         int               `json:"deletions"`
	ChangedFiles      int               `json:"changedFiles"`
	Draft             bool              `json:"isDraft"`
	AutoMergeRequest  *struct{}         `json:"autoMergeRequest"`
	State             string            `json:"state"`
	StatusCheckRollup json.RawMessage   `json:"statusCheckRollup"`
	ReviewDecision    string            `json:"reviewDecision"`
	MergeStateStatus  string            `json:"mergeStateStatus"`
	LatestReviews     []ghReview        `json:"latestReviews"`
	ReviewRequests    []ghReviewRequest `json:"reviewRequests"`
	HeadRefOID        string            `json:"headRefOid"`
	Files             []ghFile          `json:"files"`
	Commits           []ghCommit        `json:"commits"`
	Comments          []ghComment       `json:"comments"`
	Reviews           []ghReview        `json:"reviews"`
	UpdatedAt         string            `json:"updatedAt"`
}

type ghAuthor struct {
	Login string `json:"login"`
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghFile struct {
	Path       string `json:"path"`
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	ChangeType string `json:"changeType"`
}

type ghCommit struct {
	OID             string           `json:"oid"`
	MessageHeadline string           `json:"messageHeadline"`
	CommittedDate   string           `json:"committedDate"`
	AuthoredDate    string           `json:"authoredDate"`
	Authors         []ghCommitAuthor `json:"authors"`
}

type ghCommitAuthor struct {
	Name string         `json:"name"`
	User *ghCommitLogin `json:"user"`
}

type ghCommitLogin struct {
	Login string `json:"login"`
}

type ghComment struct {
	Author    ghAuthor `json:"author"`
	Body      string   `json:"body"`
	CreatedAt string   `json:"createdAt"`
}

type ghReview struct {
	Author      ghAuthor `json:"author"`
	Body        string   `json:"body"`
	SubmittedAt string   `json:"submittedAt"`
	State       string   `json:"state"`
}

type ghReviewRequest struct {
	Typename string `json:"__typename"`
	Login    string `json:"login"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
}

// reviewersFrom merges latest reviews with pending review requests. A
// re-requested reviewer shows as REQUESTED. Humans come first, then teams,
// then bots, each group keeping its order.
func reviewersFrom(reviews []ghReview, requests []ghReviewRequest) []model.Reviewer {
	var users, teams []model.Reviewer
	index := map[string]int{}
	for _, review := range reviews {
		login := strings.TrimSpace(review.Author.Login)
		if login == "" || review.State == "PENDING" {
			continue
		}
		if i, ok := index[login]; ok {
			users[i].State = review.State
			continue
		}
		index[login] = len(users)
		users = append(users, model.Reviewer{Login: login, State: review.State})
	}
	for _, request := range requests {
		if request.Typename == "Team" || (request.Login == "" && (request.Slug != "" || request.Name != "")) {
			slug := request.Slug
			if slug == "" {
				slug = request.Name
			}
			teams = append(teams, model.Reviewer{Login: slug, State: "REQUESTED"})
			continue
		}
		login := strings.TrimSpace(request.Login)
		if login == "" {
			continue
		}
		if i, ok := index[login]; ok {
			users[i].State = "REQUESTED"
			continue
		}
		index[login] = len(users)
		users = append(users, model.Reviewer{Login: login, State: "REQUESTED"})
	}
	out := make([]model.Reviewer, 0, len(users)+len(teams))
	var bots []model.Reviewer
	for _, r := range users {
		if model.IsBotLogin(r.Login) {
			r.Bot = true
			bots = append(bots, r)
			continue
		}
		out = append(out, r)
	}
	out = append(out, teams...)
	return append(out, bots...)
}

type ghReviewComment struct {
	User         ghAuthor `json:"user"`
	Body         string   `json:"body"`
	CreatedAt    string   `json:"created_at"`
	Path         string   `json:"path"`
	Line         int      `json:"line"`
	OriginalLine int      `json:"original_line"`
	Side         string   `json:"side"`
}

func (p ghPR) toModel(repo string) model.PullRequest {
	labels := make([]string, 0, len(p.Labels))
	for _, label := range p.Labels {
		labels = append(labels, label.Name)
	}
	files := make([]model.ChangedFile, 0, len(p.Files))
	for _, file := range p.Files {
		status := strings.ToLower(file.ChangeType)
		if status == "deleted" {
			status = "removed"
		}
		files = append(files, model.ChangedFile{Path: file.Path, Additions: file.Additions, Deletions: file.Deletions, Status: status})
	}
	comments := toModelComments(p.Comments)
	changedFiles := p.ChangedFiles
	if changedFiles == 0 {
		changedFiles = len(files)
	}
	return model.PullRequest{
		Repo:           repo,
		Number:         p.Number,
		Title:          p.Title,
		Author:         p.Author.Login,
		URL:            p.URL,
		Labels:         labels,
		Files:          files,
		Additions:      p.Additions,
		Deletions:      p.Deletions,
		ChangedFiles:   changedFiles,
		CIStatus:       statusFromRollup(p.StatusCheckRollup),
		Checks:         parseChecks(p.StatusCheckRollup),
		ReviewDecision: p.ReviewDecision,
		MergeState:     strings.ToUpper(p.MergeStateStatus),
		Draft:          p.Draft,
		AutoMerge:      p.AutoMergeRequest != nil,
		State:          p.State,
		HeadRefOID:     p.HeadRefOID,
		UpdatedAt:      parseTime(p.UpdatedAt),
		Commits:        toModelCommits(p.Commits),
		Comments:       comments,
	}
}

func (c ghCommit) toModel() model.PRCommit {
	author := "unknown"
	for _, item := range c.Authors {
		if item.User != nil && strings.TrimSpace(item.User.Login) != "" {
			author = item.User.Login
			break
		}
		if strings.TrimSpace(item.Name) != "" {
			author = item.Name
		}
	}
	date := parseTime(c.CommittedDate)
	if date.IsZero() {
		date = parseTime(c.AuthoredDate)
	}
	return model.PRCommit{OID: c.OID, Headline: c.MessageHeadline, Author: author, Date: date}
}

func (c ghComment) toModel() model.PRComment {
	return model.PRComment{Author: c.Author.Login, Body: c.Body, CreatedAt: parseTime(c.CreatedAt)}
}

func toModelCommits(commits []ghCommit) []model.PRCommit {
	out := make([]model.PRCommit, 0, len(commits))
	for _, commit := range commits {
		out = append(out, commit.toModel())
	}
	return out
}

func toModelComments(comments []ghComment) []model.PRComment {
	out := make([]model.PRComment, 0, len(comments))
	for _, comment := range comments {
		out = append(out, comment.toModel())
	}
	return out
}

func parseTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func statusFromRollup(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "unknown"
	}
	s := strings.ToLower(string(raw))
	switch {
	case strings.Contains(s, "failure") || strings.Contains(s, "failed") || strings.Contains(s, "error"):
		return "failing"
	case strings.Contains(s, "pending") || strings.Contains(s, "queued") || strings.Contains(s, "in_progress"):
		return "pending"
	case strings.Contains(s, "success") || strings.Contains(s, "completed"):
		return "passing"
	default:
		return "unknown"
	}
}

func parseChecks(raw json.RawMessage) []model.Check {
	checks := []model.Check{}
	if len(raw) == 0 || string(raw) == "null" {
		return checks
	}
	var items []struct {
		Typename     string `json:"__typename"`
		Name         string `json:"name"`
		WorkflowName string `json:"workflowName"`
		Context      string `json:"context"`
		State        string `json:"state"`
		Conclusion   string `json:"conclusion"`
		Status       string `json:"status"`
		DetailsURL   string `json:"detailsUrl"`
		TargetURL    string `json:"targetUrl"`
		StartedAt    string `json:"startedAt"`
		CompletedAt  string `json:"completedAt"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return checks
	}
	for _, item := range items {
		check := model.Check{
			WorkflowName: item.WorkflowName,
			StartedAt:    parseTime(item.StartedAt),
			CompletedAt:  parseTime(item.CompletedAt),
			URL:          item.DetailsURL,
		}
		if check.URL == "" {
			check.URL = item.TargetURL
		}
		if item.Typename == "StatusContext" {
			check.Name = item.Context
			check.Context = item.Context
			check.State = normalizeCheckState(item.State)
		} else {
			check.Name = item.Name
			state := item.Conclusion
			if strings.TrimSpace(state) == "" {
				state = item.Status
			}
			check.State = normalizeCheckState(state)
		}
		checks = append(checks, check)
	}
	return checks
}

func normalizeCheckState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "success", "completed", "neutral", "skipped":
		return "SUCCESS"
	case "pending", "queued", "in_progress", "waiting", "requested":
		return "PENDING"
	case "expected":
		return "EXPECTED"
	case "failure", "failed", "cancelled", "timed_out", "action_required", "startup_failure", "stale":
		return "FAILURE"
	case "error":
		return "ERROR"
	default:
		return ""
	}
}
