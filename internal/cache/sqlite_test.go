package cache

import (
	"os"
	"path/filepath"
	"testing"

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
	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 1, Title: "T", Triage: model.Triage{Risk: model.RiskSafe}}); err != nil {
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
		Repo:     "owner/repo",
		Number:   7,
		Title:    "Update docs",
		Author:   "jane",
		CIStatus: "passing",
		Triage:   model.Triage{Risk: model.RiskSafe, Summary: "docs-only"},
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
	if prs[0].Triage.Risk != model.RiskSafe {
		t.Fatalf("risk = %s", prs[0].Triage.Risk)
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

	if err := store.Put(model.PullRequest{Repo: "owner/repo", Number: 1, Title: "T", Triage: model.Triage{Risk: model.RiskReview}}); err != nil {
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

func TestSQLiteStorePruneRepo(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "zappr.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()

	for _, pr := range []model.PullRequest{
		{Repo: "owner/repo", Number: 1, Title: "closed", Triage: model.Triage{Risk: model.RiskSafe}},
		{Repo: "owner/repo", Number: 2, Title: "open", Triage: model.Triage{Risk: model.RiskSafe}},
		{Repo: "other/repo", Number: 1, Title: "other", Triage: model.Triage{Risk: model.RiskSafe}},
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
		{Repo: "owner/repo", Number: 1, Title: "a", Triage: model.Triage{Risk: model.RiskSafe}},
		{Repo: "owner/repo", Number: 2, Title: "b", Triage: model.Triage{Risk: model.RiskSafe}},
		{Repo: "other/repo", Number: 9, Title: "leftover", Triage: model.Triage{Risk: model.RiskSafe}},
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
