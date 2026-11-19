package wikigo

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// entryPage is a page with the front matter block an entry carries: nested
// keys, a list and a list of maps, then a heading and a body.
const entryPage = `---
id: 01JD5Q8Z7X3K4M2N6P9R0S1T2V
type: concept
title: Sourdough starter
aliases:
  - levain
  - starter culture
relations:
  - type: part_of
    target: 01JD5Q8Z7X3K4M2N6P9R0S1T2W
  - type: uses
    target: 01JD5Q8Z7X3K4M2N6P9R0S1T2X
---

# Sourdough starter

Flour and water left to ferment until it bubbles.
`

// TestFrontMatterSurvivesASave saves a page with front matter through the
// save API and checks that the file and the source hold the saved bytes and
// that the rendered page shows the body and none of the front matter.
func TestFrontMatterSurvivesASave(t *testing.T) {
	for _, tc := range []struct {
		name, page string
	}{
		{"LF", entryPage},
		{"CRLF", strings.ReplaceAll(entryPage, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataDir, cfg := newWiki(t)
			h, err := New(Options{ConfigFile: cfg, OwnerUser: "owner"})
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(h)
			defer srv.Close()

			if code := do(t, srv, http.MethodPost, "/api/save/cooking/starter", "", nil, tc.page); code != http.StatusOK {
				t.Fatalf("save = %d, want 200", code)
			}
			onDisk, err := os.ReadFile(filepath.Join(dataDir, "documents", "cooking", "starter", "document.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(onDisk) != tc.page {
				t.Errorf("document.md differs from the saved body:\n%q", onDisk)
			}
			if code, source := get(t, srv, "/api/source/cooking/starter", nil); code != http.StatusOK || source != tc.page {
				t.Errorf("source = %d %q, want 200 with the saved body", code, source)
			}
			code, html := get(t, srv, "/cooking/starter", nil)
			if code != http.StatusOK {
				t.Fatalf("page = %d, want 200", code)
			}
			if !strings.Contains(html, `<h1 id="sourdough-starter">Sourdough starter `) {
				t.Errorf("rendered page lacks the heading")
			}
			if !strings.Contains(html, "<p>Flour and water left to ferment until it bubbles.</p>") {
				t.Errorf("rendered page lacks the body")
			}
			for _, key := range []string{"01JD5Q8Z7X3K4M2N6P9R0S1T2V", "part_of", "starter culture", "relations:"} {
				if strings.Contains(html, key) {
					t.Errorf("rendered page shows the front matter text %q", key)
				}
			}
		})
	}
}
