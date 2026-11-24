package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

const (
	prListLimit  = 60
	prBaseFields = "number,title,author,url,body,labels,additions,deletions,changedFiles,isDraft,autoMergeRequest,state,headRefOid,statusCheckRollup,reviewDecision"
	prFields     = prBaseFields + ",mergeStateStatus,files,updatedAt"
)

func NewGHProvider() *GHProvider {
	return &GHProvider{Bin: "gh"}
}

func (p *GHProvider) ListPRs(ctx context.Context, repo string) ([]model.PullRequest, bool, error) {
	// Commits and comments are omitted: their nested connections dominate the
	// GraphQL cost of a list query. GetPR fetches them for the selected PR.
	args := []string{"pr", "list", "--repo", repo, "--state", "open", "--search", "sort:updated-desc", "--limit", strconv.Itoa(prListLimit), "--json", prFields}
	out, err := p.run(ctx, args...)
	if err != nil {
		return nil, false, err
	}

	var raw []ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, false, fmt.Errorf("parse gh pr list: %w", err)
	}
	if raw == nil {
		return nil, false, fmt.Errorf("parse gh pr list: missing PR list")
	}
	prs := make([]model.PullRequest, 0, len(raw))
	for _, item := range raw {
		prs = append(prs, item.toModel(repo))
	}
	return prs, len(raw) < prListLimit, nil
}

func (p *GHProvider) viewPR(ctx context.Context, repo string, number int, fields string) (ghPR, error) {
	out, err := p.run(ctx, "pr", "view", strconv.Itoa(number), "--repo", repo, "--json", fields)
	if err != nil {
		return ghPR{}, err
	}
	var raw ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return ghPR{}, fmt.Errorf("parse gh pr view: %w", err)
	}
	return raw, nil
}

func (p *GHProvider) CheckPRAction(ctx context.Context, repo string, number int, guarded bool) (PRActionCheck, error) {
	var comments []model.PRComment
	var evidenceErr error
	if guarded {
		comments, evidenceErr = p.conversationComments(ctx, repo, number)
	}
	// Read the head and open state last, even if the conversation was
	// unavailable. The conversation failure is not proof about the target.
	raw, err := p.viewPR(ctx, repo, number, prFields)
	if err != nil {
		return PRActionCheck{}, err
	}
	// gh resolves the number in --repo. Cross-check the returned identity
	// against the requested target, including the URL when present.
	if raw.Number != number || (raw.URL != "" && !matchesPRURL(raw.URL, repo, number)) {
		return PRActionCheck{}, fmt.Errorf("unexpected pull request: wanted %s#%d", repo, number)
	}
	switch raw.State {
	case "CLOSED", "MERGED":
		return PRActionCheck{PR: raw.toModel(repo)}, nil
	case "OPEN":
		if strings.TrimSpace(raw.HeadRefOID) == "" {
			return PRActionCheck{}, fmt.Errorf("missing PR head ref")
		}
	default:
		return PRActionCheck{}, fmt.Errorf("unknown pull request state %q", raw.State)
	}
	checks, known := decodeChecks(raw.StatusCheckRollup)
	pr := raw.toModelWithChecks(repo, checks)
	if guarded {
		pr.Comments, pr.ConversationCommentsLoaded = comments, evidenceErr == nil
		if !known {
			evidenceErr = errors.Join(evidenceErr, errors.New("missing or invalid check rollup"))
		}
	}
	return PRActionCheck{PR: pr, ChecksKnown: known, EvidenceErr: evidenceErr}, nil
}

func matchesPRURL(raw, repo string, number int) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.Count(repo, "/") == 2 {
		host, rest, _ := strings.Cut(repo, "/")
		if !strings.EqualFold(host, u.Host) {
			return false
		}
		repo = rest
	}
	return strings.EqualFold(u.Path, "/"+repo+"/pull/"+strconv.Itoa(number))
}

func (p *GHProvider) GetPR(ctx context.Context, repo string, number int) (model.PullRequest, error) {
	raw, err := p.viewPR(ctx, repo, number, prBaseFields+",files,commits,reviews,latestReviews,reviewRequests,mergeStateStatus,updatedAt")
	if err != nil {
		return model.PullRequest{}, err
	}
	pr := raw.toModel(repo)
	if pr.State == "CLOSED" || pr.State == "MERGED" {
		return pr, nil
	}
	pr.Comments, err = p.conversationComments(ctx, repo, number)
	if err != nil {
		return model.PullRequest{}, err
	}
	for _, review := range raw.Reviews {
		if strings.TrimSpace(review.Body) == "" || review.State == "PENDING" {
			continue
		}
		pr.Comments = append(pr.Comments, model.PRComment{
			ID: review.ID, URL: review.URL,
			Author: review.Author.Login, Body: review.Body,
			CreatedAt: parseTime(review.SubmittedAt), Kind: "review",
		})
	}
	out, err := p.run(ctx, "api", "repos/"+repo+"/pulls/"+strconv.Itoa(number)+"/comments?per_page=100", "--paginate", "--slurp")
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
				ID: nativeCommentID(comment.ID), URL: comment.URL, UpdatedAt: parseTime(comment.UpdatedAt),
				Author: comment.User.Login, Body: comment.Body,
				CreatedAt: parseTime(comment.CreatedAt), Kind: "inline",
				Path: comment.Path, Line: comment.Line,
				OriginalLine: comment.OriginalLine, Side: comment.Side,
			})
		}
	}
	pr.Reviews, pr.ReviewDismissals, err = p.reviewActivity(ctx, repo, number)
	if err != nil {
		return model.PullRequest{}, err
	}
	pr.ReviewActivityLoaded = true
	pr.Reviewers = reviewersFrom(raw.LatestReviews, raw.ReviewRequests)
	pr.ReviewersLoaded = true
	pr.DetailsLoaded = true
	pr.ReviewCommentsLoaded = true
	pr.ConversationCommentsLoaded = true
	return pr, nil
}

type ghConversationComment struct {
	ID        int64    `json:"id"`
	URL       string   `json:"html_url"`
	User      ghAuthor `json:"user"`
	Body      string   `json:"body"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

func nativeCommentID(id int64) string {
	if id <= 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}

func (p *GHProvider) conversationComments(ctx context.Context, repo string, number int) ([]model.PRComment, error) {
	out, err := p.run(ctx, "api", "repos/"+repo+"/issues/"+strconv.Itoa(number)+"/comments?per_page=100", "--paginate", "--slurp")
	if err != nil {
		return nil, fmt.Errorf("fetch conversation comments: %w", err)
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(out, &pages); err != nil {
		return nil, fmt.Errorf("parse gh conversation comments: %w", err)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("parse gh conversation comments: missing pages")
	}
	var comments []model.PRComment
	var badPage, badComment bool
	for _, page := range pages {
		var entries []json.RawMessage
		if err := json.Unmarshal(page, &entries); err != nil || entries == nil {
			badPage = true
			continue
		}
		for _, entry := range entries {
			var c ghConversationComment
			if err := json.Unmarshal(entry, &c); err != nil {
				badComment = true
				continue
			}
			created, updated := parseTime(c.CreatedAt), parseTime(c.UpdatedAt)
			u, err := url.Parse(c.URL)
			if c.ID <= 0 || created.IsZero() || updated.IsZero() || err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
				badComment = true
				continue
			}
			comments = append(comments, model.PRComment{ID: nativeCommentID(c.ID), URL: c.URL, Author: c.User.Login, Body: c.Body, CreatedAt: created, UpdatedAt: updated})
		}
	}
	// Partial evidence can still establish a guard reason, but it cannot
	// establish that a reason is absent.
	var evidenceErr error
	if badPage {
		evidenceErr = errors.New("parse gh conversation comments: missing or invalid page")
	}
	if badComment {
		evidenceErr = errors.Join(evidenceErr, errors.New("parse gh conversation comments: missing or invalid native provenance"))
	}
	return comments, evidenceErr
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
		// Report the operation and the failure, not write arguments such as
		// the full comment body or a long --json field list.
		operation := args[0]
		if operation == "pr" && len(args) > 1 {
			operation += " " + args[1]
		}
		return nil, fmt.Errorf("gh %s: %s", operation, msg)
	}
	return out, nil
}

type ghPR struct {
	Number            int               `json:"number"`
	Title             string            `json:"title"`
	Author            ghAuthor          `json:"author"`
	URL               string            `json:"url"`
	Body              string            `json:"body"`
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
	ID          string   `json:"id"`
	URL         string   `json:"url"`
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

// reviewersFrom merges latest reviews with pending review requests without
// replacing submitted review states. Humans come first, then teams,
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
		if _, ok := index[login]; ok {
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
	ID           int64    `json:"id"`
	URL          string   `json:"html_url"`
	UpdatedAt    string   `json:"updated_at"`
	User         ghAuthor `json:"user"`
	Body         string   `json:"body"`
	CreatedAt    string   `json:"created_at"`
	Path         string   `json:"path"`
	Line         int      `json:"line"`
	OriginalLine int      `json:"original_line"`
	Side         string   `json:"side"`
}

func (p ghPR) toModel(repo string) model.PullRequest {
	return p.toModelWithChecks(repo, parseChecks(p.StatusCheckRollup))
}

func (p ghPR) toModelWithChecks(repo string, checks []model.Check) model.PullRequest {
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
		Body:           p.Body,
		Labels:         labels,
		Files:          files,
		Additions:      p.Additions,
		Deletions:      p.Deletions,
		ChangedFiles:   changedFiles,
		CIStatus:       statusFromRollup(p.StatusCheckRollup),
		Checks:         checks,
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
	checks, _ := decodeChecks(raw)
	return checks
}

// Keep readable entries even when another entry makes the rollup incomplete.
// The guard can still show affirmative evidence, such as a pending check.
func decodeChecks(raw json.RawMessage) ([]model.Check, bool) {
	checks := []model.Check{}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		return checks, false
	}
	known := true
	for _, data := range items {
		var item struct {
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
		if err := json.Unmarshal(data, &item); err != nil || (item.Typename != "CheckRun" && item.Typename != "StatusContext") {
			known = false
			continue
		}
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
		known = known && strings.TrimSpace(check.Name) != "" && check.State != ""
		checks = append(checks, check)
	}
	return checks, known
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
