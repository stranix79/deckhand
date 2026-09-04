package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/stranix79/deckhand/internal/deck"
	"github.com/stranix79/deckhand/internal/session"
)

func newTestServer(t *testing.T) (*httptest.Server, *session.Session) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"1-a.html":         "<html>a</html>",
		"2-b.html":         "<html>b</html>",
		"assets/logo.svg":  "<svg/>",
		"assets/style.css": "body{}",
	}
	for n, c := range files {
		p := filepath.Join(root, filepath.FromSlash(n))
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A file with a forbidden extension sneaked in after validation must not be served.
	d, err := deck.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "secret.exe"), []byte("MZ"), 0o600)
	s := session.New(d)
	s.ViewerURL = "http://example.test/v/" + s.Code
	m := session.NewManager()
	m.Add(s)
	u := &Server{Lookup: m.Get, DeckOrigin: "https://decks.example.test"}
	r := chi.NewRouter()
	u.AppRoutes(r)
	u.DeckRoutes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	t.Cleanup(s.Close)
	return srv, s
}

func get(t *testing.T, url string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestPagesAndCSP(t *testing.T) {
	srv, s := newTestServer(t)
	for _, p := range []string{"/s/", "/r/", "/v/"} {
		resp := get(t, srv.URL+p+s.Code, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: %d", p, resp.StatusCode)
		}
		csp := resp.Header.Get("Content-Security-Policy")
		if !strings.Contains(csp, "frame-src 'self' https://decks.example.test") || !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe-inline") {
			t.Fatalf("%s: weak CSP %q", p, csp)
		}
	}
	if resp := get(t, srv.URL+"/s/NOPE", nil); resp.StatusCode != 404 {
		t.Fatalf("bad code should be 404, got %d", resp.StatusCode)
	}
	if resp := get(t, srv.URL+"/static/shared/ws.js", nil); resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("static: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if resp := get(t, srv.URL+"/static/../embed.go", nil); resp.StatusCode == 200 {
		t.Fatal("static must not escape web/")
	}
}

func TestDeckFiles(t *testing.T) {
	srv, s := newTestServer(t)
	base := srv.URL + "/deck/" + s.Code + "/"

	resp := get(t, base+"1-a.html", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Security-Policy") != "sandbox allow-scripts" {
		t.Fatalf("slide: %d csp=%q", resp.StatusCode, resp.Header.Get("Content-Security-Policy"))
	}
	if resp := get(t, base+"assets/style.css", nil); !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/css") {
		t.Fatalf("css content type: %q", resp.Header.Get("Content-Type"))
	}
	// SVG: attachment unless fetched as an image.
	if resp := get(t, base+"assets/logo.svg", nil); resp.Header.Get("Content-Disposition") != "attachment" {
		t.Fatalf("svg top-level must be an attachment, got %q", resp.Header.Get("Content-Disposition"))
	}
	if resp := get(t, base+"assets/logo.svg", map[string]string{"Sec-Fetch-Dest": "image"}); resp.Header.Get("Content-Disposition") != "" {
		t.Fatal("svg in <img> must be inline")
	}
	for _, bad := range []string{"secret.exe", "../../etc/passwd", "%2e%2e/1-a.html", "missing.html"} {
		if resp := get(t, base+bad, nil); resp.StatusCode == 200 {
			t.Fatalf("%s must not be served", bad)
		}
	}
	if resp := get(t, srv.URL+"/deck/ZZZZZZ/1-a.html", nil); resp.StatusCode != 404 {
		t.Fatal("unknown session must be 404")
	}
}

func TestQRAndManifest(t *testing.T) {
	srv, s := newTestServer(t)
	if resp := get(t, srv.URL+"/qr/"+s.Code+"/viewer.png", nil); resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("qr: %d", resp.StatusCode)
	}
	if resp := get(t, srv.URL+"/manifest/"+s.Code+".json", nil); resp.StatusCode != 403 {
		t.Fatal("manifest without token must be refused")
	}
	if resp := get(t, srv.URL+"/manifest/"+s.Code+".json?t="+s.Token, nil); resp.StatusCode != 200 {
		t.Fatal("manifest with token")
	}
}
