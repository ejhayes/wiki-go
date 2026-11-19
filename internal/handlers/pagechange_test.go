package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wiki-go/internal/config"
)

// The links page handlers have no route yet, so these tests call them
// directly.
func TestLinksHandlersReportAnEdit(t *testing.T) {
	previousConfig := cfg
	t.Cleanup(func() {
		cfg = previousConfig
		SetPageChangeHook(nil)
	})
	testConfig := &config.Config{}
	testConfig.Wiki.RootDir = t.TempDir()
	testConfig.Wiki.DocumentsDir = "documents"
	cfg = testConfig

	page := filepath.Join(testConfig.Wiki.RootDir, "documents", "personal", "links", "document.md")
	if err := os.MkdirAll(filepath.Dir(page), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nlayout: links\n---\n\n# Links\n\n## Tools\n- [Example](https://example.com) - One | 2026-01-01\n"
	if err := os.WriteFile(page, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var got []PageChange
	var refuse error
	SetPageChangeHook(func(c PageChange) error {
		got = append(got, c)
		return refuse
	})

	steps := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string
		body    string
	}{
		{"add", AddLinkHandler, http.MethodPost, "/api/links/add/personal%2Flinks",
			`{"url":"https://example.org","title":"Org","category":"Tools"}`},
		{"edit", EditLinkHandler, http.MethodPut, "/api/links/edit/personal%2Flinks",
			`{"oldUrl":"https://example.org","newLink":{"url":"https://example.net","title":"Net","category":"Tools"}}`},
		{"delete", DeleteLinkHandler, http.MethodDelete, "/api/links/delete/personal%2Flinks",
			`{"url":"https://example.net"}`},
	}
	for _, step := range steps {
		got = nil
		w := httptest.NewRecorder()
		step.handler(w, httptest.NewRequest(step.method, step.path, strings.NewReader(step.body)))
		if w.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.name, w.Code, w.Body.String())
		}
		if len(got) != 1 || got[0] != (PageChange{Action: PageEdit, Path: "personal/links"}) {
			t.Errorf("%s: changes = %+v, want one edit of personal/links", step.name, got)
		}
	}

	refuse = errors.New("missing type")
	w := httptest.NewRecorder()
	AddLinkHandler(w, httptest.NewRequest(http.MethodPost, "/api/links/add/personal%2Flinks",
		strings.NewReader(`{"url":"https://example.com/x","title":"X","category":"Tools"}`)))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `"message":"missing type"`) {
		t.Errorf("refused add = %d %s, want 422 with the message", w.Code, w.Body.String())
	}
}
