// Package wikigo lets another Go program run wiki-go inside its own process.
//
// The wiki keeps its state in package-level variables, so start one wiki per
// process. Call New once, then mount the returned handler on your own router:
//
//	h, err := wikigo.New(wikigo.Options{ConfigFile: "/data/wiki/config.yaml"})
//	if err != nil {
//		log.Fatal(err)
//	}
//	mux := http.NewServeMux()
//	mux.HandleFunc("/healthz", healthz)
//	mux.Handle("/", h) // the wiki serves everything the mux does not
package wikigo

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"wiki-go/internal/app"
	"wiki-go/internal/auth"
	"wiki-go/internal/config"
	"wiki-go/internal/routes"
)

// Options configures an embedded wiki.
type Options struct {
	// ConfigFile is the path to the wiki's config.yaml. The wiki writes a
	// default file there when none exists, and creates its data folders under
	// the config's root_dir.
	ConfigFile string

	// OwnerUser, when set, signs every request in as this user from the
	// config's users list, so the wiki shows no login screen. Set it only when
	// one trusted person can reach the wiki, for example on a loopback
	// address. A request that carries another valid session keeps it.
	//
	// Because it signs in without a cookie, owner mode also answers only when
	// the Host header names a loopback address, and refuses a POST, PUT,
	// PATCH or DELETE that a browser marks as coming from another site.
	OwnerUser string
}

// New prepares the wiki and returns its handler. It runs the same start-up
// steps as the wiki-go binary, then builds the handler without registering it
// on http.DefaultServeMux.
func New(opts Options) (http.Handler, error) {
	if opts.ConfigFile == "" {
		return nil, errors.New("wikigo: ConfigFile is required")
	}

	cfg, err := app.Init(opts.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("wikigo: %w", err)
	}

	handler := routes.Handler(cfg)
	if opts.OwnerUser == "" {
		return handler, nil
	}

	user, ok := findUser(cfg, opts.OwnerUser)
	if !ok {
		return nil, fmt.Errorf("wikigo: OwnerUser %q is not in the config's users", opts.OwnerUser)
	}
	return &owner{next: handler, cfg: cfg, user: user}, nil
}

func findUser(cfg *config.Config, name string) (config.User, bool) {
	for _, u := range cfg.Users {
		if u.Username == name {
			return u, true
		}
	}
	return config.User{}, false
}

// owner signs requests in as one configured user.
type owner struct {
	next http.Handler
	cfg  *config.Config
	user config.User

	mu      sync.Mutex
	cookies []*http.Cookie
}

func (o *owner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Owner mode signs in without a cookie, so a web page that the owner has
	// open could otherwise drive the wiki through the owner's browser. Answer
	// only on a loopback host (which stops DNS rebinding) and refuse writes
	// that a browser says came from another site.
	if !loopbackHost(r.Host) {
		http.Error(w, "wikigo: owner mode answers only on a loopback host", http.StatusForbidden)
		return
	}
	if unsafeMethod(r.Method) && !sameSite(r) {
		http.Error(w, "wikigo: owner mode refuses a write from another site", http.StatusForbidden)
		return
	}

	if auth.GetSession(r) == nil {
		cookies, err := o.sessionCookies()
		if err != nil {
			http.Error(w, "wikigo: cannot start the owner session", http.StatusInternalServerError)
			return
		}
		replaceSessionCookies(r, cookies)
		for _, c := range cookies {
			http.SetCookie(w, c)
		}
	}
	o.next.ServeHTTP(w, r)
}

// loopbackHost reports whether a Host header names this machine's loopback
// interface: localhost, 127.0.0.0/8 or ::1, with or without a port.
func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func unsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// sameSite reports whether a request may be a write from the wiki's own
// pages or from a program. A browser marks a cross-site request with
// Sec-Fetch-Site or an Origin that differs from the Host. A program sends
// neither header.
func sameSite(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			return false
		}
	}
	return true
}

// sessionCookies returns the cookies of one long-lived owner session, and
// starts a new session when the old one has expired.
func (o *owner) sessionCookies() ([]*http.Cookie, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.valid() {
		return o.cookies, nil
	}

	capture := &cookieCapture{header: http.Header{}}
	err := auth.CreateSession(capture, o.user.Username, o.user.Role, o.user.Groups, true, o.cfg)
	if err != nil {
		return nil, err
	}
	o.cookies = (&http.Response{Header: capture.header}).Cookies()
	return o.cookies, nil
}

func (o *owner) valid() bool {
	if len(o.cookies) == 0 {
		return false
	}
	probe := &http.Request{Header: http.Header{}}
	for _, c := range o.cookies {
		probe.AddCookie(c)
	}
	return auth.GetSession(probe) != nil
}

// replaceSessionCookies puts cookies on r in place of any session cookies it
// already carries, so a stale token cannot shadow the owner's.
func replaceSessionCookies(r *http.Request, cookies []*http.Cookie) {
	var keep []string
	for _, c := range r.Cookies() {
		if c.Name != "session_token" && c.Name != "session_user" {
			keep = append(keep, c.String())
		}
	}
	r.Header.Del("Cookie")
	for _, k := range keep {
		r.Header.Add("Cookie", k)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
}

// cookieCapture is a response writer that only records headers, so a session
// cookie can be read back after auth.CreateSession sets it.
type cookieCapture struct{ header http.Header }

func (c *cookieCapture) Header() http.Header         { return c.header }
func (c *cookieCapture) Write(b []byte) (int, error) { return len(b), nil }
func (c *cookieCapture) WriteHeader(int)             {}
