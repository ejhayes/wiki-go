package wikigo

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The password hash is for the word "spikepass". The tests never log in with
// it: they sign in through OwnerUser or send no credentials at all.
const testConfig = `server:
    host: 127.0.0.1
    port: 0
    allow_insecure_cookies: true
    trusted_proxies: []
    ssl: false
wiki:
    root_dir: %q
    documents_dir: "documents"
    title: "Embedded"
    timezone: "UTC"
    private: false
    disable_comments: false
    max_versions: 10
    max_upload_size: 10
    language: en
users:
    - username: owner
      password: "$2y$10$3wWHYSETQgNXITWyWboHQ.2WI8.UWrLzH8aIPoCygj//s0CItTsoC"
      role: admin
access_rules:
`

// newWiki writes a config under a temporary folder and returns the folder's
// data directory and the config path.
func newWiki(t *testing.T) (dataDir, configFile string) {
	t.Helper()
	dir := t.TempDir()
	dataDir = filepath.Join(dir, "data")
	configFile = filepath.Join(dir, "config.yaml")
	content := strings.Replace(testConfig, "%q", `"`+dataDir+`"`, 1)
	if err := os.WriteFile(configFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dataDir, configFile
}

func writePage(t *testing.T, dataDir, slug, markdown string) {
	t.Helper()
	dir := filepath.Join(dataDir, "documents", filepath.FromSlash(slug))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "document.md"), []byte(markdown), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, srv *httptest.Server, path string, header http.Header) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body)
}

func TestNewNeedsAConfigFile(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("New with no ConfigFile succeeded")
	}
}

func TestOwnerUserMustBeConfigured(t *testing.T) {
	_, cfg := newWiki(t)
	if _, err := New(Options{ConfigFile: cfg, OwnerUser: "nobody"}); err == nil {
		t.Fatal("New accepted an OwnerUser that is not in the config")
	}
}

func TestWithoutOwnerUserTheWikiNeedsALogin(t *testing.T) {
	_, cfg := newWiki(t)
	h, err := New(Options{ConfigFile: cfg})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	if code, _ := get(t, srv, "/api/check-auth", nil); code != http.StatusUnauthorized {
		t.Errorf("check-auth without a session = %d, want 401", code)
	}
}

func TestOwnerUserIsSignedIn(t *testing.T) {
	_, cfg := newWiki(t)
	h, err := New(Options{ConfigFile: cfg, OwnerUser: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	code, body := get(t, srv, "/api/check-auth", nil)
	if code != http.StatusOK || !strings.Contains(body, `"admin"`) {
		t.Errorf("check-auth = %d %s, want 200 with the admin role", code, body)
	}

	// A stale session cookie must not shadow the owner's.
	stale := http.Header{"Cookie": []string{"session_token=stale; session_user=someone"}}
	if code, _ := get(t, srv, "/api/check-auth", stale); code != http.StatusOK {
		t.Errorf("check-auth with a stale session = %d, want 200", code)
	}
}

func TestServesPagesWrittenFromOutside(t *testing.T) {
	dataDir, cfg := newWiki(t)
	h, err := New(Options{ConfigFile: cfg})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	// The wiki is already running when the page file appears.
	writePage(t, dataDir, "inbox/note", "# Hello embed\n\nA note from outside.\n")

	code, body := get(t, srv, "/inbox/note", nil)
	if code != http.StatusOK || !strings.Contains(body, "Hello embed") {
		t.Errorf("GET /inbox/note = %d, want 200 with the page text", code)
	}
}

func TestCommentsLandOnDisk(t *testing.T) {
	dataDir, cfg := newWiki(t)
	writePage(t, dataDir, "inbox/note", "# Hello embed\n")
	h, err := New(Options{ConfigFile: cfg, OwnerUser: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := srv.Client().Post(srv.URL+"/api/comments/add/inbox/note", "application/json",
		strings.NewReader(`{"content":"Check the date"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add comment = %d, want 200", resp.StatusCode)
	}

	files, err := filepath.Glob(filepath.Join(dataDir, "comments", "inbox", "note", "*.md"))
	if err != nil || len(files) != 1 {
		t.Fatalf("comment files = %v (err %v), want exactly one", files, err)
	}
	text, err := os.ReadFile(files[0])
	if err != nil || strings.TrimSpace(string(text)) != "Check the date" {
		t.Errorf("comment file = %q (err %v)", text, err)
	}
}

// do sends a request to srv with the given method, host and headers. An empty
// host keeps the server's own address.
func do(t *testing.T, srv *httptest.Server, method, path, host string, header http.Header, body string) int {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		req.Host = host
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestOwnerModeAnswersOnlyOnALoopbackHost(t *testing.T) {
	_, cfg := newWiki(t)
	h, err := New(Options{ConfigFile: cfg, OwnerUser: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	// A DNS-rebinding page reaches the server under its own host name.
	if code := do(t, srv, http.MethodGet, "/api/check-auth", "attacker.example.com", nil, ""); code != http.StatusForbidden {
		t.Errorf("foreign host = %d, want 403", code)
	}
	for _, host := range []string{"localhost:8484", "127.0.0.1:8484", "[::1]:8484", "localhost"} {
		if code := do(t, srv, http.MethodGet, "/api/check-auth", host, nil, ""); code != http.StatusOK {
			t.Errorf("host %q = %d, want 200", host, code)
		}
	}
}

func TestOwnerModeRefusesWritesFromAnotherSite(t *testing.T) {
	dataDir, cfg := newWiki(t)
	writePage(t, dataDir, "inbox/note", "# Hello embed\n")
	h, err := New(Options{ConfigFile: cfg, OwnerUser: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	const path = "/api/comments/add/inbox/note"
	const body = `{"content":"from elsewhere"}`

	foreign := http.Header{"Origin": []string{"https://example.com"}}
	if code := do(t, srv, http.MethodPost, path, "", foreign, body); code != http.StatusForbidden {
		t.Errorf("foreign Origin = %d, want 403", code)
	}
	crossSite := http.Header{"Sec-Fetch-Site": []string{"cross-site"}}
	if code := do(t, srv, http.MethodPost, path, "", crossSite, body); code != http.StatusForbidden {
		t.Errorf("Sec-Fetch-Site cross-site = %d, want 403", code)
	}

	// The wiki's own pages and programs are still allowed.
	own := http.Header{"Origin": []string{srv.URL}, "Sec-Fetch-Site": []string{"same-origin"}}
	if code := do(t, srv, http.MethodPost, path, "", own, body); code != http.StatusOK {
		t.Errorf("same-origin write = %d, want 200", code)
	}
	if code := do(t, srv, http.MethodPost, path, "", nil, body); code != http.StatusOK {
		t.Errorf("write with no browser headers = %d, want 200", code)
	}

	// Reads are not refused, whatever the Origin.
	if code := do(t, srv, http.MethodGet, "/inbox/note", "", foreign, ""); code != http.StatusOK {
		t.Errorf("read with a foreign Origin = %d, want 200", code)
	}
}

func TestMountsUnderAnotherRouter(t *testing.T) {
	dataDir, cfg := newWiki(t)
	writePage(t, dataDir, "inbox/note", "# Hello embed\n")
	h, err := New(Options{ConfigFile: cfg})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ping", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.Handle("/", h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if code, body := get(t, srv, "/v1/ping", nil); code != http.StatusOK || body != "pong" {
		t.Errorf("GET /v1/ping = %d %q, want 200 pong from the outer router", code, body)
	}
	if code, body := get(t, srv, "/inbox/note", nil); code != http.StatusOK || !strings.Contains(body, "Hello embed") {
		t.Errorf("GET /inbox/note = %d, want 200 from the wiki", code)
	}
}
