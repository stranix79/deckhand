// Package ui serves the embedded screens (web/) and the deck files, and
// wires the WebSocket endpoint. It is shared by the local server and the hub.
package ui

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/stranix79/deckhand/internal/deck"
	"github.com/stranix79/deckhand/internal/session"
	"github.com/stranix79/deckhand/web"
)

// Server holds what the handlers need.
type Server struct {
	// Lookup resolves a session code. The local server uses the Manager; the
	// hub adds a database lookup for hosted presentations.
	Lookup func(code string) *session.Session

	// DeckOrigin is the origin deck files are served from when it differs
	// from the app ("https://decks.example.com"). Empty = same origin. It is
	// added to the CSP frame-src so the stage may embed the slides.
	DeckOrigin string

	// AcceptOrigins are extra WebSocket origins (hub: the app origin itself is
	// always accepted by the library; this is for tests and proxies).
	AcceptOrigins []string
}

// AppRoutes mounts the pages, static assets, WebSocket and QR endpoints.
func (u *Server) AppRoutes(r chi.Router) {
	r.Get("/static/*", u.static)
	r.Get("/s/{code}", u.page("stage/index.html"))
	r.Get("/r/{code}", u.page("remote/index.html"))
	r.Get("/v/{code}", u.page("viewer/index.html"))
	r.Get("/ws/{code}", u.ws)
	r.Get("/qr/{code}/viewer.png", u.qrViewer)
	r.Get("/manifest/{code}.json", u.manifest)
}

// DeckRoutes mounts the deck file server. On the hub it is mounted on the
// deck origin only.
func (u *Server) DeckRoutes(r chi.Router) {
	r.Get("/deck/{code}/*", u.deckFile)
}

// csp is the strict policy of the app pages. Slides live in iframes from
// frame-src; everything else is same-origin, no inline scripts.
func (u *Server) csp() string {
	frame := "'self'"
	if u.DeckOrigin != "" {
		frame += " " + u.DeckOrigin
	}
	return strings.Join([]string{
		"default-src 'none'",
		"script-src 'self'",
		"style-src 'self'",
		"img-src 'self' data: blob:",
		"font-src 'self'",
		"connect-src 'self' ws: wss:",
		"frame-src " + frame,
		"manifest-src 'self'",
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors 'self'",
	}, "; ")
}

func (u *Server) page(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := strings.ToUpper(chi.URLParam(r, "code"))
		if !session.ValidCode(code) {
			http.NotFound(w, r)
			return
		}
		b, err := web.FS.ReadFile(file)
		if err != nil {
			http.Error(w, "missing screen", http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Security-Policy", u.csp())
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	}
}

func (u *Server) static(w http.ResponseWriter, r *http.Request) {
	name := path.Clean("/" + chi.URLParam(r, "*"))
	f, err := web.FS.Open(strings.TrimPrefix(name, "/"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	if ct := deck.ContentType(name); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	rs, ok := f.(interface {
		fs.File
		Seek(int64, int) (int64, error)
	})
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, rs)
}

func (u *Server) lookup(w http.ResponseWriter, r *http.Request) *session.Session {
	code := strings.ToUpper(chi.URLParam(r, "code"))
	s := u.Lookup(code)
	if s == nil {
		http.Error(w, "no such session", http.StatusNotFound)
		return nil
	}
	return s
}

func (u *Server) ws(w http.ResponseWriter, r *http.Request) {
	s := u.lookup(w, r)
	if s == nil {
		return
	}
	s.ServeWS(w, r, u.AcceptOrigins)
}

// deckFile serves one file of the deck with the content type of its
// extension. SVG is sent as an attachment unless requested by an <img>
// (brief §2): an SVG opened top-level can run scripts, an <img> cannot.
func (u *Server) deckFile(w http.ResponseWriter, r *http.Request) {
	s := u.lookup(w, r)
	if s == nil {
		return
	}
	rel := path.Clean("/" + chi.URLParam(r, "*"))
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || strings.HasPrefix(rel, "../") || rel == ".." {
		http.NotFound(w, r)
		return
	}
	ct := deck.ContentType(rel)
	if ct == "" {
		http.NotFound(w, r)
		return
	}
	f, err := s.Deck.FS().Open(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-cache")
	if strings.HasPrefix(ct, "text/html") {
		// Belt and braces: even opened outside the stage, a slide runs sandboxed.
		h.Set("Content-Security-Policy", "sandbox allow-scripts")
	}
	if strings.HasPrefix(ct, "image/svg") && r.Header.Get("Sec-Fetch-Dest") != "image" {
		h.Set("Content-Disposition", "attachment")
	}
	rs, ok := f.(interface {
		fs.File
		Seek(int64, int) (int64, error)
	})
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, rel, st.ModTime(), rs)
}

// qrViewer renders the public link as a PNG for the stage overlay and the remote.
func (u *Server) qrViewer(w http.ResponseWriter, r *http.Request) {
	s := u.lookup(w, r)
	if s == nil {
		return
	}
	png, err := qrcode.Encode(s.ViewerURL, qrcode.Medium, 512)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// manifest is the PWA manifest of the remote, with a start_url that keeps
// the token so the home-screen icon opens a working remote.
func (u *Server) manifest(w http.ResponseWriter, r *http.Request) {
	s := u.lookup(w, r)
	if s == nil {
		return
	}
	tok := r.URL.Query().Get("t")
	if tok != s.Token {
		http.Error(w, "invalid token", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-store")
	m := map[string]any{
		"name":             s.Deck.Title + " · Remote",
		"short_name":       "Deckhand",
		"start_url":        "/r/" + s.Code + "?t=" + tok,
		"display":          "standalone",
		"orientation":      "portrait",
		"background_color": "#12233B",
		"theme_color":      "#12233B",
		"icons": []map[string]string{
			{"src": "/static/remote/icon-192.png", "sizes": "192x192", "type": "image/png"},
			{"src": "/static/remote/icon-512.png", "sizes": "512x512", "type": "image/png"},
		},
	}
	_ = json.NewEncoder(w).Encode(m)
}
