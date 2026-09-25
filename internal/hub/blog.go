package hub

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// The blog is a handful of Markdown files embedded in the binary, one per
// post and per language: blog/posts/<slug>.en.md and <slug>.fr.md. Each file
// starts with a small front matter block (title, date, summary, image,
// credit). English lives at /blog and /blog/<slug>, French at /blog/fr and
// /blog/fr/<slug>; every page links to its translation with hreflang so
// search engines pair them. Images are served from /static/blog/.
//
//go:embed blog/posts/*.md blog/img/*
var blogFS embed.FS

type blogPost struct {
	Slug, Lang, Title, Summary, Image, Credit string
	Date                                      time.Time
	Body                                      template.HTML
}

// URL is the path of the post in its language.
func (p blogPost) URL() string {
	if p.Lang == "fr" {
		return "/blog/fr/" + p.Slug
	}
	return "/blog/" + p.Slug
}

// DateText is the date the way each language writes it.
func (p blogPost) DateText() string {
	if p.Lang == "fr" {
		months := []string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}
		return fmt.Sprintf("%d %s %d", p.Date.Day(), months[p.Date.Month()-1], p.Date.Year())
	}
	return p.Date.Format("January 2, 2006")
}

// blogIndexURL is the index of a language.
func blogIndexURL(lang string) string {
	if lang == "fr" {
		return "/blog/fr"
	}
	return "/blog"
}

// loadBlog parses every embedded post once, at startup. A malformed post is
// a build error in practice (the tests read them all), so it panics.
func loadBlog() map[string][]blogPost {
	entries, err := blogFS.ReadDir("blog/posts")
	if err != nil {
		panic("blog: " + err.Error())
	}
	byLang := map[string][]blogPost{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		base := strings.TrimSuffix(name, ".md")
		dot := strings.LastIndex(base, ".")
		if dot < 1 {
			panic("blog: post file must be <slug>.<lang>.md, got " + name)
		}
		slug, lang := base[:dot], base[dot+1:]
		src, err := blogFS.ReadFile("blog/posts/" + name)
		if err != nil {
			panic("blog: " + err.Error())
		}
		p, err := parsePost(slug, lang, src)
		if err != nil {
			panic("blog: " + name + ": " + err.Error())
		}
		byLang[lang] = append(byLang[lang], p)
	}
	for lang := range byLang {
		sort.Slice(byLang[lang], func(i, j int) bool { return byLang[lang][i].Date.After(byLang[lang][j].Date) })
	}
	return byLang
}

// parsePost splits the front matter ("---" ... "---", key: value lines) from
// the Markdown body and renders the body.
func parsePost(slug, lang string, src []byte) (blogPost, error) {
	p := blogPost{Slug: slug, Lang: lang}
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return p, fmt.Errorf("missing front matter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return p, fmt.Errorf("unterminated front matter")
	}
	head, body := text[4:4+end], text[4+end+5:]
	for _, line := range strings.Split(head, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "title":
			p.Title = v
		case "summary":
			p.Summary = v
		case "image":
			p.Image = v
		case "credit":
			p.Credit = v
		case "date":
			d, err := time.Parse("2006-01-02", v)
			if err != nil {
				return p, fmt.Errorf("bad date %q", v)
			}
			p.Date = d
		}
	}
	if p.Title == "" || p.Summary == "" || p.Date.IsZero() {
		return p, fmt.Errorf("title, summary and date are required")
	}
	html, err := renderMarkdown([]byte(body))
	if err != nil {
		return p, err
	}
	p.Body = html
	return p, nil
}

func (h *Hub) blogPost(lang, slug string) (blogPost, bool) {
	for _, p := range h.blog[lang] {
		if p.Slug == slug {
			return p, true
		}
	}
	return blogPost{}, false
}

// blogAlternate is the same page in the other language, for hreflang and the
// language switch. Missing translations simply do not link.
type blogAlternate struct{ Lang, URL string }

func (h *Hub) blogRoutes(r chi.Router) {
	r.Get("/blog", func(w http.ResponseWriter, r *http.Request) { h.blogIndex(w, r, "en") })
	r.Get("/blog/fr", func(w http.ResponseWriter, r *http.Request) { h.blogIndex(w, r, "fr") })
	r.Get("/blog/feed.xml", func(w http.ResponseWriter, r *http.Request) { h.blogFeed(w, r, "en") })
	r.Get("/blog/fr/feed.xml", func(w http.ResponseWriter, r *http.Request) { h.blogFeed(w, r, "fr") })
	r.Get("/blog/fr/{slug}", func(w http.ResponseWriter, r *http.Request) { h.blogShow(w, r, "fr", chi.URLParam(r, "slug")) })
	r.Get("/blog/{slug}", func(w http.ResponseWriter, r *http.Request) { h.blogShow(w, r, "en", chi.URLParam(r, "slug")) })
	r.Get("/static/blog/{file}", h.blogImage)
}

func (h *Hub) blogIndex(w http.ResponseWriter, r *http.Request, lang string) {
	title, desc := "Blog", "News from Deckhand: releases, how-tos, and the story behind the tool."
	if lang == "fr" {
		title, desc = "Blog", "Les nouvelles de Deckhand : versions, tutoriels, et l'histoire derrière l'outil."
	}
	other := "fr"
	if lang == "fr" {
		other = "en"
	}
	h.renderSite(w, r, "blog.html", map[string]any{
		"Title": title, "Lang": lang, "Posts": h.blog[lang],
		"Alternates": []blogAlternate{{lang, h.cfg.BaseURL + blogIndexURL(lang)}, {other, h.cfg.BaseURL + blogIndexURL(other)}},
		"LangEN":     blogIndexURL("en"), "LangFR": blogIndexURL("fr"),
		"Feed": blogIndexURL(lang) + "/feed.xml",
		"SEO":  seo{Description: desc, Canonical: h.cfg.BaseURL + blogIndexURL(lang)},
	})
}

func (h *Hub) blogShow(w http.ResponseWriter, r *http.Request, lang, slug string) {
	p, ok := h.blogPost(lang, slug)
	if !ok {
		h.notFound(w, r, "No such post.")
		return
	}
	other := "fr"
	if lang == "fr" {
		other = "en"
	}
	data := map[string]any{
		"Title": p.Title, "Lang": lang, "Post": p,
		"Alternates": []blogAlternate{{lang, h.cfg.BaseURL + p.URL()}},
		"Feed":       blogIndexURL(lang) + "/feed.xml",
		"SEO":        seo{Description: p.Summary, Canonical: h.cfg.BaseURL + p.URL()},
	}
	if p.Image != "" {
		data["OGImage"] = h.cfg.BaseURL + "/static/blog/" + p.Image
	}
	// Language switch: the translation when it exists, the index otherwise.
	data["LangEN"], data["LangFR"] = blogIndexURL("en"), blogIndexURL("fr")
	if o, ok := h.blogPost(other, slug); ok {
		data["Alternates"] = append(data["Alternates"].([]blogAlternate), blogAlternate{other, h.cfg.BaseURL + o.URL()})
		if other == "fr" {
			data["LangFR"] = o.URL()
		} else {
			data["LangEN"] = o.URL()
		}
	}
	if lang == "fr" {
		data["LangFR"] = p.URL()
	} else {
		data["LangEN"] = p.URL()
	}
	// Next = the post published just after this one (the list is newest first).
	posts := h.blog[lang]
	for i := range posts {
		if posts[i].Slug == slug && i > 0 {
			data["Next"] = posts[i-1]
		}
	}
	h.renderSite(w, r, "post.html", data)
}

// blogImage serves the embedded post images (JPEG or PNG by extension).
func (h *Hub) blogImage(w http.ResponseWriter, r *http.Request) {
	name := path.Base(chi.URLParam(r, "file"))
	b, err := blogFS.ReadFile("blog/img/" + name)
	if err != nil {
		h.notFound(w, r, "No such image.")
		return
	}
	ct := "image/jpeg"
	if strings.HasSuffix(name, ".png") {
		ct = "image/png"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b) //nolint:gosec // bytes of an embedded image, chosen by base name only
}

// blogFeed is a minimal RSS 2.0 feed per language, so readers and
// aggregators can follow the blog without a newsletter.
func (h *Hub) blogFeed(w http.ResponseWriter, _ *http.Request, lang string) {
	esc := template.HTMLEscapeString
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom"><channel>` + "\n")
	b.WriteString("<title>Deckhand blog</title>\n<link>" + esc(h.cfg.BaseURL+blogIndexURL(lang)) + "</link>\n")
	b.WriteString("<description>News from Deckhand</description>\n<language>" + lang + "</language>\n")
	b.WriteString(`<atom:link href="` + esc(h.cfg.BaseURL+blogIndexURL(lang)+"/feed.xml") + `" rel="self" type="application/rss+xml"/>` + "\n")
	for _, p := range h.blog[lang] {
		b.WriteString("<item><title>" + esc(p.Title) + "</title><link>" + esc(h.cfg.BaseURL+p.URL()) + "</link><guid>" + esc(h.cfg.BaseURL+p.URL()) + "</guid>")
		b.WriteString("<pubDate>" + p.Date.UTC().Format(time.RFC1123Z) + "</pubDate><description>" + esc(p.Summary) + "</description></item>\n")
	}
	b.WriteString("</channel></rss>\n")
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(b.String()))
}

// blogPaths feeds the sitemap: both indexes and every post in every language.
func (h *Hub) blogPaths() []string {
	paths := []string{"/blog", "/blog/fr"}
	for _, lang := range []string{"en", "fr"} {
		for _, p := range h.blog[lang] {
			paths = append(paths, p.URL())
		}
	}
	return paths
}
