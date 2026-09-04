package hub

import (
	"strings"
	"testing"
	"time"
)

func TestSlugAndHandle(t *testing.T) {
	cases := map[string]string{"Ship it before lunch!": "ship-it-before-lunch", "": "deck", "---": "deck", "Hello, World 2026": "hello-world-2026"}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
	if h := handleFor("Gilles.Fauvie+x@example.com"); h != "gilles-fauvie-x" {
		t.Errorf("handleFor = %q", h)
	}
	if h := handleFor("a@b.c"); !strings.HasPrefix(h, "user-") {
		t.Errorf("short handle = %q", h)
	}
}

func TestConfigValidate(t *testing.T) {
	c := Config{PG: "postgres://x", BaseURL: "https://h.example", Secret: strings.Repeat("s", 32), MailHost: "smtp"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.DeckOrigin = "https://decks.h.example/path"
	if err := c.Validate(); err == nil {
		t.Fatal("deck origin with a path must be refused")
	}
	c.DeckOrigin = "https://decks.h.example"
	if c.DeckOriginHost() != "decks.h.example" || !c.Secure() {
		t.Fatal("origin host / secure")
	}
	c.Secret = "short"
	if err := c.Validate(); err == nil {
		t.Fatal("short secret accepted")
	}
}

func TestReplayStats(t *testing.T) {
	t0 := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	events := []evt{
		{at: at(1), kind: "join", viewer: "a", slide: 0},
		{at: at(2), kind: "join", viewer: "b", slide: 0},
		{at: at(10), kind: "slide", slide: 1},
		{at: at(12), kind: "leave", viewer: "a", slide: 1},
		{at: at(20), kind: "slide", slide: 2},
		{at: at(25), kind: "join", viewer: "a", slide: 2}, // a comes back: still one unique
		{at: at(30), kind: "slide", slide: 1},             // back to slide 1
	}
	end := at(40)
	st := replay(events, t0, &end)
	if st.Unique != 2 || st.Peak != 2 {
		t.Fatalf("unique/peak: %+v", st)
	}
	if len(st.Slides) != 3 {
		t.Fatalf("slides: %+v", st.Slides)
	}
	// slide 0: 0→10 s, slide 1: 10→20 + 30→40 = 20 s, slide 2: 20→30 = 10 s
	if st.Slides[0].Duration != 10*time.Second || st.Slides[1].Duration != 20*time.Second || st.Slides[2].Duration != 10*time.Second {
		t.Fatalf("durations: %+v", st.Slides)
	}
	if st.Slides[1].Viewers != 2 || st.Slides[2].Viewers != 2 {
		t.Fatalf("viewers per slide: %+v", st.Slides)
	}
	if st.Total != 40*time.Second || st.TotalHuman != "40s" {
		t.Fatalf("total: %v %s", st.Total, st.TotalHuman)
	}
	svg := statsSVG(st)
	if !strings.HasPrefix(svg, "<svg") || strings.Count(svg, "<rect") < 4 {
		t.Fatalf("svg: %s", svg[:80])
	}
	if !strings.Contains(statsSVG(&Stats{}), "No audience data") {
		t.Fatal("empty svg")
	}
}

func TestRenderMarkdownLinks(t *testing.T) {
	out, err := renderMarkdown([]byte("See [the format](FORMAT.md) and `code`."))
	if err != nil || !strings.Contains(string(out), `href="/docs/FORMAT"`) {
		t.Fatalf("markdown: %v %s", err, out)
	}
}
