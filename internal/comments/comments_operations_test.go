package comments

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommentsInTheSameSecondByOneUserAreAllKept(t *testing.T) {
	commentsRoot := t.TempDir()
	const doc = "/guides/start"
	want := []string{"first", "second", "third"}
	for _, text := range want {
		if err := AddComment(commentsRoot, doc, text, "editor"); err != nil {
			t.Fatal(err)
		}
	}

	got, err := GetComments(commentsRoot, doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d comments, want %d: %#v", len(got), len(want), got)
	}
	seen := map[string]bool{}
	ids := map[string]bool{}
	for _, c := range got {
		seen[c.Content] = true
		ids[c.ID] = true
		if c.Author != "editor" {
			t.Errorf("author = %q, want editor", c.Author)
		}
	}
	for _, text := range want {
		if !seen[text] {
			t.Errorf("comment %q was lost", text)
		}
	}
	if len(ids) != len(want) {
		t.Errorf("comment file names are not distinct: %v", ids)
	}
}

func TestCommentOperationsUseConfiguredContainedRoot(t *testing.T) {
	commentsRoot := filepath.Join(t.TempDir(), "custom-comments")
	if err := AddComment(commentsRoot, "/guides/start", "nested comment", "editor"); err != nil {
		t.Fatal(err)
	}
	if err := AddComment(commentsRoot, "/", "homepage comment", "admin"); err != nil {
		t.Fatal(err)
	}

	nested, err := GetComments(commentsRoot, "/guides/start")
	if err != nil {
		t.Fatal(err)
	}
	if len(nested) != 1 || nested[0].Content != "nested comment" || nested[0].Author != "editor" {
		t.Fatalf("nested comments = %#v, want one editor comment", nested)
	}
	homepage, err := GetComments(commentsRoot, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(homepage) != 1 || homepage[0].Content != "homepage comment" || homepage[0].Author != "admin" {
		t.Fatalf("homepage comments = %#v, want one admin comment", homepage)
	}

	if _, err := os.Stat(filepath.Join(commentsRoot, "guides", "start", nested[0].ID)); err != nil {
		t.Fatalf("nested comment was not stored below configured root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(commentsRoot, homepage[0].ID)); err != nil {
		t.Fatalf("homepage comment was not stored below configured root: %v", err)
	}

	if err := DeleteComment(commentsRoot, nested[0].ID, "/guides/start", true); err != nil {
		t.Fatal(err)
	}
	nested, err = GetComments(commentsRoot, "/guides/start")
	if err != nil {
		t.Fatal(err)
	}
	if len(nested) != 0 {
		t.Fatalf("nested comments after delete = %#v, want none", nested)
	}
}

func TestCommentOperationsRejectUnsafePaths(t *testing.T) {
	commentsRoot := filepath.Join(t.TempDir(), "comments")
	const commentID = "20260921123456_admin.md"
	for _, documentPath := range []string{"../outside", `..\outside`, "%2e%2e/outside"} {
		if err := AddComment(commentsRoot, documentPath, "blocked", "admin"); err == nil {
			t.Errorf("AddComment(%q) succeeded, want error", documentPath)
		}
		if result, err := GetComments(commentsRoot, documentPath); err == nil {
			t.Errorf("GetComments(%q) = %#v, want error", documentPath, result)
		}
		if err := DeleteComment(commentsRoot, commentID, documentPath, true); err == nil {
			t.Errorf("DeleteComment(%q) succeeded, want error", documentPath)
		}
	}
}
