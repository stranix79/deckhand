// Package pptx converts a PowerPoint file (.pptx) into a Deckhand deck.
//
// The conversion is semantic, not pixel-perfect: it keeps what a slide says
// (title, text, lists, pictures, tables, videos, presenter notes) and lays it
// out with one clean stylesheet. Themes, animations, charts and SmartArt are
// not converted; every loss is reported as a warning so nothing disappears
// silently. See docs/DECISIONS.md (2026-11-18) for the reasoning.
//
// Only the standard library is used: a .pptx is a zip of XML parts.
package pptx

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/stranix79/deckhand/internal/deck"
)

// Hard limits on what is read from the archive, so a hostile file cannot
// exhaust memory or disk. A deck is limited to 200 MB anyway.
const (
	maxXMLPart = 64 << 20  // one XML part
	maxAsset   = 200 << 20 // one picture or video
)

// Deck is a parsed PowerPoint file, ready to be written as a Deckhand deck.
type Deck struct {
	Title    string   // first slide title, else the document title; may be empty
	Lang     string   // language of the first text run ("en-US"), may be empty
	Ratio    string   // "16:9", "16:10" or "4:3" (the closest one)
	Hidden   int      // hidden slides, skipped
	Warnings []string // everything that was not converted

	slides   []*slide
	animated int               // slides with animations, reported once
	assets   []*asset          // pictures and videos to copy, in first-use order
	byPart   map[string]*asset // zip path → asset
	names    map[string]bool   // asset file names already taken

	defaultText *node             // <p:defaultTextStyle>: text defaults of free shapes
	font        string            // body font of the theme, "" when unknown
	theme       map[string]string // colour scheme: slot (dk1, accent1…) → hex
	lineWidths  []float64         // theme line widths, in EMU
	emuW, emuH  float64           // slide size in PowerPoint units

	files  map[string]*zip.File
	closer io.Closer
}

// Slides is the number of slides that will be written.
func (d *Deck) Slides() int { return len(d.slides) }

// Assets is the number of pictures and videos that will be copied.
func (d *Deck) Assets() int { return len(d.assets) }

// WithNotes is the number of slides that carry presenter notes.
func (d *Deck) WithNotes() int {
	n := 0
	for _, s := range d.slides {
		if s.notes != "" {
			n++
		}
	}
	return n
}

// Close releases the underlying file. Safe to call on a Deck built by Read.
func (d *Deck) Close() error {
	if d.closer == nil {
		return nil
	}
	err := d.closer.Close()
	d.closer = nil
	return err
}

type asset struct {
	name string // file name under assets/
	file *zip.File
}

type slide struct {
	source int // 1-based position in the PowerPoint file
	title  string
	cover  bool // title slide layout (centred title)
	blocks []block
	notes  string

	// A drawing keeps the layout PowerPoint gave it (see canvas.go).
	drawn  bool
	canvas []elem
	bg, fg string // CSS colours of the slide background and default text
}

type blockKind int

const (
	blockText blockKind = iota
	blockImage
	blockVideo
	blockAudio
	blockTable
	blockMissing // something that could not be converted, kept visible
)

type block struct {
	kind   blockKind
	lead   bool // subtitle text
	paras  []para
	asset  string // file name under assets/
	poster string // video poster, file name under assets/
	alt    string
	rows   [][]cell
	header bool // first table row is a header
	label  string
	crop   bool // picture cropped in PowerPoint: fill its box instead of stretching
}

type bulletKind int

const (
	bulletNone bulletKind = iota
	bulletDot
	bulletNumber
)

type para struct {
	level  int
	bullet bulletKind
	runs   []run
	align  string  // drawings only: l, ctr, r, just
	size   float64 // drawings only: font size in slide pixels
}

type run struct {
	text      string
	bold      bool
	italic    bool
	underline bool
	strike    bool
	code      bool
	href      string
	br        bool   // line break, text is empty
	style     string // drawings only: CSS for this run (colour, size)
}

type cell struct {
	paras   []para
	colspan int
	rowspan int
}

// Open parses a .pptx file. Close the Deck when done.
func Open(name string) (*Deck, error) {
	f, err := os.Open(name) //nolint:gosec // the user's own file, given on the command line
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	d, err := Read(f, st.Size())
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	d.closer = f
	return d, nil
}

// Read parses a .pptx from a reader. The reader must stay open until Write
// has been called, since pictures are copied straight from the archive.
func Read(r io.ReaderAt, size int64) (*Deck, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("not a PowerPoint file (a .pptx is a zip archive): %w", err)
	}
	d := &Deck{
		files:  make(map[string]*zip.File, len(zr.File)),
		byPart: map[string]*asset{},
		names:  map[string]bool{},
	}
	for _, f := range zr.File {
		d.files[f.Name] = f
	}

	main := "ppt/presentation.xml"
	for _, r := range d.rels("") {
		if strings.HasSuffix(r.typ, "/officeDocument") && !r.external {
			main = r.target
		}
	}
	pres, err := d.part(main)
	if err != nil {
		if _, old := d.files["PowerPoint Document"]; old {
			return nil, errors.New("this is an old binary .ppt file: open it in PowerPoint or LibreOffice and save it as .pptx first")
		}
		return nil, fmt.Errorf("not a PowerPoint file: %w", err)
	}
	if pres.name != "presentation" {
		return nil, fmt.Errorf("not a PowerPoint file: main part is <%s>, expected <presentation>", pres.name)
	}

	d.Ratio = ratioFor(pres.child("sldSz"))
	d.emuW, d.emuH = num(pres.child("sldSz"), "cx"), num(pres.child("sldSz"), "cy")
	if d.emuW <= 0 || d.emuH <= 0 {
		d.emuW, d.emuH = 12192000, 6858000
	}
	d.defaultText = pres.child("defaultTextStyle")
	presRels := d.rels(main)
	d.loadTheme(presRels)
	layouts := map[string]*layoutInfo{}

	for i, id := range pres.child("sldIdLst").all("sldId") {
		pos := i + 1
		rel, ok := presRels[id.rattr("id")]
		if !ok || rel.external {
			d.warn(pos, "slide part not found in the file, skipped")
			continue
		}
		root, err := d.part(rel.target)
		if err != nil {
			d.warn(pos, "unreadable slide, skipped (%v)", err)
			continue
		}
		if root.attr("show") == "0" {
			d.Hidden++
			continue
		}
		d.slides = append(d.slides, d.readSlide(pos, rel.target, root, layouts))
	}
	if len(d.slides) == 0 {
		return nil, errors.New("no slide to convert in this file")
	}

	if d.animated > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("animations on %d slide(s) are not converted: each slide shows everything at once", d.animated))
	}

	d.Title = d.slides[0].title
	if d.Title == "" {
		d.Title = d.coreTitle()
	}
	return d, nil
}

func (d *Deck) warn(pos int, format string, args ...any) {
	d.Warnings = append(d.Warnings, fmt.Sprintf("slide %d: ", pos)+fmt.Sprintf(format, args...))
}

// file finds an entry of the archive. Relationship targets are URIs: a part
// whose name has a space may be referenced as "My%20picture.png".
func (d *Deck) file(name string) (*zip.File, bool) {
	if f, ok := d.files[name]; ok {
		return f, true
	}
	if decoded, err := url.PathUnescape(name); err == nil && decoded != name {
		f, ok := d.files[decoded]
		return f, ok
	}
	return nil, false
}

// part reads and parses one XML part of the archive.
func (d *Deck) part(name string) (*node, error) {
	f, ok := d.file(name)
	if !ok {
		return nil, fmt.Errorf("%s is missing", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return parseXML(io.LimitReader(rc, maxXMLPart))
}

type rel struct {
	typ      string
	target   string // zip path for internal targets, URL for external ones
	external bool
}

// rels loads the relationships of a part ("" for the package itself), keyed
// by id. Internal targets are resolved to full zip paths.
func (d *Deck) rels(part string) map[string]rel {
	dir, base := path.Split(part)
	root, err := d.part(dir + "_rels/" + base + ".rels")
	if err != nil {
		return nil
	}
	out := map[string]rel{}
	for _, r := range root.all("Relationship") {
		x := rel{typ: r.attr("Type"), target: r.attr("Target")}
		switch {
		case r.attr("TargetMode") == "External":
			x.external = true
		case strings.HasPrefix(x.target, "/"):
			x.target = strings.TrimPrefix(path.Clean(x.target), "/")
		default:
			x.target = path.Join(dir, x.target)
		}
		out[r.attr("Id")] = x
	}
	return out
}

// ratioFor maps the slide size to the closest ratio Deckhand knows.
func ratioFor(sz *node) string {
	cx, _ := strconv.ParseFloat(sz.attr("cx"), 64)
	cy, _ := strconv.ParseFloat(sz.attr("cy"), 64)
	if cx <= 0 || cy <= 0 {
		return "16:9"
	}
	r := cx / cy
	best, bestDiff := "16:9", 99.0
	for _, c := range []struct {
		name string
		v    float64
	}{{"16:9", 16.0 / 9}, {"16:10", 1.6}, {"4:3", 4.0 / 3}} {
		diff := r - c.v
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			best, bestDiff = c.name, diff
		}
	}
	return best
}

// genericTitles are document titles PowerPoint or a template left behind.
var genericTitles = map[string]bool{
	"powerpoint presentation":    true,
	"présentation powerpoint":    true,
	"powerpoint-präsentation":    true,
	"presentación de powerpoint": true,
	"slide 1":                    true,
}

func (d *Deck) coreTitle() string {
	root, err := d.part("docProps/core.xml")
	if err != nil {
		return ""
	}
	t := strings.TrimSpace(root.child("title").textOf())
	if genericTitles[strings.ToLower(t)] {
		return ""
	}
	return t
}

func (n *node) textOf() string {
	if n == nil {
		return ""
	}
	return n.text
}

// slideCtx carries what is needed while walking the shapes of one slide.
type slideCtx struct {
	d    *Deck
	s    *slide
	rels map[string]rel
	li   *layoutInfo

	// Drawings only.
	base       xform   // PowerPoint units → slide pixels
	pxPerPt    float64 // font points → slide pixels
	height     float64 // slide height in pixels
	freeform   int     // custom shapes that are not drawn
	namedTitle string  // text of a shape PowerPoint calls "Title…"
}

// layoutFor loads what a slide inherits from: its layout and its master.
func (d *Deck) layoutFor(part string, cache map[string]*layoutInfo) *layoutInfo {
	if li, seen := cache[part]; seen {
		return li
	}
	li := &layoutInfo{}
	li.layout, _ = d.part(part)
	for _, r := range d.rels(part) {
		if strings.HasSuffix(r.typ, "/slideMaster") && !r.external {
			li.master, _ = d.part(r.target)
		}
	}
	if m := li.master.child("clrMap"); m != nil {
		li.clrMap = map[string]string{}
		for _, a := range m.attrs {
			li.clrMap[a.Name.Local] = a.Value
		}
	}
	cache[part] = li
	return li
}

func (d *Deck) readSlide(pos int, part string, root *node, layouts map[string]*layoutInfo) *slide {
	c := &slideCtx{d: d, s: &slide{source: pos}, rels: d.rels(part), li: &layoutInfo{}}
	for _, r := range c.rels {
		switch {
		case strings.HasSuffix(r.typ, "/slideLayout") && !r.external:
			c.li = d.layoutFor(r.target, layouts)
		case strings.HasSuffix(r.typ, "/notesSlide") && !r.external:
			c.s.notes = d.readNotes(r.target)
		}
	}
	tree := root.path("cSld", "spTree")
	if isDrawing(tree) {
		c.draw(root, tree)
	} else {
		c.walk(tree)
	}
	if timing := root.child("timing"); timing != nil {
		for _, name := range []string{"set", "anim", "animEffect", "animMotion", "animScale", "animRot", "animClr"} {
			if timing.find(name) != nil {
				d.animated++
				break
			}
		}
	}
	return c.s
}

// draw converts a slide as a drawing: every element keeps its position. The
// slide is scaled to fit the deck ratio and centred, since PowerPoint allows
// any slide size and Deckhand three ratios.
func (c *slideCtx) draw(root, tree *node) {
	d := c.d
	height, err := deck.HeightFor(d.Ratio, designWidth)
	if err != nil {
		height = designWidth * 9 / 16
	}
	c.height = float64(height)
	scale := math.Min(designWidth/d.emuW, c.height/d.emuH)
	c.base = xform{sx: scale, sy: scale, tx: (designWidth - d.emuW*scale) / 2, ty: (c.height - d.emuH*scale) / 2}
	c.pxPerPt = scale * 12700

	c.s.drawn = true
	c.s.bg = c.background(root)
	c.s.fg = c.themeColor("tx1")
	c.canvasWalk(tree, c.base)
	if c.s.title == "" {
		c.s.title = c.guessTitle()
	}
	if c.freeform > 0 {
		d.warn(c.s.source, "%d freeform shape(s) not drawn, their text is kept", c.freeform)
	}
}

// readNotes returns the presenter notes of a notes part, as plain text.
func (d *Deck) readNotes(part string) string {
	root, err := d.part(part)
	if err != nil {
		return ""
	}
	var lines []string
	for _, sp := range root.path("cSld", "spTree").all("sp") {
		if sp.path("nvSpPr", "nvPr", "ph").attr("type") != "body" {
			continue
		}
		for _, p := range sp.child("txBody").all("p") {
			lines = append(lines, plain(readRuns(p, nil, nil)))
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// walk visits the children of a shape tree (or a group) in document order,
// which is the stacking order and, in practice, the reading order.
func (c *slideCtx) walk(tree *node) {
	if tree == nil {
		return
	}
	for _, ch := range tree.children {
		switch ch.name {
		case "sp":
			c.shape(ch)
		case "pic":
			c.picture(ch)
		case "grpSp":
			c.walk(ch)
		case "graphicFrame":
			c.frame(ch)
		case "AlternateContent":
			// Newer features (equations, 3D models) ship a plain fallback,
			// usually a picture: that is the one a browser can show.
			if fb := ch.child("Fallback"); fb != nil {
				c.walk(fb)
			} else {
				c.walk(ch.child("Choice"))
			}
		}
	}
}

func (c *slideCtx) shape(sp *node) {
	ph := sp.path("nvSpPr", "nvPr", "ph")
	typ := ph.attr("type")
	if ph != nil {
		switch typ {
		case "dt", "ftr", "sldNum", "hdr":
			return // date, footer, slide number: Deckhand shows its own
		}
	}
	tx := sp.child("txBody")
	if tx == nil {
		return
	}
	def := bulletNone
	if ph != nil && (typ == "" || typ == "body" || typ == "obj") && c.layoutBullets(typ, ph.attr("idx")) {
		def = bulletDot
	}
	paras := c.paragraphs(tx, def)
	if len(paras) == 0 {
		return
	}
	switch {
	case ph != nil && (typ == "title" || typ == "ctrTitle") && c.s.title == "":
		var parts []string
		for _, p := range paras {
			parts = append(parts, strings.Join(strings.Fields(plain(p.runs)), " "))
		}
		c.s.title = strings.Join(parts, " ")
		c.s.cover = typ == "ctrTitle"
	case ph != nil && typ == "subTitle":
		c.s.blocks = append(c.s.blocks, block{kind: blockText, lead: true, paras: paras})
	default:
		c.s.blocks = append(c.s.blocks, block{kind: blockText, paras: paras})
	}
}

// layoutBullets tells whether a body placeholder is bulleted by default. The
// slide itself rarely says so: the layout does, in the list style of the
// matching placeholder. Without a layout answer, body text is bulleted, which
// is what PowerPoint masters do.
func (c *slideCtx) layoutBullets(typ, idx string) bool {
	for _, sp := range c.li.layout.path("cSld", "spTree").all("sp") {
		ph := sp.path("nvSpPr", "nvPr", "ph")
		if ph == nil {
			continue
		}
		match := idx != "" && ph.attr("idx") == idx
		if idx == "" {
			match = ph.attr("type") == typ
		}
		if !match {
			continue
		}
		return sp.path("txBody", "lstStyle", "lvl1pPr").child("buNone") == nil
	}
	return true
}

func (c *slideCtx) paragraphs(tx *node, def bulletKind) []para {
	var out []para
	for _, p := range tx.all("p") {
		pp := para{bullet: def, runs: readRuns(p, c.rels, c.d)}
		if strings.TrimSpace(plain(pp.runs)) == "" {
			continue
		}
		pPr := p.child("pPr")
		if lvl, err := strconv.Atoi(pPr.attr("lvl")); err == nil && lvl > 0 && lvl < 9 {
			pp.level = lvl
		}
		switch {
		case pPr.child("buNone") != nil:
			pp.bullet = bulletNone
		case pPr.child("buAutoNum") != nil:
			pp.bullet = bulletNumber
		case pPr.child("buChar") != nil, pPr.child("buBlip") != nil:
			pp.bullet = bulletDot
		}
		out = append(out, pp)
	}
	return out
}

var monoFont = regexp.MustCompile(`(?i)courier|consolas|menlo|monaco|mono|source code|fira code|cascadia`)

var langTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// readRuns turns the runs of a paragraph into text with basic formatting.
// rels resolves hyperlinks; d, when given, records the document language.
func readRuns(p *node, rels map[string]rel, d *Deck) []run {
	var out []run
	for _, ch := range p.children {
		switch ch.name {
		case "br":
			out = append(out, run{br: true})
		case "r", "fld":
			if ch.name == "fld" && ch.attr("type") == "slidenum" {
				continue
			}
			text := ch.child("t").textOf()
			if text == "" {
				continue
			}
			rPr := ch.child("rPr")
			r := run{
				text:   text,
				bold:   rPr.attr("b") == "1" || rPr.attr("b") == "true",
				italic: rPr.attr("i") == "1" || rPr.attr("i") == "true",
				code:   monoFont.MatchString(rPr.child("latin").attr("typeface")),
			}
			if u := rPr.attr("u"); u != "" && u != "none" {
				r.underline = true
			}
			if s := rPr.attr("strike"); s != "" && s != "noStrike" {
				r.strike = true
			}
			if link, ok := rels[rPr.child("hlinkClick").rattr("id")]; ok && link.external && safeURL(link.target) {
				r.href = link.target
				r.underline = false // links are underlined by PowerPoint, not by the author
			}
			if d != nil && d.Lang == "" {
				if l := rPr.attr("lang"); langTag.MatchString(l) {
					d.Lang = l
				}
			}
			out = append(out, r)
		}
	}
	return out
}

func safeURL(u string) bool {
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "mailto:")
}

func plain(runs []run) string {
	var b strings.Builder
	for _, r := range runs {
		if r.br {
			b.WriteByte('\n')
			continue
		}
		b.WriteString(r.text)
	}
	return b.String()
}

// webImages are the picture formats a browser shows and a deck accepts.
var webImages = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".avif": true, ".svg": true,
}

var webMedia = map[string]blockKind{
	".mp4": blockVideo, ".webm": blockVideo, ".mov": blockVideo,
	".mp3": blockAudio, ".m4a": blockAudio, ".ogg": blockAudio, ".wav": blockAudio,
}

func (c *slideCtx) picture(pic *node) {
	nv := pic.child("nvPicPr")
	alt := strings.TrimSpace(nv.child("cNvPr").attr("descr"))
	blip := pic.path("blipFill", "blip")

	// A video or a sound is a picture (its poster) with a media reference.
	nvPr := nv.child("nvPr")
	media := nvPr.child("videoFile")
	if media == nil {
		media = nvPr.child("audioFile")
	}
	if media != nil {
		r, ok := c.rels[media.rattr("link")]
		kind, known := webMedia[strings.ToLower(path.Ext(r.target))]
		switch {
		case ok && r.external:
			c.missing("linked video or sound (%s): only embedded media are converted", r.target)
		case ok && known:
			if name := c.d.addAsset(r.target); name != "" {
				b := block{kind: kind, asset: name, alt: alt}
				if p, ok := c.rels[blip.rattr("embed")]; ok && !p.external && webImages[strings.ToLower(path.Ext(p.target))] {
					b.poster = c.d.addAsset(p.target)
				}
				c.s.blocks = append(c.s.blocks, b)
				return
			}
			c.missing("media %s is missing from the file", path.Base(r.target))
		default:
			c.missing("media %s: format not playable in a browser", path.Base(r.target))
		}
		return
	}

	r, ok := c.rels[blip.rattr("embed")]
	if !ok {
		if l, linked := c.rels[blip.rattr("link")]; linked {
			c.missing("linked picture (%s): only embedded pictures are converted", l.target)
		}
		return
	}
	ext := strings.ToLower(path.Ext(r.target))
	if r.external || !webImages[ext] {
		kind := strings.TrimPrefix(ext, ".")
		if kind == "" {
			kind = "this"
		}
		c.missing("picture %s: %s is not a web image format, export it as PNG", path.Base(r.target), kind)
		return
	}
	name := c.d.addAsset(r.target)
	if name == "" {
		c.missing("picture %s is missing from the file", path.Base(r.target))
		return
	}
	c.s.blocks = append(c.s.blocks, block{kind: blockImage, asset: name, alt: alt, crop: pic.path("blipFill", "srcRect") != nil})
}

// missing keeps a visible placeholder in the slide and a warning in the
// report, so the author knows exactly what to redo by hand.
func (c *slideCtx) missing(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	c.d.warn(c.s.source, "%s", msg)
	c.s.blocks = append(c.s.blocks, block{kind: blockMissing, label: msg})
}

func (c *slideCtx) frame(gf *node) {
	gd := gf.path("graphic", "graphicData")
	switch {
	case gd.child("tbl") != nil:
		c.table(gd.child("tbl"))
	case gd.find("chart") != nil:
		c.missing("chart not converted: export it as a picture, or rebuild it in HTML")
	case gd.find("relIds") != nil:
		c.missing("SmartArt diagram not converted: export it as a picture")
	case gd.find("pic") != nil:
		c.picture(gd.find("pic")) // embedded object with a preview picture
	default:
		c.missing("embedded object not converted")
	}
}

func (c *slideCtx) table(tbl *node) {
	b := block{kind: blockTable, header: tbl.child("tblPr").attr("firstRow") == "1"}
	for _, tr := range tbl.all("tr") {
		var row []cell
		for _, tc := range tr.all("tc") {
			if tc.attr("hMerge") == "1" || tc.attr("vMerge") == "1" {
				continue // covered by a spanning cell
			}
			cl := cell{paras: c.paragraphs(tc.child("txBody"), bulletNone)}
			cl.colspan, _ = strconv.Atoi(tc.attr("gridSpan"))
			cl.rowspan, _ = strconv.Atoi(tc.attr("rowSpan"))
			row = append(row, cl)
		}
		if len(row) > 0 {
			b.rows = append(b.rows, row)
		}
	}
	if len(b.rows) > 0 {
		c.s.blocks = append(c.s.blocks, b)
	}
}

var unsafeName = regexp.MustCompile(`[^a-z0-9._-]+`)

// addAsset registers a media part to copy and returns its file name under
// assets/. The same part used on ten slides is copied once. Returns "" when
// the part is not in the archive.
func (d *Deck) addAsset(part string) string {
	if a, ok := d.byPart[part]; ok {
		return a.name
	}
	f, ok := d.file(part)
	if !ok {
		return ""
	}
	ext := strings.ToLower(path.Ext(part))
	stem := strings.Trim(unsafeName.ReplaceAllString(strings.ToLower(strings.TrimSuffix(path.Base(part), path.Ext(part))), "-"), "-.")
	if stem == "" {
		stem = "media"
	}
	name := stem + ext
	for i := 2; d.names[name]; i++ {
		name = fmt.Sprintf("%s-%d%s", stem, i, ext)
	}
	d.names[name] = true
	a := &asset{name: name, file: f}
	d.byPart[part] = a
	d.assets = append(d.assets, a)
	return name
}
