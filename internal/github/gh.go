package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/senilio/zappr/internal/model"
)

type GHProvider struct {
	Bin string
}

func NewGHProvider() *GHProvider {
	return &GHProvider{Bin: "gh"}
}

func (p *GHProvider) ListOpenPRs(ctx context.Context, repo string) ([]model.PullRequest, error) {
	out, err := p.run(ctx, "pr", "list", "--repo", repo, "--state", "open", "--json", "number,title,author,url,labels,additions,deletions,changedFiles,isDraft,statusCheckRollup")
	if err != nil {
		return nil, err
	}

	var raw []ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse gh pr list: %w", err)
	}

	prs := make([]model.PullRequest, 0, len(raw))
	for _, item := range raw {
		pr := item.toModel(repo)
		files, err := p.getFiles(ctx, repo, pr.Number)
		if err == nil {
			pr.Files = files
			if pr.ChangedFiles == 0 {
				pr.ChangedFiles = len(files)
			}
		}
		prs = append(prs, pr)
	}
	return prs, nil
}

func (p *GHProvider) GetPR(ctx context.Context, repo string, number int) (model.PullRequest, error) {
	out, err := p.run(ctx, "pr", "view", strconv.Itoa(number), "--repo", repo, "--json", "number,title,author,url,labels,additions,deletions,changedFiles,isDraft,statusCheckRollup,files")
	if err != nil {
		return model.PullRequest{}, err
	}
	var raw ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return model.PullRequest{}, fmt.Errorf("parse gh pr view: %w", err)
	}
	return raw.toModel(repo), nil
}

func (p *GHProvider) GetDiff(ctx context.Context, repo string, number int) (string, error) {
	out, err := p.run(ctx, "pr", "diff", strconv.Itoa(number), "--repo", repo)
	if err != nil {
		return "", err
	}
	return string(out), nil
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
	_, err := p.run(ctx, "pr", "review", strconv.Itoa(number), "--repo", repo, "--comment", "--body", body)
	return err
}

func (p *GHProvider) getFiles(ctx context.Context, repo string, number int) ([]model.ChangedFile, error) {
	out, err := p.run(ctx, "pr", "view", strconv.Itoa(number), "--repo", repo, "--json", "files")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Files []ghFile `json:"files"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse gh files: %w", err)
	}
	files := make([]model.ChangedFile, 0, len(raw.Files))
	for _, file := range raw.Files {
		files = append(files, model.ChangedFile{Path: file.Path, Additions: file.Additions, Deletions: file.Deletions, Status: file.Status})
	}
	return files, nil
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
	StatusCheckRollup json.RawMessage `json:"statusCheckRollup"`
	Files             []ghFile        `json:"files"`
}

type ghAuthor struct {
	Login string `json:"login"`
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghFile struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Status    string `json:"status"`
}

func (p ghPR) toModel(repo string) model.PullRequest {
	labels := make([]string, 0, len(p.Labels))
	for _, label := range p.Labels {
		labels = append(labels, label.Name)
	}
	files := make([]model.ChangedFile, 0, len(p.Files))
	for _, file := range p.Files {
		files = append(files, model.ChangedFile{Path: file.Path, Additions: file.Additions, Deletions: file.Deletions, Status: file.Status})
	}
	return model.PullRequest{
		Repo:         repo,
		Number:       p.Number,
		Title:        p.Title,
		Author:       p.Author.Login,
		URL:          p.URL,
		Labels:       labels,
		Files:        files,
		Additions:    p.Additions,
		Deletions:    p.Deletions,
		ChangedFiles: p.ChangedFiles,
		CIStatus:     statusFromRollup(p.StatusCheckRollup),
		Draft:        p.Draft,
	}
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
