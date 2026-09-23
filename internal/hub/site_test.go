package hub

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// noindexTag is the exact robots tag the layout emits; the word alone also
// appears in the changelog text, so tests must match the tag.
const noindexTag = `<meta name="robots" content="noindex">`

// newSiteServer serves the public site routes without a database: nothing
// under / , /docs, /changelog or /vs touches it for an anonymous visitor.
func newSiteServer(t *testing.T) *httptest.Server {
	t.Helper()
	h, err := New(Config{BaseURL: "https://h.example", Secret: testSecret}, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Router())
	t.Cleanup(srv.Close)
	return srv
}

func getBody(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec // test server URL
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestComparisonPages(t *testing.T) {
	srv := newSiteServer(t)
	pages := map[string]string{
		"/vs":               "Deckhand compared with reveal.js, Slidev and Google Slides",
		"/vs/reveal-js":     "Deckhand vs reveal.js",
		"/vs/slidev":        "Deckhand vs Slidev",
		"/vs/google-slides": "Deckhand vs Google Slides",
	}
	for path, title := range pages {
		resp, body := getBody(t, srv.URL+path)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("%s: content type %q", path, ct)
		}
		if resp.Header.Get("Content-Security-Policy") != pageCSP {
			t.Fatalf("%s: csp %q", path, resp.Header.Get("Content-Security-Policy"))
		}
		if !strings.Contains(body, "<title>"+title+" · Deckhand</title>") {
			t.Fatalf("%s: title %q missing", path, title)
		}
		if !strings.Contains(body, `<link rel="canonical" href="https://h.example`+path+`">`) {
			t.Fatalf("%s: canonical missing", path)
		}
		if !strings.Contains(body, `og:image" content="https://h.example/static/site/og.png"`) || !strings.Contains(body, `<meta name="description"`) {
			t.Fatalf("%s: open graph / description missing", path)
		}
		if strings.Contains(body, noindexTag) {
			t.Fatalf("%s: must be indexable", path)
		}
		if path != "/vs" && (!strings.Contains(body, "<table>") || !strings.Contains(body, "instead")) {
			t.Fatalf("%s: comparison table or 'when to pick X instead' missing", path)
		}
	}
	if resp, _ := getBody(t, srv.URL+"/vs/keynote"); resp.StatusCode != 404 {
		t.Fatalf("unknown comparison: %d", resp.StatusCode)
	}
	// App pages keep noindex.
	if _, body := getBody(t, srv.URL+"/login"); !strings.Contains(body, noindexTag) {
		t.Fatal("login page must stay noindex")
	}
}

func TestDocPageNames(t *testing.T) {
	srv := newSiteServer(t)
	for _, path := range []string{"/docs/LLM", "/docs/LLM.md", "/docs/llm"} {
		resp, body := getBody(t, srv.URL+path)
		if resp.StatusCode != 200 || !strings.Contains(body, "Generate a deck with an LLM") {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
	}
	if resp, _ := getBody(t, srv.URL+"/docs/NOPE"); resp.StatusCode != 404 {
		t.Fatalf("unknown doc: %d", resp.StatusCode)
	}
}

func TestSitemapAndRobots(t *testing.T) {
	srv := newSiteServer(t)
	resp, body := getBody(t, srv.URL+"/sitemap.xml")
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/xml") {
		t.Fatalf("sitemap: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, loc := range []string{"https://h.example/", "https://h.example/docs/FORMAT", "https://h.example/docs/LLM", "https://h.example/changelog", "https://h.example/vs", "https://h.example/vs/google-slides"} {
		if !strings.Contains(body, "<loc>"+loc+"</loc>") {
			t.Fatalf("sitemap lacks %s:\n%s", loc, body)
		}
	}
	if strings.Contains(body, "/app") || strings.Contains(body, "/d/") {
		t.Fatal("sitemap must not list app pages or deck permalinks")
	}
	// Every listed page must answer 200 and be indexable.
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "<loc>") {
			continue
		}
		path := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(line), "</loc></url>"), "<url><loc>https://h.example")
		r2, b2 := getBody(t, srv.URL+path)
		if r2.StatusCode != 200 || strings.Contains(b2, noindexTag) {
			t.Fatalf("%s: %d indexable=%v", path, r2.StatusCode, !strings.Contains(b2, noindexTag))
		}
	}

	resp, body = getBody(t, srv.URL+"/robots.txt")
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("robots: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !strings.Contains(body, "Sitemap: https://h.example/sitemap.xml") || !strings.Contains(body, "Disallow: /app") {
		t.Fatalf("robots:\n%s", body)
	}
}

func TestDemoVideo(t *testing.T) {
	srv := newSiteServer(t)
	resp, body := getBody(t, srv.URL+"/static/site/demo.mp4")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "video/mp4" || len(body) < 100000 {
		t.Fatalf("demo.mp4: status=%d type=%q len=%d", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/static/site/demo.mp4", nil)
	req.Header.Set("Range", "bytes=0-99")
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusPartialContent || r2.ContentLength != 100 {
		t.Fatalf("range request: status=%d len=%d", r2.StatusCode, r2.ContentLength)
	}
	if resp, body := getBody(t, srv.URL+"/static/site/demo-poster.jpg"); resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" || len(body) < 10000 {
		t.Fatalf("poster: status=%d type=%q len=%d", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
	}
	_, landing := getBody(t, srv.URL+"/")
	if !strings.Contains(landing, `<source src="/static/site/demo.mp4"`) || !strings.Contains(landing, `poster="/static/site/demo-poster.jpg"`) {
		t.Fatal("landing does not embed the demo video")
	}
}
