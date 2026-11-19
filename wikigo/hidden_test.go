package wikigo

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHiddenFoldersAreNotPages puts a Git repository's files in a page
// folder, as a program that versions the pages does, and checks that the wiki
// neither lists, serves nor writes them.
func TestHiddenFoldersAreNotPages(t *testing.T) {
	dataDir, srv, _ := newChangeWiki(t)
	writePage(t, dataDir, "personal", "# Personal\n")
	writePage(t, dataDir, "personal/.git/objects/ab", "# Not a page\n")
	root := filepath.Join(dataDir, "documents", "personal")
	for name, text := range map[string]string{".gitignore": "/inbox/\n", ".git/config": "[core]\n", "notes.txt": "visible\n"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, path := range []string{"/personal", "/api/documents/list", "/sitemap.xml", "/api/search?q=page", "/api/files/list/personal"} {
		code, body := get(t, srv, path, nil)
		if code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, code)
		}
		for _, hidden := range []string{".git", "gitignore", "Not a page"} {
			if strings.Contains(body, hidden) {
				t.Errorf("GET %s shows %q", path, hidden)
			}
		}
	}
	if _, body := get(t, srv, "/api/files/list/personal", nil); !strings.Contains(body, "notes.txt") {
		t.Errorf("the attachment list lost a visible file: %s", body)
	}
	for _, path := range []string{"/personal/.git", "/personal/.git/objects/ab", "/api/files/personal/.gitignore",
		"/api/files/personal/.git/config", "/api/source/personal/.git/objects/ab"} {
		if code, _ := get(t, srv, path, nil); code == http.StatusOK {
			t.Errorf("GET %s = 200, want it refused", path)
		}
	}
	if code, _ := send(t, srv, http.MethodPost, "/api/save/personal/.git/hooks", "text/plain", strings.NewReader("# x\n")); code == http.StatusOK {
		t.Error("a save into .git succeeded")
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "document.md")); err == nil {
		t.Error("a save wrote into .git")
	}
}
