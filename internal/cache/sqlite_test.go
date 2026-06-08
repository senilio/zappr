package cache

import (
	"path/filepath"
	"testing"

	"github.com/senilio/zappr/internal/model"
)

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
	if err := store.Skip("owner/repo", 7); err != nil {
		t.Fatalf("Skip: %v", err)
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
