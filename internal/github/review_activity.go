package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/senilio/zappr/internal/model"
)

const reviewActivityQuery = `query($owner: String!, $repo: String!, $number: Int!, $endCursor: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      timelineItems(first: 100, after: $endCursor, itemTypes: [PULL_REQUEST_REVIEW, REVIEW_DISMISSED_EVENT]) {
        pageInfo { hasNextPage endCursor }
        nodes {
          __typename
          ... on PullRequestReview { id url author { login } body state submittedAt }
          ... on ReviewDismissedEvent { id url createdAt previousReviewState review { id } pullRequestCommit { id } }
        }
      }
    }
  }
}`

type reviewActivityPage struct {
	Nodes []struct {
		ghReview
		Type                string `json:"__typename"`
		CreatedAt           string `json:"createdAt"`
		PreviousReviewState string `json:"previousReviewState"`
		Review              struct{ ID string }
		PullRequestCommit   *struct{ ID string }
	}
	PageInfo struct {
		HasNextPage *bool
		EndCursor   string
	}
}

func (p *GHProvider) reviewActivity(ctx context.Context, repo string, number int) ([]model.PRReview, []model.PRReviewDismissal, error) {
	parts := strings.Split(repo, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return nil, nil, fmt.Errorf("review activity: invalid repository %q", repo)
	}
	// Keep the query on one argument line for CLI diagnostics.
	args := []string{"api", "graphql", "-f", "query=" + strings.Join(strings.Fields(reviewActivityQuery), " ")}
	if len(parts) == 3 {
		args = append(args, "--hostname", parts[0])
		parts = parts[1:]
	}
	args = append(args, "-f", "owner="+parts[0], "-f", "repo="+parts[1], "-F", "number="+strconv.Itoa(number))
	var reviews []model.PRReview
	var dismissals []model.PRReviewDismissal
	for cursor := ""; ; {
		pageArgs := args
		if cursor != "" {
			pageArgs = append(pageArgs, "-f", "endCursor="+cursor)
		}
		out, err := p.run(ctx, pageArgs...)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch review activity: %w", err)
		}
		var response struct {
			Data struct {
				Repository struct {
					PullRequest struct {
						TimelineItems *reviewActivityPage
					}
				}
			}
			Errors []struct{ Message string }
		}
		if err := json.Unmarshal(out, &response); err != nil {
			return nil, nil, fmt.Errorf("parse review activity: %w", err)
		}
		if len(response.Errors) != 0 {
			return nil, nil, fmt.Errorf("fetch review activity: %s", response.Errors[0].Message)
		}
		page := response.Data.Repository.PullRequest.TimelineItems
		if page == nil || page.Nodes == nil || page.PageInfo.HasNextPage == nil {
			return nil, nil, fmt.Errorf("review activity: incomplete GitHub timeline")
		}
		for _, node := range page.Nodes {
			switch node.Type {
			case "PullRequestReview":
				if node.State != "PENDING" {
					reviews = append(reviews, model.PRReview{ID: node.ID, URL: node.URL, Author: node.Author.Login, Body: node.Body, State: node.State, SubmittedAt: parseTime(node.SubmittedAt)})
				}
			case "ReviewDismissedEvent":
				dismissals = append(dismissals, model.PRReviewDismissal{
					ID: node.ID, URL: node.URL, ReviewID: node.Review.ID, PreviousState: node.PreviousReviewState,
					CreatedAt: parseTime(node.CreatedAt), Stale: node.PreviousReviewState == "APPROVED" && node.PullRequestCommit != nil,
				})
			}
		}
		if !*page.PageInfo.HasNextPage {
			return reviews, dismissals, nil
		}
		if page.PageInfo.EndCursor == "" || page.PageInfo.EndCursor == cursor {
			return nil, nil, fmt.Errorf("review activity: missing or repeated pagination cursor")
		}
		cursor = page.PageInfo.EndCursor
	}
}
