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
	args := []string{"pr", "list", "--repo", repo, "--state", "open", "--search", "sort:updated-desc", "--limit", strconv.Itoa(prListLimit), "--json", "number,title,author,url,labels,additions,deletions,changedFiles,isDraft,state,headRefOid,statusCheckRollup,reviewDecision,files,updatedAt"}
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
	out, err := p.run(ctx, "pr", "view", strconv.Itoa(number), "--repo", repo, "--json", "number,title,author,url,labels,additions,deletions,changedFiles,isDraft,state,headRefOid,statusCheckRollup,reviewDecision,files,commits,comments,updatedAt")
	if err != nil {
		return model.PullRequest{}, err
	}
	var raw ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return model.PullRequest{}, fmt.Errorf("parse gh pr view: %w", err)
	}
	pr := raw.toModel(repo)
	pr.DetailsLoaded = true
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
		_, err := p.run(ctx, "pr", "edit", strconv.Itoa(number), "--repo", repo, "--draft")
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
	Number            int             `json:"number"`
	Title             string          `json:"title"`
	Author            ghAuthor        `json:"author"`
	URL               string          `json:"url"`
	Labels            []ghLabel       `json:"labels"`
	Additions         int             `json:"additions"`
	Deletions         int             `json:"deletions"`
	ChangedFiles      int             `json:"changedFiles"`
	Draft             bool            `json:"isDraft"`
	State             string          `json:"state"`
	StatusCheckRollup json.RawMessage `json:"statusCheckRollup"`
	ReviewDecision    string          `json:"reviewDecision"`
	HeadRefOID        string          `json:"headRefOid"`
	Files             []ghFile        `json:"files"`
	Commits           []ghCommit      `json:"commits"`
	Comments          []ghComment     `json:"comments"`
	UpdatedAt         string          `json:"updatedAt"`
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
		ReviewDecision: p.ReviewDecision,
		Draft:          p.Draft,
		State:          p.State,
		HeadRefOID:     p.HeadRefOID,
		UpdatedAt:      parseTime(p.UpdatedAt),
		Commits:        toModelCommits(p.Commits),
		Comments:       comments,
		AtlantisPlan:   model.LatestAtlantisPlan(comments),
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
