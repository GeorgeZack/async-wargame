package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"asyncwargame/content"
	"asyncwargame/internal/game"
)

func serverWith(t *testing.T, opts Options) (*Server, *httptest.Server) {
	t.Helper()
	cat, err := game.LoadCatalog(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	store, err := game.OpenStore(filepath.Join(t.TempDir(), "state.json"), cat)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(store, opts)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, ts
}

func TestHostPassword(t *testing.T) {
	_, ts := serverWith(t, Options{HostPassword: "lantern"})
	b := newBrowser(t, ts)
	_, body := b.get("/")
	mustContain(t, body, "Host password")
	form := url.Values{"campaign": {"Lantern"}, "scenario": {"dark-coast-rescue"}, "name": {"Ref"}, "password": {"wrong"}}
	if res, _ := b.post("/rooms", form); res.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong password: %d", res.StatusCode)
	}
	form.Set("password", "lantern")
	if res, _ := b.post("/rooms", form); res.StatusCode != http.StatusOK || !strings.Contains(res.Request.URL.Path, "/r/") {
		t.Fatalf("right password: %d %s", res.StatusCode, res.Request.URL)
	}
	// Ten wrong guesses lock the address out, even for the right password.
	for range 10 {
		form.Set("password", "guess")
		b.post("/rooms", form)
	}
	form.Set("password", "lantern")
	if res, _ := b.post("/rooms", form); res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("after ten guesses: %d", res.StatusCode)
	}
}

func TestSecureCookiesBehindProxy(t *testing.T) {
	for _, trust := range []bool{false, true} {
		srv, _ := serverWith(t, Options{TrustProxy: trust})
		req := httptest.NewRequest("POST", "/rooms", strings.NewReader(url.Values{"campaign": {"Lantern"}, "scenario": {"dark-coast-rescue"}, "name": {"Ref"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("Origin", "http://example.com")
		req.Host = "example.com"
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		cookie := rec.Header().Get("Set-Cookie")
		if cookie == "" {
			t.Fatalf("trust %v: no cookie (%d)", trust, rec.Code)
		}
		if got := strings.Contains(cookie, "Secure"); got != trust {
			t.Errorf("trust %v: Secure %v in %q", trust, got, cookie)
		}
	}
	srv := &Server{opts: Options{TrustProxy: true}}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9")
	if ip := srv.clientIP(req); ip != "203.0.113.9" {
		t.Errorf("client IP %q", ip)
	}
}
