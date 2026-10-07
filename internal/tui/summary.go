package tui

import (
	"slices"
	"time"

	"github.com/senilio/zappr/internal/model"
)

func newestCommits(commits []model.PRCommit) []model.PRCommit {
	ordered := slices.Clone(commits)
	slices.SortStableFunc(ordered, func(a, b model.PRCommit) int {
		return b.Date.Compare(a.Date)
	})
	return ordered
}

func newestComments(comments []model.PRComment) []model.PRComment {
	ordered := slices.Clone(comments)
	slices.SortStableFunc(ordered, func(a, b model.PRComment) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return ordered
}

func summaryTimestamp(date, now time.Time) string {
	if date.IsZero() {
		return ""
	}
	date = date.Local()
	now = now.Local()
	// Use UTC for calendar arithmetic so midnight DST changes cannot shift the day.
	yesterday := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC)
	day := date.Format("2006-01-02")
	switch day {
	case now.Format("2006-01-02"):
		day = "Today"
	case yesterday.Format("2006-01-02"):
		day = "Yesterday"
	}
	return day + " " + date.Format("15:04")
}
