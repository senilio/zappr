package cache

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/senilio/zappr/internal/model"
)

func TestOpenSQLitePrivateModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	path := filepath.Join(dir, "zappr.db")
	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()

	if mode := sqliteFileMode(t, dir); mode != 0o700 {
		t.Fatalf("cache directory mode = %o, want 700", mode)
	}
	if mode := sqliteFileMode(t, path); mode != 0o600 {
		t.Fatalf("database mode = %o, want 600", mode)
	}
	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 1, Title: "T"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	sidecars := []string{path + "-wal", path + "-shm"}
	for _, sidecar := range sidecars {
		if _, err := os.Stat(sidecar); os.IsNotExist(err) {
			t.Skipf("SQLite did not create %s", filepath.Base(sidecar))
		} else if err != nil {
			t.Fatalf("stat %s: %v", sidecar, err)
		}
		if mode := sqliteFileMode(t, sidecar); mode != 0o600 {
			t.Fatalf("%s mode = %o, want 600", filepath.Base(sidecar), mode)
		}
	}

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod cache directory: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod database: %v", err)
	}
	for _, sidecar := range sidecars {
		if err := os.Chmod(sidecar, 0o644); err != nil {
			t.Fatalf("chmod %s: %v", filepath.Base(sidecar), err)
		}
	}
	store2, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite second: %v", err)
	}
	defer store2.Close()
	if mode := sqliteFileMode(t, dir); mode != 0o700 {
		t.Fatalf("corrected cache directory mode = %o, want 700", mode)
	}
	if mode := sqliteFileMode(t, path); mode != 0o600 {
		t.Fatalf("corrected database mode = %o, want 600", mode)
	}
	for _, sidecar := range sidecars {
		if mode := sqliteFileMode(t, sidecar); mode != 0o600 {
			t.Fatalf("corrected %s mode = %o, want 600", filepath.Base(sidecar), mode)
		}
	}
}

func sqliteFileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

func TestSQLiteStorePersistsPRs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zappr.db")
	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()

	pr := model.PullRequest{
		Repo:      "owner/repo",
		Number:    7,
		Title:     "Update docs",
		Author:    "jane",
		CIStatus:  "passing",
		Draft:     true,
		AutoMerge: true,
	}
	if err := store.Put(pr); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.SetSkip("owner/repo", 7, true); err != nil {
		t.Fatalf("SetSkip: %v", err)
	}

	store2, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite second: %v", err)
	}
	defer store2.Close()

	prs, err := store2.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("len = %d, want 1", len(prs))
	}
	if !prs[0].Skipped {
		t.Fatal("skipped state not persisted")
	}
	if !prs[0].Draft || !prs[0].AutoMerge {
		t.Fatal("PR state badges not persisted")
	}
	if prs[0].Title != "Update docs" || prs[0].Author != "jane" || prs[0].CIStatus != "passing" {
		t.Fatalf("PR details not persisted: %#v", prs[0])
	}

	if err := store2.SetSkip("owner/repo", 7, false); err != nil {
		t.Fatalf("SetSkip unskip: %v", err)
	}
	prs, err = store2.List()
	if err != nil {
		t.Fatalf("List after unskip: %v", err)
	}
	if prs[0].Skipped {
		t.Fatal("unskip state not persisted")
	}
}

func TestSQLiteStoreClear(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "zappr.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()

	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 1, Title: "T"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	prs, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(prs) != 0 {
		t.Fatalf("len = %d, want 0", len(prs))
	}
}

func TestSQLiteStorePersistsReviewCommentMetadata(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "zappr.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	data := `{"Repo":"owner/repo","Number":7,"details_loaded":true,"review_comments_loaded":true,"Comments":[{"Author":"bot","Body":"Summary","Kind":"review"},{"Author":"alice","Body":"Inline","Kind":"inline","Path":"roles.tf","Line":291,"OriginalLine":290,"Side":"LEFT"}]}`
	var pr model.PullRequest
	if err := json.Unmarshal([]byte(data), &pr); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(pr); err != nil {
		t.Fatal(err)
	}
	for name, load := range map[string]func() (model.PullRequest, error){
		"get": func() (model.PullRequest, error) { return store.Get("owner/repo", 7) },
		"list": func() (model.PullRequest, error) {
			prs, err := store.List()
			if err != nil {
				return model.PullRequest{}, err
			}
			return prs[0], nil
		},
		"list repo": func() (model.PullRequest, error) {
			prs, err := store.ListRepo("owner/repo")
			if err != nil {
				return model.PullRequest{}, err
			}
			return prs[0], nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := load()
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if string(fields["review_comments_loaded"]) != "true" {
				t.Fatalf("review comments loaded marker lost: %s", encoded)
			}
			var comments []map[string]any
			if err := json.Unmarshal(fields["Comments"], &comments); err != nil {
				t.Fatal(err)
			}
			if len(comments) != 2 || comments[0]["Kind"] != "review" || comments[1]["Kind"] != "inline" || comments[1]["Path"] != "roles.tf" || comments[1]["Line"] != float64(291) || comments[1]["OriginalLine"] != float64(290) || comments[1]["Side"] != "LEFT" {
				t.Fatalf("review comment metadata lost: %s", encoded)
			}
		})
	}
}

func TestSQLiteStoreIgnoresRetiredCachedDetails(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "zappr.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 7}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Old cached rows must remain readable without retaining removed details.
	data := `{"Repo":"owner/repo","Number":7,"Title":"Cached PR","Comments":[{"Author":"bot","Body":"Plan: 1 to add, 0 to change, 0 to destroy."}],"details_loaded":true,"atlantis_plan":{"summary":"legacy plan"},"Triage":{"risk":"safe","confidence":0.9,"summary":"old recommendation","AI":{"summary":"old AI recommendation"}}}`
	if _, err := store.db.Exec("update pull_requests set data = ? where repo = ? and number = ?", data, "owner/repo", 7); err != nil {
		t.Fatalf("write legacy data: %v", err)
	}
	want := model.PullRequest{
		Repo: "owner/repo", Number: 7, Title: "Cached PR", DetailsLoaded: true,
		Comments: []model.PRComment{{Author: "bot", Body: "Plan: 1 to add, 0 to change, 0 to destroy."}},
	}
	pr, err := store.Get("owner/repo", 7)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(pr, want) {
		t.Fatalf("cached PR = %#v, want %#v", pr, want)
	}
	for _, list := range []func() ([]model.PullRequest, error){store.List, func() ([]model.PullRequest, error) { return store.ListRepo("owner/repo") }} {
		prs, err := list()
		if err != nil || !reflect.DeepEqual(prs, []model.PullRequest{want}) {
			t.Fatalf("cached PR list = %#v, err = %v", prs, err)
		}
	}
	if err := store.Put(pr); err != nil {
		t.Fatalf("rewrite cached PR: %v", err)
	}
	if err := store.db.QueryRow("select data from pull_requests where repo = ? and number = ?", "owner/repo", 7).Scan(&data); err != nil {
		t.Fatalf("read rewritten data: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &fields); err != nil {
		t.Fatalf("decode rewritten data: %v", err)
	}
	for _, retired := range []string{"atlantis_plan", "Triage"} {
		if _, ok := fields[retired]; ok {
			t.Fatalf("rewritten cache still contains retired field %q", retired)
		}
	}
}

func TestSQLiteStoreMigratesLegacyRiskWithoutLosingPRs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zappr.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`create table pull_requests (
		repo text not null, number integer not null, title text not null,
		author text not null, risk text not null, ci_status text not null,
		skipped integer not null default 0, data text not null,
		updated_at text not null, primary key (repo, number)
	);
	create index idx_pull_requests_risk on pull_requests(risk);
	insert into pull_requests values ('owner/repo', 7, 'Cached PR', 'user', 'safe', 'passing', 1,
		'{"Repo":"owner/repo","Number":7,"Title":"Cached PR","Comments":[{"Body":"Keep this comment"}],"Triage":{"AI":{"summary":"old recommendation"}}}',
		'2026-01-01T00:00:00Z');`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	for i := 0; i < 2; i++ {
		store, err := OpenSQLite(path)
		if err != nil {
			t.Fatalf("open migrated database: %v", err)
		}
		pr, err := store.Get("owner/repo", 7)
		if err != nil || pr.Title != "Cached PR" || !pr.Skipped || len(pr.Comments) != 1 || pr.Comments[0].Body != "Keep this comment" {
			t.Fatalf("legacy PR lost: %#v, err = %v", pr, err)
		}
		var columns int
		if err := store.db.QueryRow("select count(*) from pragma_table_info('pull_requests') where name = 'risk'").Scan(&columns); err != nil {
			t.Fatalf("inspect migrated schema: %v", err)
		}
		if columns != 0 {
			t.Fatal("legacy risk column still present")
		}
		if err := store.Put(pr); err != nil {
			t.Fatalf("update migrated PR: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("close migrated database: %v", err)
		}
	}
}

func TestSQLiteStorePruneRepo(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "zappr.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()

	for _, pr := range []model.PullRequest{
		{Repo: "owner/repo", Number: 1, Title: "closed"},
		{Repo: "owner/repo", Number: 2, Title: "open"},
		{Repo: "other/repo", Number: 1, Title: "other"},
	} {
		if err := store.Put(pr); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	if err := store.PruneRepo("owner/repo", map[int]bool{2: true}); err != nil {
		t.Fatalf("PruneRepo: %v", err)
	}

	prs, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("len = %d, want 2", len(prs))
	}
	for _, pr := range prs {
		if pr.Repo == "owner/repo" && pr.Number == 1 {
			t.Fatal("closed PR was not pruned")
		}
	}
}

func TestSQLiteStoreListRepoScopesToRepo(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "zappr.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()

	for _, pr := range []model.PullRequest{
		{Repo: "owner/repo", Number: 1, Title: "a"},
		{Repo: "owner/repo", Number: 2, Title: "b"},
		{Repo: "other/repo", Number: 9, Title: "leftover"},
	} {
		if err := store.Put(pr); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	// A repo with PRs returns only its own.
	prs, err := store.ListRepo("owner/repo")
	if err != nil {
		t.Fatalf("ListRepo: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("ListRepo(owner/repo) len = %d, want 2", len(prs))
	}
	for _, pr := range prs {
		if pr.Repo != "owner/repo" {
			t.Fatalf("ListRepo returned foreign repo %s", pr.Repo)
		}
	}

	// A repo with zero PRs must not surface other repos' stale rows.
	empty, err := store.ListRepo("fresh/repo")
	if err != nil {
		t.Fatalf("ListRepo empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListRepo(fresh/repo) len = %d, want 0 (stale cross-repo PRs leaked)", len(empty))
	}
}

func TestSQLiteStoreRoundTripsChecks(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "zappr.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	started := time.Date(2026, 10, 29, 16, 31, 14, 0, time.UTC)
	checks := []model.Check{{
		Name: "atlantis/plan: sso-sso", Context: "atlantis/plan: sso-sso", WorkflowName: "wf",
		State: "SUCCESS", StartedAt: started, CompletedAt: started.Add(time.Minute), URL: "https://atlantis.example/jobs/abc",
	}}
	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 9, Title: "T", Checks: checks}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	pr, err := store.Get("owner/repo", 9)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(pr.Checks, checks) {
		t.Fatalf("Checks = %#v, want %#v", pr.Checks, checks)
	}
	var data string
	if err := store.db.QueryRow("select data from pull_requests where repo = ? and number = ?", "owner/repo", 9).Scan(&data); err != nil {
		t.Fatalf("read stored data: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &fields); err != nil {
		t.Fatalf("decode stored data: %v", err)
	}
	if _, ok := fields["checks"]; !ok {
		t.Fatalf("stored data has no checks key: %s", data)
	}
	if _, ok := fields["atlantis_plan"]; ok {
		t.Fatalf("stored data contains atlantis_plan: %s", data)
	}
}

func TestSQLiteStoreRoundTripsReviewers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zappr.db")
	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	reviewers := []model.Reviewer{{Login: "alice", State: "APPROVED"}}
	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 9, MergeState: "BEHIND", Reviewers: reviewers, ReviewersLoaded: true}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	store.Close()
	store, err = OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer store.Close()
	pr, err := store.Get("owner/repo", 9)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if pr.MergeState != "BEHIND" || !pr.ReviewersLoaded || !reflect.DeepEqual(pr.Reviewers, reviewers) {
		t.Fatalf("cached PR = %#v", pr)
	}
	var data string
	if err := store.db.QueryRow("select data from pull_requests where repo = ? and number = ?", "owner/repo", 9).Scan(&data); err != nil {
		t.Fatalf("read stored data: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &fields); err != nil {
		t.Fatalf("decode stored data: %v", err)
	}
	for _, key := range []string{"merge_state", "reviewers", "reviewers_loaded"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("stored data has no %s key: %s", key, data)
		}
	}
}

func TestSQLiteStoreLoadsLegacyRowWithoutReviewers(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "zappr.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 7}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	data := `{"Repo":"owner/repo","Number":7,"Title":"Cached PR","details_loaded":true,"review_comments_loaded":true}`
	if _, err := store.db.Exec("update pull_requests set data = ? where repo = ? and number = ?", data, "owner/repo", 7); err != nil {
		t.Fatalf("write legacy data: %v", err)
	}
	pr, err := store.Get("owner/repo", 7)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !pr.DetailsLoaded || pr.ReviewersLoaded || pr.Reviewers != nil || pr.MergeState != "" {
		t.Fatalf("legacy PR = %#v", pr)
	}
}
