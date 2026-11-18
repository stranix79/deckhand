package pptx

import (
	_ "embed" // the stylesheet shipped with every imported deck
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/stranix79/deckhand/internal/deck"
)

//go:embed style.css
var stylesheet string

// designWidth is the width imported slides are laid out for. The height
// follows the ratio of the PowerPoint file.
const designWidth = 1920

// Options tune what Write produces.
type Options struct {
	Title string // used when the PowerPoint file has no usable title
	Dark  bool   // dark theme instead of the light one
	Force bool   // write into a directory that is not empty
}

// Write creates the deck in dir: one HTML file per slide, deck.json with the
// presenter notes, and assets/ with the stylesheet, pictures and videos.
// Existing files are only overwritten with Force; nothing is ever deleted.
func (d *Deck) Write(dir string, opt Options) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 && !opt.Force {
		return fmt.Errorf("%s already exists and is not empty (use --force to write into it)", dir)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil { //nolint:gosec // a deck is meant to be read and served
		return err
	}

	height, err := deck.HeightFor(d.Ratio, designWidth)
	if err != nil {
		return err
	}
	css := strings.ReplaceAll(stylesheet, "{{HEIGHT}}", strconv.Itoa(height))
	drawFont := ""
	if d.font != "" {
		drawFont = `"` + d.font + `", `
	}
	css = strings.ReplaceAll(css, "{{DRAWFONT}}", drawFont)
	if err := writeFile(filepath.Join(dir, "assets", "style.css"), []byte(css)); err != nil {
		return err
	}
	for _, a := range d.assets {
		if err := copyAsset(a, filepath.Join(dir, "assets", a.name)); err != nil {
			return err
		}
	}

	title := d.Title
	if title == "" {
		title = opt.Title
	}
	type manifestSlide struct {
		File  string `json:"file"`
		Notes string `json:"notes,omitempty"`
	}
	manifest := struct {
		Title  string          `json:"title"`
		Ratio  string          `json:"ratio"`
		Width  int             `json:"width"`
		Slides []manifestSlide `json:"slides"`
	}{Title: title, Ratio: d.Ratio, Width: designWidth}

	lang := d.Lang
	if lang == "" {
		lang = "en"
	}
	digits := len(strconv.Itoa(len(d.slides)))
	if digits < 2 {
		digits = 2
	}
	for i, s := range d.slides {
		name := fmt.Sprintf("%0*d-%s.html", digits, i+1, slug(s.title))
		page := s.html(lang, opt.Dark, fmt.Sprintf("Slide %d", i+1))
		if err := writeFile(filepath.Join(dir, name), []byte(page)); err != nil {
			return err
		}
		manifest.Slides = append(manifest.Slides, manifestSlide{File: name, Notes: s.notes})
	}

	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, "deck.json"), append(raw, '\n'))
}

func writeFile(name string, data []byte) error {
	return os.WriteFile(name, data, 0o644) //nolint:gosec // a deck is meant to be read and served
}

func copyAsset(a *asset, dst string) error {
	rc, err := a.file.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // a deck is meant to be read and served
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(rc, maxAsset+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxAsset {
		err = fmt.Errorf("%s is larger than %d MB", a.name, maxAsset>>20)
	}
	return err
}

// html renders one slide as a self-contained page.
func (s *slide) html(lang string, dark bool, fallbackTitle string) string {
	title := s.title
	if title == "" {
		title = fallbackTitle
	}
	head := "<!doctype html>\n" +
		`<html lang="` + html.EscapeString(lang) + `"%s>` + "\n" +
		"<head>\n<meta charset=\"utf-8\">\n" +
		"<title>" + escText(title) + "</title>\n" +
		"<link rel=\"stylesheet\" href=\"assets/style.css\">\n</head>\n"
	if s.drawn {
		// A drawing keeps its own colours: the dark theme does not apply.
		return fmt.Sprintf(head, "") + s.canvasHTML() + "</html>\n"
	}

	var flow, media []string
	chars, items := 0, 0
	textOnly := true // nothing but paragraphs: a candidate for a centred slide
	for _, b := range s.blocks {
		if b.kind != blockText {
			textOnly = false
		}
		switch b.kind {
		case blockText:
			flow = append(flow, parasHTML(b.paras, b.lead))
			for _, p := range b.paras {
				chars += len([]rune(plain(p.runs)))
				items++
			}
		case blockTable:
			flow = append(flow, tableHTML(b))
			for _, row := range b.rows {
				items++
				for _, c := range row {
					for _, p := range c.paras {
						chars += len([]rune(plain(p.runs)))
					}
				}
			}
		case blockMissing:
			flow = append(flow, `<div class="missing">`+escText(b.label)+`</div>`)
			items += 2
		case blockImage:
			media = append(media, fmt.Sprintf(`<img src="assets/%s" alt="%s">`, b.asset, html.EscapeString(b.alt)))
		case blockVideo:
			poster := ""
			if b.poster != "" {
				poster = fmt.Sprintf(` poster="assets/%s"`, b.poster)
			}
			media = append(media, fmt.Sprintf(`<video src="assets/%s"%s controls playsinline></video>`, b.asset, poster))
		case blockAudio:
			media = append(media, fmt.Sprintf(`<audio src="assets/%s" controls></audio>`, b.asset))
		}
	}

	// Text next to pictures only gets half the slide: it fills up twice as fast.
	if len(media) > 0 {
		chars *= 2
		items *= 2
	}
	classes := []string{}
	if s.cover || (textOnly && items <= 2 && chars < 160) {
		classes = append(classes, "cover")
	}
	switch {
	case chars > 900 || items > 13:
		classes = append(classes, "denser")
	case chars > 450 || items > 7:
		classes = append(classes, "dense")
	}

	var body strings.Builder
	if s.title != "" {
		tag := "h2"
		if len(classes) > 0 && classes[0] == "cover" {
			tag = "h1"
		}
		fmt.Fprintf(&body, "  <%s>%s</%s>\n", tag, escText(s.title), tag)
	}
	indent := func(parts []string, pad string) string {
		return pad + strings.ReplaceAll(strings.Join(parts, "\n"), "\n", "\n"+pad) + "\n"
	}
	count := "n" + strconv.Itoa(len(media))
	if len(media) > 4 {
		count = "many"
	}
	switch {
	case len(flow) > 0 && len(media) > 0:
		body.WriteString("  <div class=\"cols\">\n    <div class=\"flow\">\n")
		body.WriteString(indent(flow, "      "))
		fmt.Fprintf(&body, "    </div>\n    <div class=\"media %s\">\n", count)
		body.WriteString(indent(media, "      "))
		body.WriteString("    </div>\n  </div>\n")
	case len(media) > 0:
		fmt.Fprintf(&body, "  <div class=\"media only %s\">\n", count)
		body.WriteString(indent(media, "    "))
		body.WriteString("  </div>\n")
	case len(flow) > 0:
		body.WriteString("  <div class=\"flow\">\n")
		body.WriteString(indent(flow, "    "))
		body.WriteString("  </div>\n")
	}

	htmlClass := ""
	if dark {
		htmlClass = ` class="dark"`
	}
	bodyClass := ""
	if len(classes) > 0 {
		bodyClass = ` class="` + strings.Join(classes, " ") + `"`
	}
	return fmt.Sprintf(head, htmlClass) + "<body" + bodyClass + ">\n" + body.String() + "</body>\n</html>\n"
}

// canvasHTML renders a drawing: every element at its place, in the order
// PowerPoint stacked them.
func (s *slide) canvasHTML() string {
	var b strings.Builder
	style := ""
	if s.bg != "" {
		style += "background:" + s.bg + ";"
	}
	if s.fg != "" {
		style += "color:" + s.fg + ";"
	}
	fmt.Fprintf(&b, "<body class=\"canvas\" style=\"%s\">\n", html.EscapeString(strings.TrimSuffix(style, ";")))
	inSVG := false
	closeSVG := func() {
		if inSVG {
			b.WriteString("  </svg>\n")
			inSVG = false
		}
	}
	for _, e := range s.canvas {
		if e.kind == elemLine {
			// Consecutive lines share one <svg> laid over the whole slide.
			if !inSVG {
				b.WriteString("  <svg class=\"ln\">\n")
				inSVG = true
			}
			dash := ""
			if e.dash {
				dash = fmt.Sprintf(` stroke-dasharray="%s %s"`, f1(e.width*4), f1(e.width*3))
			}
			fmt.Fprintf(&b, "    <path d=\"%s\" fill=\"none\" stroke=\"%s\" stroke-width=\"%s\"%s/>\n", e.path, e.stroke, f1(e.width), dash)
			for _, h := range e.heads {
				if h != "" {
					fmt.Fprintf(&b, "    <polygon points=\"%s\" fill=\"%s\"/>\n", h, e.stroke)
				}
			}
			continue
		}
		closeSVG()
		pos := fmt.Sprintf("left:%s;top:%s;width:%s;height:%s", px(e.x), px(e.y), px(e.w), px(e.h))
		if e.rot != 0 {
			pos += ";transform:rotate(" + f1(e.rot) + "deg)"
		}
		switch e.kind {
		case elemShape:
			class := "sh"
			if e.nowrap {
				class += " nw"
			}
			css := pos
			if len(e.style) > 0 {
				css += ";" + strings.Join(e.style, ";")
			}
			if len(e.paras) == 0 {
				fmt.Fprintf(&b, "  <div class=\"%s\" style=\"%s\"></div>\n", class, html.EscapeString(css))
				continue
			}
			fmt.Fprintf(&b, "  <div class=\"%s\" style=\"%s\">%s</div>\n", class, html.EscapeString(css), drawnParasHTML(e.paras))
		case elemBlock:
			switch e.block.kind {
			case blockImage:
				class := "ob"
				if e.block.crop {
					class += " crop"
				}
				fmt.Fprintf(&b, "  <img class=\"%s\" style=\"%s\" src=\"assets/%s\" alt=\"%s\">\n", class, pos, e.block.asset, html.EscapeString(e.block.alt))
			case blockVideo:
				poster := ""
				if e.block.poster != "" {
					poster = fmt.Sprintf(` poster="assets/%s"`, e.block.poster)
				}
				fmt.Fprintf(&b, "  <video class=\"ob\" style=\"%s\" src=\"assets/%s\"%s controls playsinline></video>\n", pos, e.block.asset, poster)
			case blockAudio:
				fmt.Fprintf(&b, "  <audio class=\"ob\" style=\"%s\" src=\"assets/%s\" controls></audio>\n", pos, e.block.asset)
			case blockTable:
				fmt.Fprintf(&b, "  <div class=\"ob\" style=\"%s;font-size:%s\">\n%s\n  </div>\n", pos, px(e.fontPx), tableHTML(e.block))
			case blockMissing:
				fmt.Fprintf(&b, "  <div class=\"ob missing\" style=\"%s\">%s</div>\n", pos, escText(e.block.label))
			}
		}
	}
	closeSVG()
	b.WriteString("</body>\n")
	return b.String()
}

// drawnParasHTML renders the text of a shape: one <p> per paragraph, with
// its own size and alignment, bullets drawn by CSS from data-b.
func drawnParasHTML(paras []para) string {
	var b strings.Builder
	number := 0
	for _, p := range paras {
		css := "font-size:" + px(p.size)
		switch p.align {
		case "ctr":
			css += ";text-align:center"
		case "r":
			css += ";text-align:right"
		case "just":
			css += ";text-align:justify"
		}
		attrs := ""
		indent := float64(p.level) * 1.2
		switch p.bullet {
		case bulletDot:
			number = 0
			attrs = ` class="bu" data-b="•"`
			indent += 1.2
		case bulletNumber:
			number++
			attrs = fmt.Sprintf(` class="bu" data-b="%d."`, number)
			indent += 1.2
		default:
			number = 0
		}
		if indent > 0 {
			css += ";margin-left:" + f1(indent) + "em"
		}
		text := runsHTML(p.runs)
		if text == "" {
			text = "&nbsp;"
		}
		fmt.Fprintf(&b, "<p%s style=\"%s\">%s</p>", attrs, css, text)
	}
	return b.String()
}

func runsHTML(runs []run) string {
	var b strings.Builder
	for _, r := range runs {
		if r.br {
			b.WriteString("<br>")
			continue
		}
		open, closing := "", ""
		wrap := func(on bool, tag string) {
			if on {
				open += "<" + tag + ">"
				closing = "</" + tag + ">" + closing
			}
		}
		if r.href != "" {
			open += `<a href="` + html.EscapeString(r.href) + `">`
			closing = "</a>" + closing
		}
		wrap(r.bold, "strong")
		wrap(r.italic, "em")
		wrap(r.underline, "u")
		wrap(r.strike, "s")
		wrap(r.code, "code")
		if r.style != "" {
			open += `<span style="` + html.EscapeString(r.style) + `">`
			closing = "</span>" + closing
		}
		b.WriteString(open + escText(r.text) + closing)
	}
	return b.String()
}

// textEscaper escapes what must be escaped between tags, and nothing more:
// apostrophes and quotes stay readable in the HTML the author will edit.
var textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escText(s string) string { return textEscaper.Replace(s) }

// item is one entry of a (possibly nested) list.
type item struct {
	html     string
	numbered bool
	children []*item
}

// parasHTML renders paragraphs: plain ones as <p>, runs of bulleted ones as
// nested lists that follow the PowerPoint indent levels.
func parasHTML(paras []para, lead bool) string {
	var out []string
	var roots []*item
	var last []*item // last item seen at each depth of the current list
	flush := func() {
		if len(roots) > 0 {
			out = append(out, listHTML(roots, ""))
			roots, last = nil, nil
		}
	}
	for _, p := range paras {
		if p.bullet == bulletNone {
			flush()
			class := ""
			if lead {
				class = ` class="lead"`
			}
			out = append(out, "<p"+class+">"+runsHTML(p.runs)+"</p>")
			continue
		}
		it := &item{html: runsHTML(p.runs), numbered: p.bullet == bulletNumber}
		depth := p.level
		if depth > len(last) {
			depth = len(last) // a level-2 bullet with no level-1 parent
		}
		if depth == 0 {
			roots = append(roots, it)
		} else {
			parent := last[depth-1]
			parent.children = append(parent.children, it)
		}
		last = append(last[:depth], it)
	}
	flush()
	return strings.Join(out, "\n")
}

func listHTML(items []*item, pad string) string {
	tag := "ul"
	if items[0].numbered {
		tag = "ol"
	}
	var b strings.Builder
	b.WriteString(pad + "<" + tag + ">\n")
	for _, it := range items {
		if len(it.children) == 0 {
			b.WriteString(pad + "  <li>" + it.html + "</li>\n")
			continue
		}
		b.WriteString(pad + "  <li>" + it.html + "\n")
		b.WriteString(listHTML(it.children, pad+"    ") + "\n")
		b.WriteString(pad + "  </li>\n")
	}
	b.WriteString(pad + "</" + tag + ">")
	return b.String()
}

func tableHTML(t block) string {
	var b strings.Builder
	b.WriteString("<table>\n")
	for i, row := range t.rows {
		tag := "td"
		if i == 0 && t.header {
			tag = "th"
		}
		b.WriteString("  <tr>")
		for _, c := range row {
			span := ""
			if c.colspan > 1 {
				span += fmt.Sprintf(` colspan="%d"`, c.colspan)
			}
			if c.rowspan > 1 {
				span += fmt.Sprintf(` rowspan="%d"`, c.rowspan)
			}
			var lines []string
			for _, p := range c.paras {
				lines = append(lines, runsHTML(p.runs))
			}
			b.WriteString("<" + tag + span + ">" + strings.Join(lines, "<br>") + "</" + tag + ">")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>")
	return b.String()
}

// plainASCII maps the accented letters of Western European languages to
// their base letter, so "Résumé" gives "resume" in a file name.
var plainASCII = strings.NewReplacer(
	"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a", "æ", "ae",
	"ç", "c", "è", "e", "é", "e", "ê", "e", "ë", "e",
	"ì", "i", "í", "i", "î", "i", "ï", "i", "ñ", "n",
	"ò", "o", "ó", "o", "ô", "o", "õ", "o", "ö", "o", "ø", "o", "œ", "oe",
	"ù", "u", "ú", "u", "û", "u", "ü", "u", "ý", "y", "ÿ", "y", "ß", "ss",
)

// slug turns a slide title into a short, safe file name part.
func slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range plainASCII.Replace(strings.ToLower(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	if b.Len() == 0 {
		return "slide"
	}
	return strings.TrimRight(b.String(), "-")
}
