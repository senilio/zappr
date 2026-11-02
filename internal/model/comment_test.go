package model

import "testing"

func TestCommentKeyUsesNativeKindAndID(t *testing.T) {
	a := PRComment{ID: "101", Body: "old"}
	b := PRComment{ID: "101", Kind: "inline"}
	if a.Key() != "comment:101" || b.Key() != "inline:101" {
		t.Fatal(a.Key(), b.Key())
	}
	a.Body = "edited"
	if a.Key() != "comment:101" {
		t.Fatal("native identity changed on edit")
	}
}

func TestCommentKeyFallbackIsDeterministic(t *testing.T) {
	a := PRComment{Author: "sam", Body: "old"}
	if a.Key() == "" || a.Key() != a.Key() {
		t.Fatal("unstable fallback")
	}
	b := a
	b.Body = "new"
	if a.Key() == b.Key() {
		t.Fatal("legacy contents not distinguished")
	}
}
