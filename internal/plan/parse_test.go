package plan

import (
	"github.com/senilio/zappr/internal/model"
	"os"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func planComment(id, body string, second int) model.PRComment {
	when := time.Unix(1000+int64(second), 0).UTC()
	return model.PRComment{ID: id, URL: "https://github.com/example/infra/issues/7#issuecomment-" + id, Author: "deploy-bot[bot]", CreatedAt: when, UpdatedAt: when, Body: body}
}
func fivePartComments(t *testing.T) []model.PRComment {
	c := []model.PRComment{planComment("1", fixture(t, "split-root"), 0)}
	for i, id := range []string{"2", "3", "4"} {
		c = append(c, planComment(id, strings.Replace(fixture(t, "split-middle"), "PART", id, 1), 1))
		c[i+1].UpdatedAt = c[i+1].CreatedAt.Add(time.Duration(4-i) * time.Second)
	}
	return append(c, planComment("5", fixture(t, "split-terminal"), 2))
}
func TestParseFivePartChain(t *testing.T) {
	comments := fivePartComments(t)
	doc := Parse(comments, true)
	if len(doc.Reports) != 1 {
		t.Fatal(len(doc.Reports))
	}
	r := doc.Reports[0]
	if !r.SourceComplete || len(r.Sources) != 5 || r.CommitVerified || len(r.Projects) != 1 || !r.Projects[0].DetailAvailable {
		t.Fatal(r)
	}
	p := r.Projects[0]
	if strings.Contains(p.Text, "Continued plan output") || len(p.LineSources) != len(strings.Split(p.Text, "\n")) {
		t.Fatal(p)
	}
	for i, s := range p.LineSources {
		if s.CommentKey == "comment:5" && strings.Contains(strings.Split(p.Text, "\n")[i], "Last line") {
			return
		}
	}
	t.Fatal("terminal source mapping lost")
}
func TestParseSameSecondProviderOrder(t *testing.T) {
	r := Parse(fivePartComments(t), true).Reports[0]
	for i, id := range []string{"1", "2", "3", "4", "5"} {
		if r.Sources[i].CommentKey != "comment:"+id {
			t.Fatal(r.Sources)
		}
	}
}
func TestParseDoesNotJoinAcrossInterveningComments(t *testing.T) {
	c := fivePartComments(t)
	c = append(c[:1], append([]model.PRComment{planComment("human", "hello", 0)}, c[1:]...)...)
	doc := Parse(c, true)
	for _, r := range doc.Reports {
		if r.ID == "comment:1:0" && (r.SourceComplete || len(r.Sources) != 1) {
			t.Fatal(r)
		}
	}
}
func TestParseRejectsOrphanOrConflictingContinuations(t *testing.T) {
	for _, c := range [][]model.PRComment{
		{planComment("1", fixture(t, "split-terminal"), 0)},
		{planComment("1", fixture(t, "split-root"), 0), planComment("2", fixture(t, "single-project"), 1)},
	} {
		doc := Parse(c, true)
		if len(doc.Reports) == 0 || doc.Reports[len(doc.Reports)-1].SourceComplete {
			t.Fatal(doc)
		}
	}
	c := fivePartComments(t)
	c[1].Author = "other-bot"
	doc := Parse(c, true)
	for _, r := range doc.Reports {
		if r.ID == "comment:1:0" && r.SourceComplete {
			t.Fatal(r)
		}
	}
}
func TestParseIncompleteFetchAndLegacyIDs(t *testing.T) {
	for _, complete := range []bool{false, true} {
		c := fivePartComments(t)
		if complete {
			for i := range c {
				c[i].ID = ""
			}
		}
		for _, r := range Parse(c, complete).Reports {
			if r.SourceComplete {
				t.Fatal(r)
			}
		}
	}
}
func TestFingerprintChangesOnEditNotReportIdentity(t *testing.T) {
	c := []model.PRComment{planComment("1", fixture(t, "single-project"), 0)}
	before := Parse(c, true)
	fp := Fingerprint(c, true)
	c[0].Body = strings.Replace(c[0].Body, "alpha", "beta", -1)
	c[0].UpdatedAt = c[0].UpdatedAt.Add(time.Second)
	after := Parse(c, true)
	if fp == Fingerprint(c, true) || before.Reports[0].ID != after.Reports[0].ID || before.Reports[0].Revision == after.Reports[0].Revision {
		t.Fatal("revision/identity conflated")
	}
}
