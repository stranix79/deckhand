package hub

import (
	"bytes"
	"html/template"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	"github.com/stranix79/deckhand/docs"
	"github.com/stranix79/deckhand/site"
	"github.com/stranix79/deckhand/web"
)

// docPages are the docs rendered under /docs/{name}, in menu order.
var docPages = []string{"EXAMPLES", "FORMAT", "CLI", "HUB", "PROTOCOL", "SECURITY"}

func (h *Hub) siteRoutes(r chi.Router) {
	r.Get("/", h.landing)
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/docs/FORMAT", http.StatusFound) })
	r.Get("/docs/{name}", h.docPage)
	r.Get("/changelog", h.changelog)
	r.Get("/static/site/og.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(site.OG)
	})
	r.Get("/static/hub/hub.css", func(w http.ResponseWriter, _ *http.Request) {
		b, _ := web.FS.ReadFile("hub/hub.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(b)
	})
}

// landing serves site/index.html as provided (brief §8), with the CSP that
// lets it load its Google Fonts and run its inline language switch.
func (h *Hub) landing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", pageCSP)
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(site.Index)
}

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

func renderMarkdown(src []byte) (template.HTML, error) {
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return "", err
	}
	// Links between docs: FORMAT.md → /docs/FORMAT.
	out := buf.String()
	for _, n := range docPages {
		out = strings.ReplaceAll(out, `href="`+n+`.md`, `href="/docs/`+n)
	}
	return template.HTML(out), nil //nolint:gosec // our own Markdown, not user input
}

func (h *Hub) docPage(w http.ResponseWriter, r *http.Request) {
	name := strings.ToUpper(strings.TrimSuffix(chi.URLParam(r, "name"), ".md"))
	src, err := docs.FS.ReadFile(name + ".md")
	if err != nil {
		h.notFound(w, r, "No such document.")
		return
	}
	body, err := renderMarkdown(src)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.render(w, r, "doc.html", map[string]any{"Title": name, "Name": name, "Docs": docPages, "Body": body})
}

// changelog renders CHANGELOG.md from the working directory at build time is
// not possible (it lives at the repo root), so it is embedded via docs/:
// the Makefile copies it there before building.
func (h *Hub) changelog(w http.ResponseWriter, r *http.Request) {
	src, err := docs.FS.ReadFile("CHANGELOG.md")
	if err != nil {
		if b, e := os.ReadFile("CHANGELOG.md"); e == nil {
			src = b
		} else {
			h.notFound(w, r, "No changelog.")
			return
		}
	}
	body, err := renderMarkdown(src)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.render(w, r, "doc.html", map[string]any{"Title": "Changelog", "Name": "CHANGELOG", "Docs": docPages, "Body": body})
}
