package wikigo

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// changes records the page changes a wiki reports, and refuses them while
// refuse is set.
type changes struct {
	mu     sync.Mutex
	got    []Change
	refuse error
}

func (c *changes) onChange(ch Change) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, ch)
	return c.refuse
}

// take returns the changes recorded since the last call.
func (c *changes) take() []Change {
	c.mu.Lock()
	defer c.mu.Unlock()
	got := c.got
	c.got = nil
	return got
}

// wait returns the changes recorded since the last call, once there are n.
func (c *changes) wait(t *testing.T, n int) []Change {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		count := len(c.got)
		c.mu.Unlock()
		if count >= n {
			return c.take()
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waited for %d page changes, got %v", n, c.take())
	return nil
}

func newChangeWiki(t *testing.T) (dataDir string, srv *httptest.Server, rec *changes) {
	t.Helper()
	dataDir, cfg := newWiki(t)
	rec = &changes{}
	h, err := New(Options{ConfigFile: cfg, OwnerUser: "owner", OnChange: rec.onChange})
	if err != nil {
		t.Fatal(err)
	}
	srv = httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return dataDir, srv, rec
}

// send makes a request with a body and content type, and returns the status
// and the decoded JSON message.
func send(t *testing.T, srv *httptest.Server, method, path, contentType string, body io.Reader) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var answer struct {
		Message string `json:"message"`
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &answer)
	return resp.StatusCode, answer.Message
}

func sendJSON(t *testing.T, srv *httptest.Server, method, path, body string) (int, string) {
	t.Helper()
	return send(t, srv, method, path, "application/json", strings.NewReader(body))
}

func multipartBody(t *testing.T, fields map[string]string, fileField, fileName string, file []byte) (string, io.Reader) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	part, err := w.CreateFormFile(fileField, fileName)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(file)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return w.FormDataContentType(), &buf
}

func wantChanges(t *testing.T, step string, got []Change, want ...Change) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: changes = %+v, want %+v", step, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: change %d = %+v, want %+v", step, i, got[i], want[i])
		}
	}
}

func TestOnChangeReportsEachPageWrite(t *testing.T) {
	dataDir, srv, rec := newChangeWiki(t)

	// Save of a new page, then of the same page again.
	if code, _ := send(t, srv, http.MethodPost, "/api/save/personal/saved", "text/plain", strings.NewReader("# One\n")); code != http.StatusOK {
		t.Fatalf("save new = %d", code)
	}
	wantChanges(t, "save new", rec.take(), Change{Action: "create", Path: "personal/saved"})
	if code, _ := send(t, srv, http.MethodPost, "/api/save/personal/saved", "text/plain", strings.NewReader("# Two\n")); code != http.StatusOK {
		t.Fatalf("save = %d", code)
	}
	wantChanges(t, "save", rec.take(), Change{Action: "edit", Path: "personal/saved"})

	// The home page is not under the documents folder.
	if code, _ := send(t, srv, http.MethodPost, "/api/save/", "text/plain", strings.NewReader("# Home\n")); code != http.StatusOK {
		t.Fatalf("save home = %d", code)
	}
	wantChanges(t, "save home", rec.take())

	// Create.
	if code, msg := sendJSON(t, srv, http.MethodPost, "/api/document/create", `{"title":"A","path":"personal/a"}`); code != http.StatusOK {
		t.Fatalf("create = %d %s", code, msg)
	}
	wantChanges(t, "create", rec.take(), Change{Action: "create", Path: "personal/a"})

	// Version restore: the save above made a version of personal/saved.
	versions, err := filepath.Glob(filepath.Join(dataDir, "versions", "documents", "personal", "saved", "*.md"))
	if err != nil || len(versions) == 0 {
		t.Fatalf("versions = %v (err %v), want one", versions, err)
	}
	stamp := strings.TrimSuffix(filepath.Base(versions[0]), ".md")
	if code, msg := send(t, srv, http.MethodPost, "/api/versions/personal/saved/"+stamp+"/restore", "", nil); code != http.StatusOK {
		t.Fatalf("restore = %d %s", code, msg)
	}
	wantChanges(t, "restore", rec.take(), Change{Action: "edit", Path: "personal/saved"})

	// Upload, rename and delete of a file on a page are edits of the page.
	ct, body := multipartBody(t, map[string]string{"docPath": "personal/a"}, "file", "notes.txt", []byte("plain text\n"))
	if code, msg := send(t, srv, http.MethodPost, "/api/files/upload", ct, body); code != http.StatusOK {
		t.Fatalf("upload = %d %s", code, msg)
	}
	wantChanges(t, "upload", rec.take(), Change{Action: "edit", Path: "personal/a"})
	if code, msg := sendJSON(t, srv, http.MethodPost, "/api/files/rename", `{"currentPath":"personal/a/notes.txt","newName":"renamed.txt"}`); code != http.StatusOK {
		t.Fatalf("rename = %d %s", code, msg)
	}
	wantChanges(t, "rename", rec.take(), Change{Action: "edit", Path: "personal/a"})
	if code, msg := send(t, srv, http.MethodDelete, "/api/files/delete/personal/a/renamed.txt", "", nil); code != http.StatusOK {
		t.Fatalf("file delete = %d %s", code, msg)
	}
	wantChanges(t, "file delete", rec.take(), Change{Action: "edit", Path: "personal/a"})

	// Move within a scope, then across scopes.
	if code, msg := sendJSON(t, srv, http.MethodPost, "/api/document/move", `{"sourcePath":"personal/a","targetPath":"personal","newSlug":"b"}`); code != http.StatusOK {
		t.Fatalf("move = %d %s", code, msg)
	}
	wantChanges(t, "move", rec.take(), Change{Action: "move", Path: "personal/b", OldPath: "personal/a"})
	if code, msg := sendJSON(t, srv, http.MethodPost, "/api/document/move", `{"sourcePath":"personal/b","targetPath":"work"}`); code != http.StatusOK {
		t.Fatalf("move across = %d %s", code, msg)
	}
	wantChanges(t, "move across", rec.take(), Change{Action: "move", Path: "work/b", OldPath: "personal/b"})

	// Delete.
	if code, msg := send(t, srv, http.MethodDelete, "/api/document/work/b", "", nil); code != http.StatusOK {
		t.Fatalf("delete = %d %s", code, msg)
	}
	wantChanges(t, "delete", rec.take(), Change{Action: "delete", Path: "work/b"})

	// Import writes one page per Markdown file, after the request is answered.
	var zipped bytes.Buffer
	zw := zip.NewWriter(&zipped)
	f, err := zw.Create("imported/page.md")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(f, "# Imported\n")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	ct, body = multipartBody(t, nil, "zipFile", "pages.zip", zipped.Bytes())
	if code, msg := send(t, srv, http.MethodPost, "/api/import", ct, body); code != http.StatusOK {
		t.Fatalf("import = %d %s", code, msg)
	}
	wantChanges(t, "import", rec.wait(t, 1), Change{Action: "create", Path: "imported/page"})
}

func TestOnChangeErrorAnswers422WithItsText(t *testing.T) {
	dataDir, srv, rec := newChangeWiki(t)
	rec.refuse = errors.New("missing type")

	code, msg := send(t, srv, http.MethodPost, "/api/save/personal/refused", "text/plain", strings.NewReader("# Refused\n"))
	if code != http.StatusUnprocessableEntity || msg != "missing type" {
		t.Errorf("save = %d %q, want 422 \"missing type\"", code, msg)
	}
	wantChanges(t, "refused save", rec.take(), Change{Action: "create", Path: "personal/refused"})
	// The wiki leaves the written page for the callback's owner to handle.
	if _, err := os.Stat(filepath.Join(dataDir, "documents", "personal", "refused", "document.md")); err != nil {
		t.Errorf("page after a refused save: %v", err)
	}

	code, msg = sendJSON(t, srv, http.MethodPost, "/api/document/create", `{"title":"C","path":"personal/c"}`)
	if code != http.StatusUnprocessableEntity || msg != "missing type" {
		t.Errorf("create = %d %q, want 422 \"missing type\"", code, msg)
	}
}

func TestWithoutOnChangeWritesAreNotReported(t *testing.T) {
	_, cfg := newWiki(t)
	// A wiki built with OnChange, then one without: the second must not call
	// the first one's callback.
	rec := &changes{}
	if _, err := New(Options{ConfigFile: cfg, OwnerUser: "owner", OnChange: rec.onChange}); err != nil {
		t.Fatal(err)
	}
	h, err := New(Options{ConfigFile: cfg, OwnerUser: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	if code, _ := send(t, srv, http.MethodPost, "/api/save/personal/x", "text/plain", strings.NewReader("# X\n")); code != http.StatusOK {
		t.Fatalf("save = %d", code)
	}
	wantChanges(t, "save", rec.take())
}

func TestEditorShowsTheSaveMessage(t *testing.T) {
	_, srv, _ := newChangeWiki(t)
	code, js := get(t, srv, "/static/js/editor.js", nil)
	if code != http.StatusOK {
		t.Fatalf("editor.js = %d", code)
	}
	for _, want := range []string{"await response.json()", "saveError.message = data.message", "alert(error.fromServer ? error.message : 'Failed to save changes')"} {
		if !strings.Contains(js, want) {
			t.Errorf("editor.js lacks %q", want)
		}
	}
}
