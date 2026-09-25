package hub

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	"github.com/stranix79/deckhand"
	"github.com/stranix79/deckhand/docs"
	"github.com/stranix79/deckhand/site"
	"github.com/stranix79/deckhand/web"
)

// docPages are the docs rendered under /docs/{name}, in menu order.
var docPages = []string{"EXAMPLES", "FORMAT", "LLM", "CLI", "HUB", "PROTOCOL", "SECURITY"}

func (h *Hub) siteRoutes(r chi.Router) {
	r.Get("/", h.landing)
	r.Post("/newsletter", h.newsletterSubscribe)
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/docs/FORMAT", http.StatusFound) })
	r.Get("/docs/{name}", h.docPage)
	r.Get("/changelog", h.changelog)
	r.Get("/vs", h.vsIndex)
	r.Get("/vs/{slug}", h.vsPage)
	h.blogRoutes(r)
	r.Get("/sitemap.xml", h.sitemap)
	r.Get("/robots.txt", h.robots)
	r.Get("/llms.txt", h.llmsTxt)
	r.Get("/llms-full.txt", h.llmsFullTxt)
	r.Get("/"+indexNowKey+".txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(indexNowKey))
	})
	r.Get("/static/site/og.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(site.OG)
	})
	r.Get("/static/site/demo.mp4", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeContent(w, r, "demo.mp4", time.Time{}, bytes.NewReader(site.Demo))
	})
	r.Get("/static/site/demo-poster.jpg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(site.Poster)
	})
	r.Get("/static/hub/site.css", func(w http.ResponseWriter, _ *http.Request) {
		b, _ := web.FS.ReadFile("hub/site.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(b)
	})
	r.Get("/static/hub/hub.css", func(w http.ResponseWriter, _ *http.Request) {
		b, _ := web.FS.ReadFile("hub/hub.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(b)
	})
}

// landing serves site/index.html as provided (brief §8), with the CSP that
// lets it load its Google Fonts and run its inline language switch. With
// DECKHAND_ANALYTICS_ID set, the gtag.js snippet is inserted before </head>
// at request time (index.html itself stays untouched) and the CSP opens the
// Google Analytics hosts; this is the only page that ever loads it.
func (h *Hub) landing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	// The newsletter form carries a signed timestamp (see newsletter.go);
	// with the 5 minute cache it is always at least a few seconds old when
	// a human posts it, which is exactly the point.
	page := bytes.ReplaceAll(site.Index, []byte(newsletterPlaceholder),
		[]byte(newsletterToken(h.cfg.Secret, time.Now())))
	// Structured data for search engines goes in the head too, the same
	// way; both CSPs allow inline scripts so it needs no nonce.
	head := h.landingJSONLD()
	if h.cfg.AnalyticsID == "" {
		w.Header().Set("Content-Security-Policy", pageCSP)
	} else {
		w.Header().Set("Content-Security-Policy", landingAnalyticsCSP)
		head += analyticsSnippet(h.cfg.AnalyticsID)
	}
	_, _ = w.Write(bytes.Replace(page, []byte("</head>"), []byte(head+"</head>"), 1))
}

// landingJSONLD is the schema.org description of Deckhand as a software
// application, for the rich results of search engines. Marshalled with
// encoding/json, which escapes < > & so the block cannot close its own
// script tag whatever the strings contain.
func (h *Hub) landingJSONLD() string {
	data := map[string]any{
		"@context":            "https://schema.org",
		"@type":               "SoftwareApplication",
		"name":                "Deckhand",
		"url":                 h.cfg.BaseURL + "/",
		"description":         "Deckhand turns a folder of HTML pages into a presentation: a stage on the big screen, a remote on your phone, and a live link for everyone in the room. One Go binary, MIT.",
		"applicationCategory": "PresentationApplication",
		"operatingSystem":     "macOS, Linux, Windows",
		"license":             "https://opensource.org/license/mit",
		"codeRepository":      "https://github.com/stranix79/deckhand",
		"image":               h.cfg.BaseURL + "/static/site/og.png",
		"screenshot":          h.cfg.BaseURL + "/static/site/og.png",
		"author": map[string]any{
			"@type": "Person",
			"name":  "Gilles Fauvie",
			"url":   "https://stranix.net",
		},
		"offers": map[string]any{
			"@type":         "Offer",
			"price":         "0",
			"priceCurrency": "EUR",
			"description":   "Self-hosted, free and open source.",
		},
	}
	b, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return `<script type="application/ld+json">` + string(b) + "</script>\n"
}

// indexNowKey is the IndexNow site key; the file at /<key>.txt proves to
// Bing and the other IndexNow engines that we own the host.
const indexNowKey = "0b1e3f2aadf66f64493e0974b540006c"

// llmsTxt answers the llms.txt convention (llmstxt.org): a short Markdown
// index of the site for language models, built from the same routes as
// the sitemap so it never lists a page that does not exist.
func (h *Hub) llmsTxt(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(h.llmsIndex()))
}

// llmsFullTxt is llms.txt followed by the full text of docs/LLM.md, the
// same embed that /docs/LLM renders: the prompt and the rules a model needs
// to write a valid deck without fetching anything else.
func (h *Hub) llmsFullTxt(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	var b strings.Builder
	b.WriteString(h.llmsIndex())
	if src, err := docs.FS.ReadFile("LLM.md"); err == nil {
		b.WriteString("\n---\n\n")
		b.Write(src)
	}
	_, _ = w.Write([]byte(b.String()))
}

func (h *Hub) llmsIndex() string {
	u := h.cfg.BaseURL
	var b strings.Builder
	b.WriteString("# Deckhand\n\n")
	b.WriteString("> Present a folder of HTML slides: a stage on the big screen, a remote on your phone, a live link for the audience. One Go binary, MIT.\n\n")
	b.WriteString("A deck is one HTML file per slide, optionally a deck.json for order, notes and ratio. `deckhand present ./deck` serves the stage, the remote and the viewers on the local network; the hub at " + u + " adds viewers anywhere and permanent links.\n\n")
	b.WriteString("## Docs\n\n")
	b.WriteString("- [Generate a deck with an LLM](" + u + "/docs/LLM): prompt that produces a deck passing deckhand validate\n")
	for _, n := range docPages {
		if n == "LLM" {
			continue
		}
		b.WriteString("- [" + n + "](" + u + "/docs/" + n + "): " + docDescriptions[n] + "\n")
	}
	b.WriteString("- [Changelog](" + u + "/changelog)\n")
	b.WriteString("- [Comparisons](" + u + "/vs)\n")
	for _, p := range vsPages {
		b.WriteString("- [" + p.Title + "](" + u + "/vs/" + p.Slug + ")\n")
	}
	b.WriteString("- [Blog](" + u + "/blog), [in French](" + u + "/blog/fr), [RSS](" + u + "/blog/feed.xml)\n\n")
	b.WriteString("## Source\n\n")
	b.WriteString("- [GitHub](https://github.com/stranix79/deckhand): Go, MIT (hub under BSL 1.1)\n")
	b.WriteString("- [Homebrew tap](https://github.com/stranix79/homebrew-tap): brew install stranix79/tap/deckhand\n\n")
	b.WriteString("## Pricing\n\n")
	b.WriteString("- CLI and self-hosted hub: free\n")
	b.WriteString("- Deckhand Hub Pro at " + u + ": from 9 EUR per month, viewers outside the room, permanent links, audience stats\n\n")
	b.WriteString("## Author\n\n")
	b.WriteString("- Gilles Fauvie, https://stranix.net\n")
	b.WriteString("- CODE79, https://code79.com\n")
	return b.String()
}

// landingAnalyticsCSP is pageCSP plus what gtag.js needs: its script host,
// the collection endpoints (fetch/beacon) and the fallback tracking pixel.
const landingAnalyticsCSP = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' https://www.googletagmanager.com; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src 'self' https://fonts.gstatic.com; " +
	"img-src 'self' data: https://*.google-analytics.com https://*.googletagmanager.com; " +
	"frame-src 'self'; " +
	"connect-src 'self' https://*.google-analytics.com https://*.analytics.google.com https://*.googletagmanager.com; " +
	"base-uri 'self'; form-action 'self'; frame-ancestors 'self'"

// analyticsSnippet is the standard GA4 tag with IP anonymisation left to
// Google's defaults; the ID is escaped so a malformed env value cannot
// break out of the attribute or the script.
func analyticsSnippet(id string) string {
	id = template.JSEscapeString(template.HTMLEscapeString(id))
	return `<script async src="https://www.googletagmanager.com/gtag/js?id=` + id + `"></script>` +
		`<script>window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}` +
		`gtag('js',new Date());gtag('config','` + id + `');</script>` + "\n"
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
	h.render(w, r, "doc.html", map[string]any{"Title": name, "Name": name, "Docs": docPages, "Body": body,
		"SEO": seo{Description: docDescriptions[name], Canonical: h.cfg.BaseURL + "/docs/" + name}})
}

// docDescriptions feed the meta description of each doc page; a doc without
// one (a file dropped in docs/ later) still renders, with noindex.
var docDescriptions = map[string]string{
	"EXAMPLES": "Example Deckhand decks: the Field notes showcase and the minimal Ship it deck, live on the hub and as source on GitHub.",
	"FORMAT":   "The Deckhand deck format: one HTML file per slide, natural order, optional deck.json with title, ratio and presenter notes, limits, and the postMessage protocol for fragments.",
	"LLM":      "A prompt that makes Claude, ChatGPT or a local model produce a complete Deckhand deck that passes deckhand validate, with the reasons behind each rule and the usual failures.",
	"CLI":      "The deckhand command line: validate, present, login, push, serve, and what each one prints.",
	"HUB":      "The Deckhand Hub: hosted at deckhand.show or self-hosted, plans, configuration and deployment.",
	"PROTOCOL": "The Deckhand wire protocol between stage, remote, viewers and hub.",
	"SECURITY": "How Deckhand isolates untrusted slides: sandboxed iframes, strict CSP, separate deck origin, upload checks.",
}

// changelog renders the repository CHANGELOG.md, embedded by the root
// package (a Go embed cannot reach a parent directory from docs/).
func (h *Hub) changelog(w http.ResponseWriter, r *http.Request) {
	src := deckhand.Changelog
	body, err := renderMarkdown(src)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.render(w, r, "doc.html", map[string]any{"Title": "Changelog", "Name": "CHANGELOG", "Docs": docPages, "Body": body,
		"SEO": seo{Description: "What changed in each Deckhand release.", Canonical: h.cfg.BaseURL + "/changelog"}})
}

// sitemap lists the public pages: landing, docs, changelog, comparisons.
// Deck permalinks are deliberately absent; their owners share them.
func (h *Hub) sitemap(w http.ResponseWriter, _ *http.Request) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, p := range h.publicPaths() {
		b.WriteString("  <url><loc>" + template.HTMLEscapeString(h.cfg.BaseURL+p) + "</loc></url>\n")
	}
	b.WriteString("</urlset>\n")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(b.String()))
}

// publicPaths is every path a crawler should know about, in sitemap order.
func (h *Hub) publicPaths() []string {
	paths := []string{"/"}
	for _, n := range docPages {
		paths = append(paths, "/docs/"+n)
	}
	paths = append(paths, "/changelog", "/vs")
	for _, p := range vsPages {
		paths = append(paths, "/vs/"+p.Slug)
	}
	paths = append(paths, h.blogPaths()...)
	return paths
}

// robots keeps crawlers out of the signed-in app and the live screens, which
// are useless without a session anyway, and points at the sitemap.
func (h *Hub) robots(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte("User-agent: *\nAllow: /\nDisallow: /app\nDisallow: /billing\nDisallow: /login\nDisallow: /auth/\nDisallow: /api/\nDisallow: /s/\nDisallow: /r/\nDisallow: /v/\n\nSitemap: " + h.cfg.BaseURL + "/sitemap.xml\n"))
}
