package pptx

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// This file is the second conversion mode. A slide that is a drawing (boxes,
// arrows, labels placed by hand) means nothing once flattened to a list of
// texts, so it is redrawn where PowerPoint put things: shapes become
// positioned <div>s with their colours, connectors become SVG lines.

// layoutInfo is what a slide inherits: its layout, the master behind it and
// the master's colour mapping.
type layoutInfo struct {
	layout *node
	master *node
	clrMap map[string]string
}

// xform maps PowerPoint units (EMU) to slide pixels. Groups have their own
// coordinate space, so entering a group yields a new xform.
type xform struct{ sx, sy, tx, ty float64 }

func num(n *node, name string) float64 {
	v, _ := strconv.ParseFloat(n.attr(name), 64)
	return v
}

func (t xform) group(xf *node) xform {
	if xf == nil {
		return t
	}
	ox, oy := num(xf.child("off"), "x"), num(xf.child("off"), "y")
	ex, ey := num(xf.child("ext"), "cx"), num(xf.child("ext"), "cy")
	cx, cy := num(xf.child("chOff"), "x"), num(xf.child("chOff"), "y")
	cex, cey := num(xf.child("chExt"), "cx"), num(xf.child("chExt"), "cy")
	rx, ry := 1.0, 1.0
	if cex > 0 && ex > 0 {
		rx = ex / cex
	}
	if cey > 0 && ey > 0 {
		ry = ey / cey
	}
	return xform{
		sx: t.sx * rx, sy: t.sy * ry,
		tx: t.tx + t.sx*(ox-cx*rx), ty: t.ty + t.sy*(oy-cy*ry),
	}
}

// box is where an element sits on the slide, in pixels.
type box struct {
	x, y, w, h   float64
	rot          float64 // degrees, clockwise
	flipH, flipV bool
}

func (t xform) box(xf *node) (box, bool) {
	off, ext := xf.child("off"), xf.child("ext")
	if off == nil || ext == nil {
		return box{}, false
	}
	return box{
		x: num(off, "x")*t.sx + t.tx, y: num(off, "y")*t.sy + t.ty,
		w: num(ext, "cx") * t.sx, h: num(ext, "cy") * t.sy,
		rot:   num(xf, "rot") / 60000,
		flipH: xf.attr("flipH") == "1", flipV: xf.attr("flipV") == "1",
	}, true
}

type elemKind int

const (
	elemShape elemKind = iota // a box with an optional fill, border and text
	elemLine                  // a line or a connector
	elemBlock                 // a picture, a video, a table or a placeholder box
)

type elem struct {
	kind elemKind
	box
	style  []string // CSS declarations of a shape
	paras  []para
	nowrap bool

	path   string // SVG path of a line
	heads  []string
	stroke string
	width  float64
	dash   bool

	block  block
	fontPx float64
}

// isDrawing tells whether a slide was laid out by hand rather than by filling
// the placeholders of its layout: connectors, groups, or several free shapes.
func isDrawing(tree *node) bool {
	connectors, groups, free := 0, 0, 0
	var visit func(n *node)
	visit = func(n *node) {
		if n == nil {
			return
		}
		for _, ch := range n.children {
			switch ch.name {
			case "cxnSp":
				connectors++
			case "grpSp":
				groups++
				visit(ch)
			case "sp":
				if ch.path("nvSpPr", "nvPr", "ph") == nil {
					free++
				}
			case "AlternateContent":
				if fb := ch.child("Fallback"); fb != nil {
					visit(fb)
				} else {
					visit(ch.child("Choice"))
				}
			}
		}
	}
	visit(tree)
	return connectors > 0 || (groups > 0 && free >= 2) || free >= 4
}

// canvasWalk is walk for drawings: same traversal, positioned output.
func (c *slideCtx) canvasWalk(tree *node, t xform) {
	if tree == nil {
		return
	}
	for _, ch := range tree.children {
		switch ch.name {
		case "sp":
			c.cshape(ch, t, false)
		case "cxnSp":
			c.cshape(ch, t, true)
		case "pic":
			pic := ch
			c.placed(pic.path("spPr", "xfrm"), t, func() { c.picture(pic) })
		case "grpSp":
			c.canvasWalk(ch, t.group(ch.path("grpSpPr", "xfrm")))
		case "graphicFrame":
			gf := ch
			c.placed(gf.child("xfrm"), t, func() { c.frame(gf) })
		case "AlternateContent":
			if fb := ch.child("Fallback"); fb != nil {
				c.canvasWalk(fb, t)
			} else {
				c.canvasWalk(ch.child("Choice"), t)
			}
		}
	}
}

// placed runs one of the flow-mode converters (picture, table…) and pins
// whatever it produced at the position of the source element.
func (c *slideCtx) placed(xf *node, t xform, convert func()) {
	before := len(c.s.blocks)
	convert()
	produced := append([]block(nil), c.s.blocks[before:]...)
	c.s.blocks = c.s.blocks[:before]
	b, ok := t.box(xf)
	if !ok {
		return
	}
	for _, blk := range produced {
		c.s.canvas = append(c.s.canvas, elem{kind: elemBlock, box: b, block: blk, fontPx: 18 * c.pxPerPt})
	}
}

// lineGeoms are the preset shapes that are a stroke, not a surface.
func lineGeom(prst string) bool {
	return prst == "line" || prst == "straightConnector1" ||
		strings.HasPrefix(prst, "bentConnector") || strings.HasPrefix(prst, "curvedConnector")
}

func (c *slideCtx) cshape(sp *node, t xform, connector bool) {
	nv := sp.child("nvSpPr")
	if connector {
		nv = sp.child("nvCxnSpPr")
	}
	ph := nv.path("nvPr", "ph")
	typ := ph.attr("type")
	if ph != nil {
		switch typ {
		case "dt", "ftr", "sldNum", "hdr":
			return
		}
	}
	var chain []*node
	if ph != nil {
		chain = c.phChain(typ, ph.attr("idx"))
	}
	spPr := sp.child("spPr")
	xf := spPr.child("xfrm")
	for i := 0; xf == nil && i < len(chain); i++ {
		xf = chain[i].path("spPr", "xfrm")
	}
	b, ok := t.box(xf)
	if !ok {
		if ph == nil {
			return
		}
		b = c.fallbackBox(typ)
	}
	style := sp.child("style")
	prst := spPr.child("prstGeom").attr("prst")
	if connector || lineGeom(prst) {
		c.cline(spPr, style, b, prst, t)
		return
	}

	e := elem{kind: elemShape, box: b}
	fill := c.fill(spPr, style)
	stroke, sw, dash := c.stroke(spPr, style, t)
	if spPr.child("custGeom") != nil {
		c.freeform++
		fill, stroke = "", ""
	}
	clip, radius := geometryCSS(prst, b)
	if fill != "" {
		e.style = append(e.style, "background:"+fill)
	}
	if stroke != "" && clip == "" {
		kind := "solid"
		if dash {
			kind = "dashed"
		}
		e.style = append(e.style, fmt.Sprintf("border:%s %s %s", px(sw), kind, stroke))
	}
	if radius != "" {
		e.style = append(e.style, "border-radius:"+radius)
	}
	if clip != "" && (fill != "" || stroke != "") {
		if fill == "" {
			e.style = append(e.style, "background:"+stroke)
		}
		e.style = append(e.style, "clip-path:"+clip)
	}

	group := "otherStyle"
	defBullet := bulletNone
	if ph != nil {
		switch typ {
		case "title", "ctrTitle":
			group = "titleStyle"
		default:
			group = "bodyStyle"
			if (typ == "" || typ == "body" || typ == "obj") && c.layoutBullets(typ, ph.attr("idx")) {
				defBullet = bulletDot
			}
		}
	}
	tx := sp.child("txBody")
	tc := &textCtx{c: c, sp: sp, chain: chain, group: group, style: style, defBullet: defBullet}
	textCSS := tc.boxCSS(tx)
	e.paras = tc.paragraphs(tx)
	if fill == "" && stroke == "" && len(e.paras) == 0 {
		return
	}
	if len(e.paras) > 0 {
		e.style = append(e.style, textCSS...)
		e.nowrap = tc.bodyAttr(tx, "wrap") == "none"
		if c.s.title == "" && ph != nil && (typ == "title" || typ == "ctrTitle") {
			c.s.title = titleText(e.paras)
		}
		if c.namedTitle == "" && strings.HasPrefix(strings.ToLower(nv.child("cNvPr").attr("name")), "tit") {
			c.namedTitle = titleText(e.paras)
		}
	}
	c.s.canvas = append(c.s.canvas, e)
}

func titleText(paras []para) string {
	var parts []string
	for _, p := range paras {
		if t := strings.Join(strings.Fields(plain(p.runs)), " "); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

// fallbackBox places a placeholder that has no position anywhere in the
// file: a band at the top for a title, the rest of the slide otherwise.
func (c *slideCtx) fallbackBox(typ string) box {
	w, h := float64(designWidth), c.height
	if typ == "title" || typ == "ctrTitle" {
		return box{x: w * 0.06, y: h * 0.05, w: w * 0.88, h: h * 0.16}
	}
	return box{x: w * 0.06, y: h * 0.25, w: w * 0.88, h: h * 0.65}
}

// phChain returns the layout placeholder and the master placeholder a slide
// placeholder inherits from, closest first.
func (c *slideCtx) phChain(typ, idx string) []*node {
	var chain []*node
	find := func(root *node, match func(ph *node) bool) *node {
		for _, sp := range root.path("cSld", "spTree").all("sp") {
			if ph := sp.path("nvSpPr", "nvPr", "ph"); ph != nil && match(ph) {
				return sp
			}
		}
		return nil
	}
	if l := find(c.li.layout, func(ph *node) bool {
		if idx != "" {
			return ph.attr("idx") == idx
		}
		return ph.attr("type") == typ
	}); l != nil {
		chain = append(chain, l)
		if typ == "" {
			typ = l.path("nvSpPr", "nvPr", "ph").attr("type")
		}
	}
	masterType := typ
	switch typ {
	case "ctrTitle":
		masterType = "title"
	case "", "subTitle", "obj":
		masterType = "body"
	}
	if m := find(c.li.master, func(ph *node) bool { return ph.attr("type") == masterType }); m != nil {
		chain = append(chain, m)
	}
	return chain
}

func (c *slideCtx) color(holder *node) string {
	col, _ := c.d.colorOf(holder, c.li.clrMap)
	return col
}

// fill returns the CSS background of a shape, "" for none.
func (c *slideCtx) fill(spPr, style *node) string {
	if spPr != nil {
		for _, ch := range spPr.children {
			switch ch.name {
			case "noFill", "blipFill", "grpFill":
				return ""
			case "solidFill":
				return c.color(ch)
			case "pattFill":
				return c.color(ch.child("fgClr"))
			case "gradFill":
				stops := ch.child("gsLst").all("gs")
				if len(stops) == 0 {
					return ""
				}
				first, last := c.color(stops[0]), c.color(stops[len(stops)-1])
				if first != "" && last != "" && first != last {
					return "linear-gradient(" + first + "," + last + ")"
				}
				return first
			}
		}
	}
	if ref := style.child("fillRef"); ref != nil && ref.attr("idx") != "0" {
		return c.color(ref)
	}
	return ""
}

// stroke returns the outline of a shape: colour ("" for none), width in
// pixels, and whether it is dashed.
func (c *slideCtx) stroke(spPr, style *node, t xform) (col string, w float64, dash bool) {
	ln := spPr.child("ln")
	if ln.child("noFill") != nil {
		return "", 0, false
	}
	if sf := ln.child("solidFill"); sf != nil {
		col = c.color(sf)
	} else if gf := ln.child("gradFill"); gf != nil {
		if stops := gf.child("gsLst").all("gs"); len(stops) > 0 {
			col = c.color(stops[0])
		}
	}
	ref := style.child("lnRef")
	idx, _ := strconv.Atoi(ref.attr("idx"))
	if col == "" && idx > 0 {
		col = c.color(ref)
	}
	if col == "" {
		return "", 0, false
	}
	emu := num(ln, "w")
	if emu == 0 {
		emu = 9525
		if idx > 0 && idx <= len(c.d.lineWidths) {
			emu = c.d.lineWidths[idx-1]
		}
	}
	w = math.Max(1.5, emu*math.Abs(t.sx))
	if d := ln.child("prstDash").attr("val"); d != "" && d != "solid" {
		dash = true
	}
	return col, w, dash
}

// geometryCSS approximates the common preset shapes with CSS. Shapes that
// need a clip-path lose their outline; unknown ones stay rectangles.
func geometryCSS(prst string, b box) (clip, radius string) {
	switch prst {
	case "roundRect", "round1Rect", "round2SameRect", "round2DiagRect", "flowChartAlternateProcess", "wedgeRoundRectCallout":
		return "", px(math.Min(b.w, b.h) * 0.167)
	case "ellipse", "flowChartConnector", "donut":
		return "", "50%"
	case "can", "flowChartMagneticDisk":
		return "", "50% / 14%"
	case "flowChartTerminator":
		return "", px(b.h / 2)
	case "cloud", "cloudCallout":
		return "", px(math.Min(b.w, b.h) * 0.4)
	case "diamond", "flowChartDecision":
		return "polygon(50% 0,100% 50%,50% 100%,0 50%)", ""
	case "triangle", "flowChartExtract":
		return "polygon(50% 0,100% 100%,0 100%)", ""
	case "rtTriangle":
		return "polygon(0 0,100% 100%,0 100%)", ""
	case "parallelogram", "flowChartInputOutput":
		return "polygon(20% 0,100% 0,80% 100%,0 100%)", ""
	case "trapezoid":
		return "polygon(20% 0,80% 0,100% 100%,0 100%)", ""
	case "hexagon", "flowChartPreparation":
		return "polygon(20% 0,80% 0,100% 50%,80% 100%,20% 100%,0 50%)", ""
	case "homePlate":
		return "polygon(0 0,80% 0,100% 50%,80% 100%,0 100%)", ""
	case "chevron":
		return "polygon(0 0,80% 0,100% 50%,80% 100%,0 100%,20% 50%)", ""
	case "rightArrow":
		return "polygon(0 25%,65% 25%,65% 0,100% 50%,65% 100%,65% 75%,0 75%)", ""
	case "leftArrow":
		return "polygon(100% 25%,35% 25%,35% 0,0 50%,35% 100%,35% 75%,100% 75%)", ""
	case "downArrow":
		return "polygon(25% 0,75% 0,75% 65%,100% 65%,50% 100%,0 65%,25% 65%)", ""
	case "upArrow":
		return "polygon(25% 100%,75% 100%,75% 35%,100% 35%,50% 0,0 35%,25% 35%)", ""
	case "leftRightArrow":
		return "polygon(0 50%,20% 0,20% 25%,80% 25%,80% 0,100% 50%,80% 100%,80% 75%,20% 75%,20% 100%)", ""
	case "foldedCorner":
		return "polygon(0 0,100% 0,100% 80%,85% 100%,0 100%)", ""
	}
	return "", ""
}

// cline converts a line or a connector to an SVG path in slide pixels.
func (c *slideCtx) cline(spPr, style *node, b box, prst string, t xform) {
	col, w, dash := c.stroke(spPr, style, t)
	if col == "" {
		return
	}
	var pts [][2]float64
	curved := false
	switch {
	case prst == "bentConnector2":
		pts = [][2]float64{{0, 0}, {b.w, 0}, {b.w, b.h}}
	case strings.HasPrefix(prst, "bentConnector"):
		mid := b.w * adjust(spPr, 0.5)
		pts = [][2]float64{{0, 0}, {mid, 0}, {mid, b.h}, {b.w, b.h}}
	case strings.HasPrefix(prst, "curvedConnector"):
		curved = true
		pts = [][2]float64{{0, 0}, {b.w / 2, 0}, {b.w / 2, b.h}, {b.w, b.h}}
	default:
		pts = [][2]float64{{0, 0}, {b.w, b.h}}
	}
	// A connector is drawn in its own box, then flipped, rotated and placed.
	sin, cos := math.Sincos(b.rot * math.Pi / 180)
	for i, p := range pts {
		x, y := p[0], p[1]
		if b.flipH {
			x = b.w - x
		}
		if b.flipV {
			y = b.h - y
		}
		dx, dy := x-b.w/2, y-b.h/2
		pts[i] = [2]float64{b.x + b.w/2 + dx*cos - dy*sin, b.y + b.h/2 + dx*sin + dy*cos}
	}

	e := elem{kind: elemLine, stroke: col, width: w, dash: dash}
	ln := spPr.child("ln")
	arrow := func(end string) bool {
		t := ln.child(end).attr("type")
		return t != "" && t != "none"
	}
	last := len(pts) - 1
	if arrow("headEnd") {
		e.heads = append(e.heads, arrowHead(&pts[0], pts[1], w))
	}
	if arrow("tailEnd") {
		e.heads = append(e.heads, arrowHead(&pts[last], pts[last-1], w))
	}
	var d strings.Builder
	fmt.Fprintf(&d, "M%s %s", f1(pts[0][0]), f1(pts[0][1]))
	if curved {
		fmt.Fprintf(&d, "C%s %s %s %s %s %s", f1(pts[1][0]), f1(pts[1][1]), f1(pts[2][0]), f1(pts[2][1]), f1(pts[3][0]), f1(pts[3][1]))
	} else {
		for _, p := range pts[1:] {
			fmt.Fprintf(&d, "L%s %s", f1(p[0]), f1(p[1]))
		}
	}
	e.path = d.String()
	c.s.canvas = append(c.s.canvas, e)
}

// adjust reads the first adjustment of a preset shape (a share of its width).
func adjust(spPr *node, def float64) float64 {
	gd := spPr.path("prstGeom", "avLst", "gd")
	if v, err := strconv.ParseFloat(strings.TrimPrefix(gd.attr("fmla"), "val "), 64); err == nil && gd != nil {
		return v / 100000
	}
	return def
}

// arrowHead returns the triangle of an arrow whose tip is at *tip, coming
// from the direction of from, and pulls *tip back so the stroke does not
// poke through the point.
func arrowHead(tip *[2]float64, from [2]float64, w float64) string {
	dx, dy := tip[0]-from[0], tip[1]-from[1]
	length := math.Hypot(dx, dy)
	if length == 0 {
		return ""
	}
	ux, uy := dx/length, dy/length
	size := math.Max(14, w*4.5)
	half := size * 0.38
	bx, by := tip[0]-ux*size, tip[1]-uy*size
	points := fmt.Sprintf("%s,%s %s,%s %s,%s",
		f1(tip[0]), f1(tip[1]),
		f1(bx-uy*half), f1(by+ux*half),
		f1(bx+uy*half), f1(by-ux*half))
	if length > size {
		tip[0], tip[1] = tip[0]-ux*size*0.7, tip[1]-uy*size*0.7
	}
	return points
}

// textCtx resolves the text properties of one shape. PowerPoint spreads them
// over the run, the shape, the layout placeholder, the master placeholder
// and the master text styles: the first one that answers wins.
type textCtx struct {
	c         *slideCtx
	sp        *node
	chain     []*node
	group     string // titleStyle, bodyStyle or otherStyle
	style     *node  // <p:style> of the shape
	defBullet bulletKind
	color     string // default text colour of the shape, set by boxCSS
}

// levels lists the list-style definitions of a paragraph level, closest
// first. The last one may be the master text style: own tells how many come
// from the shape and its placeholders.
func (tc *textCtx) levels(level int) (out []*node, own int) {
	name := fmt.Sprintf("lvl%dpPr", level+1)
	add := func(n *node) {
		if n != nil {
			out = append(out, n)
		}
	}
	add(tc.sp.path("txBody", "lstStyle", name))
	for _, n := range tc.chain {
		add(n.path("txBody", "lstStyle", name))
	}
	own = len(out)
	add(tc.inherited(name))
	return out, own
}

// inherited is the last resort for a text property: the master text style
// for a placeholder, the presentation default for any other shape.
func (tc *textCtx) inherited(level string) *node {
	if tc.group == "otherStyle" {
		return tc.c.d.defaultText.child(level)
	}
	return tc.c.li.master.path("txStyles", tc.group, level)
}

func (tc *textCtx) bodyAttr(tx *node, name string) string {
	if v := tx.child("bodyPr").attr(name); v != "" {
		return v
	}
	for _, n := range tc.chain {
		if v := n.path("txBody", "bodyPr").attr(name); v != "" {
			return v
		}
	}
	return ""
}

// boxCSS is the CSS of the text frame: padding, vertical anchor, colour.
func (tc *textCtx) boxCSS(tx *node) []string {
	inset := func(name string, def float64) float64 {
		if v := tc.bodyAttr(tx, name); v != "" {
			def, _ = strconv.ParseFloat(v, 64)
		}
		return def * tc.c.base.sx
	}
	out := []string{fmt.Sprintf("padding:%s %s %s %s",
		px(inset("tIns", 45720)), px(inset("rIns", 91440)), px(inset("bIns", 45720)), px(inset("lIns", 91440)))}
	switch tc.bodyAttr(tx, "anchor") {
	case "ctr":
		out = append(out, "justify-content:center")
	case "b":
		out = append(out, "justify-content:flex-end")
	}
	// Default colour: the shape style, else the master text style, else the
	// slide's text colour.
	tc.color = tc.c.color(tc.style.child("fontRef"))
	if tc.color == "" {
		tc.color = tc.c.color(tc.inherited("lvl1pPr").path("defRPr", "solidFill"))
	}
	if tc.color == "" {
		tc.color = tc.c.s.fg
	}
	if tc.color != tc.c.s.fg {
		out = append(out, "color:"+tc.color)
	}
	return out
}

func (tc *textCtx) paragraphs(tx *node) []para {
	scale := 1.0
	if v := num(tx.path("bodyPr", "normAutofit"), "fontScale"); v > 0 {
		scale = v / 100000
	}
	var out []para
	for _, p := range tx.all("p") {
		pPr := p.child("pPr")
		pp := para{bullet: tc.defBullet, runs: readRuns(p, tc.c.rels, tc.c.d)}
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
		levels, own := tc.levels(pp.level)

		// Paragraph defaults, from the styles.
		pt, bold, boldSet, color := 0.0, false, false, ""
		pp.align = pPr.attr("algn")
		for i, l := range levels {
			def := l.child("defRPr")
			if pt == 0 {
				pt = num(def, "sz") / 100
			}
			if b := def.attr("b"); b != "" && !boldSet {
				bold, boldSet = b == "1", true
			}
			if color == "" && i < own {
				color = tc.c.color(def.child("solidFill"))
			}
			if pp.align == "" {
				pp.align = l.attr("algn")
			}
		}
		if pt == 0 {
			switch tc.group {
			case "titleStyle":
				pt = 44
			case "bodyStyle":
				pt = math.Max(16, 28-4*float64(pp.level))
			default:
				pt = 18
			}
		}

		// Runs override the defaults one by one.
		runNodes := runElements(p)
		first := true
		for i := range pp.runs {
			r := &pp.runs[i]
			if r.br || i >= len(runNodes) {
				continue
			}
			rPr := runNodes[i].child("rPr")
			size := pt
			if sz := num(rPr, "sz"); sz > 0 {
				size = sz / 100
			}
			if first {
				pp.size = size * scale * tc.c.pxPerPt
				first = false
			}
			var css []string
			if own := size * scale * tc.c.pxPerPt; math.Abs(own-pp.size) > 0.5 {
				css = append(css, "font-size:"+px(own))
			}
			col := tc.c.color(rPr.child("solidFill"))
			if col == "" && r.href == "" {
				col = color
			}
			if col != "" && col != tc.color {
				css = append(css, "color:"+col)
			}
			if bold && rPr.attr("b") == "" {
				r.bold = true
			}
			r.style = strings.Join(css, ";")
		}
		if first { // empty paragraph: it still takes one line of room
			size := pt
			if sz := num(p.child("endParaRPr"), "sz"); sz > 0 {
				size = sz / 100
			}
			pp.size = size * scale * tc.c.pxPerPt
			pp.runs = nil
		}
		out = append(out, pp)
	}
	// Empty paragraphs only matter between two lines of text.
	for len(out) > 0 && len(out[0].runs) == 0 {
		out = out[1:]
	}
	for len(out) > 0 && len(out[len(out)-1].runs) == 0 {
		out = out[:len(out)-1]
	}
	return out
}

// runElements lists the XML elements readRuns made a run from, in the same
// order, so that both can be walked side by side.
func runElements(p *node) []*node {
	var out []*node
	for _, ch := range p.children {
		switch ch.name {
		case "br":
			out = append(out, ch)
		case "r", "fld":
			if ch.name == "fld" && ch.attr("type") == "slidenum" {
				continue
			}
			if ch.child("t").textOf() == "" {
				continue
			}
			out = append(out, ch)
		}
	}
	return out
}

// background is the colour behind a drawing: the slide's own, else the
// layout's, else the master's, else the theme background.
func (c *slideCtx) background(root *node) string {
	for _, n := range []*node{root, c.li.layout, c.li.master} {
		bg := n.path("cSld", "bg")
		if bg == nil {
			continue
		}
		if col := c.fill(bg.child("bgPr"), nil); col != "" {
			return col
		}
		if col := c.color(bg.child("bgRef")); col != "" {
			return col
		}
	}
	return c.themeColor("bg1")
}

// themeColor resolves a logical colour (tx1, bg1, accent3…) to CSS.
func (c *slideCtx) themeColor(slot string) string {
	if m, ok := c.li.clrMap[slot]; ok {
		slot = m
	} else if m, ok := defaultClrMap[slot]; ok {
		slot = m
	}
	if hex := c.d.theme[slot]; len(hex) == 6 {
		return "#" + strings.ToLower(hex)
	}
	return ""
}

// guessTitle names a drawing that has no title placeholder: a shape that
// PowerPoint still calls "Title…", else the biggest short text near the top.
func (c *slideCtx) guessTitle() string {
	if c.namedTitle != "" {
		return c.namedTitle
	}
	best, bestSize := "", 0.0
	for _, e := range c.s.canvas {
		if e.kind != elemShape || len(e.paras) != 1 || e.y > c.height*0.3 {
			continue
		}
		text := titleText(e.paras)
		if text == "" || len([]rune(text)) > 90 {
			continue
		}
		if e.paras[0].size > bestSize {
			best, bestSize = text, e.paras[0].size
		}
	}
	return best
}

func f1(v float64) string {
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64)
}

func px(v float64) string { return f1(v) + "px" }
