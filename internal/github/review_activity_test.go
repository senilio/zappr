package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const emptyReviewTimeline = `{"data":{"repository":{"pullRequest":{"timelineItems":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}}`

func emptyReviewTimelineScript() string {
	return "if [ \"$1 $2\" = 'api graphql' ]; then printf '%s' " + testShellQuote(emptyReviewTimeline) + "; exit; fi\n"
}

func TestGetPRReviewActivity(t *testing.T) {
	page := `{"data":{"repository":{"pullRequest":{"timelineItems":{"nodes":[
		{"__typename":"PullRequestReview","id":"active","author":{"login":"alice"},"state":"APPROVED","submittedAt":"2026-10-20T12:00:00Z","url":"https://github.com/o/r/pull/7#pullrequestreview-1"},
		{"__typename":"PullRequestReview","id":"pending","state":"PENDING"},
		{"__typename":"PullRequestReview","id":"review-1","state":"DISMISSED"},
		{"__typename":"PullRequestReview","id":"review-2","state":"DISMISSED"},
		{"__typename":"PullRequestReview","id":"review-3","state":"DISMISSED"},
		{"__typename":"ReviewDismissedEvent","id":"event-1","review":{"id":"review-1"},"previousReviewState":"APPROVED","pullRequestCommit":{"id":"new"},"createdAt":"2026-10-21T12:00:00Z","url":"https://github.com/o/r/pull/7#event-1"},
		{"__typename":"ReviewDismissedEvent","id":"event-2","review":{"id":"review-2"},"previousReviewState":"APPROVED"},
		{"__typename":"ReviewDismissedEvent","id":"event-3","review":{"id":"review-3"},"previousReviewState":"CHANGES_REQUESTED","pullRequestCommit":{"id":"new"}}
	],"pageInfo":{"hasNextPage":false}}}}}}`
	pr, err := timelineProvider(t, "api\\ graphql*) printf '%s' "+testShellQuote(page)+";;").GetPR(context.Background(), "o/r", 7)
	if err != nil {
		t.Fatal(err)
	}
	if !pr.ReviewActivityLoaded || len(pr.Reviews) != 4 || len(pr.ReviewDismissals) != 3 || len(pr.Comments) != 1 {
		t.Fatalf("unexpected activity or comment evidence: %+v", pr)
	}
	if r := pr.Reviews[0]; r.Author != "alice" || r.State != "APPROVED" || r.Body != "" || r.SubmittedAt.IsZero() || !strings.HasSuffix(r.URL, "#pullrequestreview-1") {
		t.Fatalf("review: %+v", r)
	}
	if d := pr.ReviewDismissals[0]; d.ReviewID != "review-1" || !d.Stale || d.CreatedAt.IsZero() || !strings.HasSuffix(d.URL, "#event-1") {
		t.Fatalf("stale dismissal: %+v", d)
	}
	if pr.ReviewDismissals[1].Stale || pr.ReviewDismissals[2].Stale || pr.ReviewDismissals[2].PreviousState != "CHANGES_REQUESTED" {
		t.Fatalf("other dismissals: %+v", pr.ReviewDismissals[1:])
	}
}

func TestGetPRReviewDismissalsPaginate(t *testing.T) {
	wrap := func(nodes, info string) string {
		return `{"data":{"repository":{"pullRequest":{"timelineItems":{"nodes":` + nodes + `,"pageInfo":` + info + `}}}}}`
	}
	node := func(id string) string { return `[{"__typename":"ReviewDismissedEvent","id":"` + id + `"}]` }
	first := wrap(node("first"), `{"hasNextPage":true,"endCursor":"next"}`)
	second := wrap(node("second"), `{"hasNextPage":false}`)
	p := timelineProvider(t, "*endCursor=next*) printf '%s' "+testShellQuote(second)+";;\napi\\ graphql*) printf '%s' "+testShellQuote(first)+";;")
	pr, err := p.GetPR(context.Background(), "o/r", 7)
	if err != nil || len(pr.ReviewDismissals) != 2 || pr.ReviewDismissals[1].ID != "second" {
		t.Fatalf("paginated result: %+v, %v", pr.ReviewDismissals, err)
	}
	p = timelineProvider(t, "api\\ graphql*) printf '%s' "+testShellQuote(`{"errors":[{"message":"timeline failed"}]}`)+";;")
	if pr, err = p.GetPR(context.Background(), "o/r", 7); err == nil || !strings.Contains(err.Error(), "timeline failed") || pr.ReviewActivityLoaded {
		t.Fatalf("error result: %+v, %v", pr, err)
	}
}

func timelineProvider(t *testing.T, cases string) *GHProvider {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "gh")
	script := "#!/bin/sh\ncase \"$*\" in\n" + cases + "\npr*) printf '%s' '{\"number\":7,\"reviews\":[{\"body\":\"Review body\",\"state\":\"COMMENTED\"}]}' ;; \n*) printf '[[]]' ;; \nesac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &GHProvider{Bin: bin}
}

func TestGetPRRejectsIncompleteReviewTimeline(t *testing.T) {
	for _, timeline := range []string{
		`{}`, `{"nodes":null,"pageInfo":{"hasNextPage":false}}`,
		`{"nodes":[]}`, `{"nodes":[],"pageInfo":{}}`,
		`{"nodes":[],"pageInfo":{"hasNextPage":null}}`,
	} {
		t.Run(timeline, func(t *testing.T) {
			response := `{"data":{"repository":{"pullRequest":{"timelineItems":` + timeline + `}}}}`
			p := timelineProvider(t, "api\\ graphql*) printf '%s' "+testShellQuote(response)+";;")
			pr, err := p.GetPR(context.Background(), "o/r", 7)
			if err == nil || pr.ReviewActivityLoaded {
				t.Fatalf("incomplete timeline accepted: loaded=%t, error=%v", pr.ReviewActivityLoaded, err)
			}
		})
	}
}
