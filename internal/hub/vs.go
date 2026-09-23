package hub

import (
	"embed"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// vsFS holds the comparison pages served under /vs, one Markdown file per
// page plus index.md. They go through the same renderer as the docs.
//
//go:embed vs/*.md
var vsFS embed.FS

// vsPage is one "Deckhand vs X" page. Slug is the path under /vs and the
// Markdown file name; Nav is the short label in the page menu.
type vsPage struct {
	Slug, Nav, Title, Description string
}

var vsPages = []vsPage{
	{Slug: "reveal-js", Nav: "reveal.js", Title: "Deckhand vs reveal.js",
		Description: "HTML slides both ways: reveal.js as a framework with themes, plugins and PDF export, Deckhand as one binary that presents a folder of HTML files with a phone remote and a live audience link."},
	{Slug: "slidev", Nav: "Slidev", Title: "Deckhand vs Slidev",
		Description: "Slidev writes slides in Markdown on Vue and Vite with exports and a presenter mode; Deckhand presents HTML you already have, offline on the LAN, with a phone remote and no toolchain."},
	{Slug: "google-slides", Nav: "Google Slides", Title: "Deckhand vs Google Slides",
		Description: "Google Slides is a collaborative editor with audience Q&A; Deckhand presents HTML files you own, from one binary, with the audience following live on their own screens without an account."},
}

var vsIndexPage = vsPage{
	Title:       "Deckhand compared with reveal.js, Slidev and Google Slides",
	Description: "Where Deckhand fits next to reveal.js, Slidev and Google Slides, and where each of them is the better choice.",
}

// seo is what the layout needs to let a page be indexed: a description, a
// canonical URL and the Open Graph tags. Pages without it carry noindex.
type seo struct {
	Description, Canonical string
}

func (h *Hub) vsIndex(w http.ResponseWriter, r *http.Request) {
	h.vsRender(w, r, "index", vsIndexPage)
}

func (h *Hub) vsPage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	for _, p := range vsPages {
		if p.Slug == slug {
			h.vsRender(w, r, slug, p)
			return
		}
	}
	h.notFound(w, r, "No such comparison.")
}

func (h *Hub) vsRender(w http.ResponseWriter, r *http.Request, file string, p vsPage) {
	src, err := vsFS.ReadFile("vs/" + file + ".md")
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	body, err := renderMarkdown(src)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	canonical := h.cfg.BaseURL + "/vs"
	if p.Slug != "" {
		canonical += "/" + p.Slug
	}
	h.renderPublic(w, r, "vs.html", map[string]any{
		"Title": p.Title, "Slug": p.Slug, "Pages": vsPages, "Body": body,
		"SEO": seo{Description: p.Description, Canonical: canonical},
	})
}
