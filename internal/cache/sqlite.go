package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/senilio/zappr/internal/model"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	path string
	db   *sql.DB
}

func OpenSQLite(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("cache path is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &SQLiteStore{path: path, db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	for _, file := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(file, 0o600); err != nil && !os.IsNotExist(err) {
			db.Close()
			return nil, err
		}
	}
	return store, nil
}

func (s *SQLiteStore) Put(pr model.PullRequest) error {
	if old, err := s.get(pr.Repo, pr.Number); err == nil {
		pr.Skipped = old.Skipped
	}
	data, err := json.Marshal(pr)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
insert into pull_requests (repo, number, title, author, risk, ci_status, skipped, data, updated_at)
values (?, ?, ?, ?, ?, ?, ?, ?, ?)
on conflict(repo, number) do update set
  title = excluded.title,
  author = excluded.author,
  risk = excluded.risk,
  ci_status = excluded.ci_status,
  skipped = excluded.skipped,
  data = excluded.data,
  updated_at = excluded.updated_at
`, pr.Repo, pr.Number, pr.Title, pr.Author, string(pr.Triage.Risk), pr.CIStatus, boolToInt(pr.Skipped), string(data), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) Get(repo string, number int) (model.PullRequest, error) {
	return s.get(repo, number)
}

func (s *SQLiteStore) Delete(repo string, number int) error {
	_, err := s.db.Exec(`delete from pull_requests where repo = ? and number = ?`, repo, number)
	return err
}

func (s *SQLiteStore) List() ([]model.PullRequest, error) {
	rows, err := s.db.Query(`select data, skipped from pull_requests order by updated_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prs []model.PullRequest
	for rows.Next() {
		var data string
		var skipped int
		if err := rows.Scan(&data, &skipped); err != nil {
			return nil, err
		}
		var pr model.PullRequest
		if err := json.Unmarshal([]byte(data), &pr); err != nil {
			return nil, err
		}
		pr.Skipped = skipped != 0
		prs = append(prs, pr)
	}
	return prs, rows.Err()
}

func (s *SQLiteStore) ListRepo(repo string) ([]model.PullRequest, error) {
	rows, err := s.db.Query(`select data, skipped from pull_requests where repo = ? order by updated_at desc`, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prs []model.PullRequest
	for rows.Next() {
		var data string
		var skipped int
		if err := rows.Scan(&data, &skipped); err != nil {
			return nil, err
		}
		var pr model.PullRequest
		if err := json.Unmarshal([]byte(data), &pr); err != nil {
			return nil, err
		}
		pr.Skipped = skipped != 0
		prs = append(prs, pr)
	}
	return prs, rows.Err()
}

func (s *SQLiteStore) PruneRepo(repo string, openNumbers map[int]bool) error {
	rows, err := s.db.Query(`select number from pull_requests where repo = ?`, repo)
	if err != nil {
		return err
	}
	defer rows.Close()

	var closed []int
	for rows.Next() {
		var number int
		if err := rows.Scan(&number); err != nil {
			return err
		}
		if !openNumbers[number] {
			closed = append(closed, number)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, number := range closed {
		if _, err := s.db.Exec(`delete from pull_requests where repo = ? and number = ?`, repo, number); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) SetSkip(repo string, number int, skipped bool) error {
	pr, err := s.get(repo, number)
	if err != nil {
		return nil
	}
	pr.Skipped = skipped
	data, err := json.Marshal(pr)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`update pull_requests set skipped = ?, data = ?, updated_at = ? where repo = ? and number = ?`, boolToInt(skipped), string(data), time.Now().UTC().Format(time.RFC3339), repo, number)
	return err
}

func (s *SQLiteStore) Clear() error {
	_, err := s.db.Exec(`delete from pull_requests`)
	return err
}

func (s *SQLiteStore) Path() string {
	return s.path
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
pragma journal_mode = wal;
create table if not exists pull_requests (
  repo text not null,
  number integer not null,
  title text not null,
  author text not null,
  risk text not null,
  ci_status text not null,
  skipped integer not null default 0,
  data text not null,
  updated_at text not null,
  primary key (repo, number)
);
create index if not exists idx_pull_requests_updated_at on pull_requests(updated_at);
create index if not exists idx_pull_requests_risk on pull_requests(risk);
`)
	return err
}

func (s *SQLiteStore) get(repo string, number int) (model.PullRequest, error) {
	var data string
	var skipped int
	err := s.db.QueryRow(`select data, skipped from pull_requests where repo = ? and number = ?`, repo, number).Scan(&data, &skipped)
	if err != nil {
		return model.PullRequest{}, err
	}
	var pr model.PullRequest
	if err := json.Unmarshal([]byte(data), &pr); err != nil {
		return model.PullRequest{}, err
	}
	pr.Skipped = skipped != 0
	return pr, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
