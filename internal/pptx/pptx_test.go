package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stranix79/deckhand/internal/deck"
)

// The fixtures are built in memory, part by part: a .pptx is a zip of XML
// files, and writing them by hand keeps each test readable next to what it
// checks. No binary file in the repository.

const ns = `xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" ` +
	`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

const relType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"

// rels builds a .rels part from "id type target" triples; a target starting
// with http is external.
func xrels(triples ...string) string {
	var b strings.Builder
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, t := range triples {
		f := strings.Fields(t)
		mode := ""
		if strings.HasPrefix(f[2], "http") {
			mode = ` TargetMode="External"`
		}
		b.WriteString(`<Relationship Id="` + f[0] + `" Type="` + relType + f[1] + `" Target="` + f[2] + `"` + mode + `/>`)
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

// base is what every fixture shares: the package, a presentation with the
// given size and slides, a theme, one master and one layout.
func base(size string, slides int) map[string]string {
	ids, presRels := "", []string{"rIdT theme theme/theme1.xml"}
	for i := 1; i <= slides; i++ {
		n := string(rune('0' + i))
		ids += `<p:sldId id="25` + n + `" r:id="rId` + n + `"/>`
		presRels = append(presRels, "rId"+n+" slide slides/slide"+n+".xml")
	}
	return map[string]string{
		"_rels/.rels": xrels("rId1 officeDocument ppt/presentation.xml"),
		"ppt/presentation.xml": `<p:presentation ` + ns + `><p:sldIdLst>` + ids + `</p:sldIdLst><p:sldSz ` + size + `/>` +
			`<p:defaultTextStyle><a:lvl1pPr><a:defRPr sz="1800"/></a:lvl1pPr></p:defaultTextStyle></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": xrels(presRels...),
		"ppt/theme/theme1.xml": `<a:theme ` + ns + `><a:themeElements><a:clrScheme name="x">` +
			`<a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1><a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>` +
			`<a:accent1><a:srgbClr val="156082"/></a:accent1><a:accent2><a:srgbClr val="E97132"/></a:accent2></a:clrScheme>` +
			`<a:fontScheme name="x"><a:minorFont><a:latin typeface="Aptos"/></a:minorFont></a:fontScheme>` +
			`<a:fmtScheme><a:lnStyleLst><a:ln w="12700"/><a:ln w="19050"/></a:lnStyleLst></a:fmtScheme></a:themeElements></a:theme>`,
		"ppt/slideMasters/slideMaster1.xml": `<p:sldMaster ` + ns + `><p:cSld><p:bg><p:bgRef idx="1001"><a:schemeClr val="bg1"/></p:bgRef></p:bg><p:spTree/></p:cSld>` +
			`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2"/></p:sldMaster>`,
		"ppt/slideLayouts/slideLayout1.xml": `<p:sldLayout ` + ns + `><p:cSld><p:spTree>` +
			`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Text"/><p:cNvSpPr/><p:nvPr><p:ph type="body" idx="13"/></p:nvPr></p:nvSpPr><p:spPr/>` +
			`<p:txBody><a:bodyPr/><a:lstStyle><a:lvl1pPr><a:buNone/></a:lvl1pPr></a:lstStyle><a:p/></p:txBody></p:sp>` +
			`</p:spTree></p:cSld></p:sldLayout>`,
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": xrels("rId1 slideMaster ../slideMasters/slideMaster1.xml"),
	}
}

func slideXML(attrs, tree string) string {
	return `<p:sld ` + ns + attrs + `><p:cSld><p:spTree><p:nvGrpSpPr/><p:grpSpPr/>` + tree + `</p:spTree></p:cSld></p:sld>`
}

// ph is a placeholder shape; paras is the content of its text body.
func xph(kind, paras string) string {
	return `<p:sp><p:nvSpPr><p:cNvPr id="2" name="x"/><p:cNvSpPr/><p:nvPr><p:ph ` + kind + `/></p:nvPr></p:nvSpPr><p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:lstStyle/>` + paras + `</p:txBody></p:sp>`
}

func xp(pPr, text string) string {
	return `<a:p>` + pPr + `<a:r><a:rPr lang="fr-BE"/><a:t>` + text + `</a:t></a:r></a:p>`
}

func xopen(t *testing.T, parts map[string]string) *Deck {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	d, err := Read(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return d
}

func xwrite(t *testing.T, d *Deck, opt Options) (dir string, files map[string]string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "deck")
	if err := d.Write(dir, opt); err != nil {
		t.Fatalf("Write: %v", err)
	}
	files = map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = string(raw)
	}
	return dir, files
}

func mustContain(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("%s: missing %q in:\n%s", what, w, got)
		}
	}
}

const widescreen = `cx="12192000" cy="6858000"`

// A deck made the way PowerPoint wants: placeholders, bullets, a picture, a
// table, notes, a hidden slide, and two things that cannot be converted.
func classic() map[string]string {
	p := base(widescreen, 3)
	p["ppt/slides/slide1.xml"] = slideXML("",
		xph(`type="ctrTitle"`, xp("", "Résultats &amp; suite"))+
			xph(`type="subTitle" idx="1"`, xp("", "Novembre 2026")))
	p["ppt/slides/_rels/slide1.xml.rels"] = xrels("rId1 slideLayout ../slideLayouts/slideLayout1.xml", "rId2 notesSlide ../notesSlides/notesSlide1.xml")
	p["ppt/notesSlides/notesSlide1.xml"] = `<p:notes ` + ns + `><p:cSld><p:spTree>` +
		xph(`type="sldImg"`, "") + xph(`type="body" idx="1"`, xp("", "Dire bonjour.")+xp("", "Attendre.")) + `</p:spTree></p:cSld></p:notes>`

	body := xp("", "Migration terminée") +
		xp(`<a:pPr lvl="1"/>`, "Zéro coupure") +
		`<a:p><a:r><a:rPr b="1"/><a:t>Gras</a:t></a:r><a:r><a:rPr><a:latin typeface="Courier New"/></a:rPr><a:t>pg_dump</a:t></a:r>` +
		`<a:r><a:rPr><a:hlinkClick r:id="rId9"/></a:rPr><a:t>le site</a:t></a:r></a:p>` +
		xp(`<a:pPr><a:buNone/></a:pPr>`, "Sans puce")
	pic := func(rid string) string {
		return `<p:pic><p:nvPicPr><p:cNvPr id="5" name="pic" descr="Une photo"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr>` +
			`<p:blipFill><a:blip r:embed="` + rid + `"/></p:blipFill><p:spPr/></p:pic>`
	}
	table := `<p:graphicFrame><p:xfrm><a:off x="0" y="0"/><a:ext cx="100" cy="100"/></p:xfrm><a:graphic><a:graphicData>` +
		`<a:tbl><a:tblPr firstRow="1"/><a:tr><a:tc><a:txBody>` + xp("", "Mois") + `</a:txBody></a:tc><a:tc gridSpan="2"><a:txBody>` + xp("", "Dispo") + `</a:txBody></a:tc><a:tc hMerge="1"><a:txBody><a:p/></a:txBody></a:tc></a:tr>` +
		`<a:tr><a:tc><a:txBody>` + xp("", "Octobre") + `</a:txBody></a:tc><a:tc><a:txBody>` + xp("", "99,99 %") + `</a:txBody></a:tc><a:tc><a:txBody>` + xp("", "ok") + `</a:txBody></a:tc></a:tr></a:tbl>` +
		`</a:graphicData></a:graphic></p:graphicFrame>`
	chart := `<p:graphicFrame><a:graphic><a:graphicData><c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" r:id="rId8"/></a:graphicData></a:graphic></p:graphicFrame>`
	p["ppt/slides/slide2.xml"] = slideXML("",
		xph(`type="title"`, xp("", "Ce qui a marché"))+xph(`idx="1"`, body)+
			xph(`type="sldNum" idx="12"`, xp("", "2"))+pic("rId2")+pic("rId3")+table+chart)
	p["ppt/slides/_rels/slide2.xml.rels"] = xrels("rId1 slideLayout ../slideLayouts/slideLayout1.xml",
		"rId2 image ../media/Image_1.PNG", "rId3 image ../media/drawing.emf", "rId9 hyperlink https://deckhand.show")
	p["ppt/media/Image_1.PNG"] = "png-bytes"
	p["ppt/media/drawing.emf"] = "emf-bytes"

	p["ppt/slides/slide3.xml"] = slideXML(` show="0"`, xph(`type="title"`, xp("", "Cachée")))
	return p
}

func TestClassicDeck(t *testing.T) {
	d := xopen(t, classic())
	if d.Title != "Résultats & suite" || d.Ratio != "16:9" || d.Lang != "fr-BE" {
		t.Errorf("title %q, ratio %q, lang %q", d.Title, d.Ratio, d.Lang)
	}
	if d.Slides() != 2 || d.Hidden != 1 || d.WithNotes() != 1 || d.Assets() != 1 {
		t.Errorf("slides %d, hidden %d, notes %d, assets %d", d.Slides(), d.Hidden, d.WithNotes(), d.Assets())
	}
	warnings := strings.Join(d.Warnings, "\n")
	mustContain(t, "warnings", warnings, "slide 2: chart not converted", "slide 2: picture drawing.emf: emf is not a web image format")

	dir, files := xwrite(t, d, Options{})
	cover := files["01-resultats-suite.html"]
	mustContain(t, "cover", cover, `<html lang="fr-BE">`, `<body class="cover">`, `<h1>Résultats &amp; suite</h1>`, `<p class="lead">Novembre 2026</p>`)

	content := files["02-ce-qui-a-marche.html"]
	mustContain(t, "content", content,
		"<h2>Ce qui a marché</h2>",
		"<li>Migration terminée\n", "<li>Zéro coupure</li>", // nested list
		`<strong>Gras</strong><code>pg_dump</code><a href="https://deckhand.show">le site</a>`,
		"<p>Sans puce</p>",
		`<img src="assets/image_1.png" alt="Une photo">`,
		`<tr><th>Mois</th><th colspan="2">Dispo</th></tr>`, "<td>99,99 %</td>",
		`<div class="missing">chart not converted`,
		`class="cols"`)
	if strings.Contains(content, ">2<") {
		t.Errorf("the slide number placeholder must be dropped:\n%s", content)
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "assets", "image_1.png")); err != nil || string(raw) != "png-bytes" {
		t.Errorf("picture not copied: %v %q", err, raw)
	}

	var manifest struct {
		Title  string
		Ratio  string
		Width  int
		Slides []struct{ File, Notes string }
	}
	if err := json.Unmarshal([]byte(files["deck.json"]), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Title != "Résultats & suite" || len(manifest.Slides) != 2 || manifest.Slides[0].Notes != "Dire bonjour.\nAttendre." {
		t.Errorf("deck.json: %+v", manifest)
	}

	// The real judge: the loader every other command goes through.
	loaded, err := deck.Load(dir)
	if err != nil {
		t.Fatalf("the converted deck does not load: %v", err)
	}
	defer func() { _ = loaded.Close() }()
	if len(loaded.Slides) != 2 || loaded.Height != 1080 || loaded.Slides[0].Notes == "" {
		t.Errorf("loaded deck: %+v", loaded)
	}
}

func TestLayoutSaysNoBullets(t *testing.T) {
	p := base(widescreen, 1)
	p["ppt/slides/slide1.xml"] = slideXML("", xph(`type="title"`, xp("", "Citation"))+xph(`type="body" idx="13"`, xp("", "Une phrase, pas une liste")))
	p["ppt/slides/_rels/slide1.xml.rels"] = xrels("rId1 slideLayout ../slideLayouts/slideLayout1.xml")
	_, files := xwrite(t, xopen(t, p), Options{Dark: true})
	page := files["01-citation.html"]
	mustContain(t, "page", page, `<html lang="fr-BE" class="dark">`, "<p>Une phrase, pas une liste</p>")
	if strings.Contains(page, "<li>") {
		t.Errorf("the layout placeholder has buNone, no list expected:\n%s", page)
	}
}

// A slide drawn by hand: two boxes and an arrow. It must keep its geometry
// and the colours of the theme.
func TestDrawing(t *testing.T) {
	shape := func(name, x, y, extra, text string) string {
		return `<p:sp><p:nvSpPr><p:cNvPr id="3" name="` + name + `"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
			`<p:spPr><a:xfrm><a:off x="` + x + `" y="` + y + `"/><a:ext cx="1219200" cy="609600"/></a:xfrm><a:prstGeom prst="roundRect"/>` + extra + `</p:spPr>` +
			`<p:style><a:lnRef idx="2"><a:schemeClr val="accent2"/></a:lnRef><a:fillRef idx="1"><a:schemeClr val="accent2"/></a:fillRef><a:fontRef idx="minor"><a:schemeClr val="lt1"/></a:fontRef></p:style>` +
			`<p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr sz="2400" b="1"/><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp>`
	}
	arrow := `<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="9" name="Arrow"/><p:cNvCxnSpPr/><p:nvPr/></p:nvCxnSpPr>` +
		`<p:spPr><a:xfrm><a:off x="1219200" y="304800"/><a:ext cx="1219200" cy="0"/></a:xfrm><a:prstGeom prst="straightConnector1"/>` +
		`<a:ln w="38100"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill><a:prstDash val="dash"/><a:tailEnd type="triangle"/></a:ln></p:spPr></p:cxnSp>`
	p := base(widescreen, 1)
	p["ppt/slides/slide1.xml"] = slideXML("",
		shape("Title 1", "0", "0", `<a:solidFill><a:schemeClr val="accent1"><a:lumMod val="50000"/></a:schemeClr></a:solidFill>`, "Source")+
			arrow+shape("Box", "2438400", "0", "", "Cible"))
	p["ppt/slides/_rels/slide1.xml.rels"] = xrels("rId1 slideLayout ../slideLayouts/slideLayout1.xml")

	d := xopen(t, p)
	_, files := xwrite(t, d, Options{Dark: true})
	page := files["01-source.html"] // named after the shape PowerPoint calls "Title"
	mustContain(t, "drawing", page,
		`<html lang="en">`, // a drawing keeps its own colours, even with --dark
		`<body class="canvas" style="background:#ffffff;color:#000000">`,
		// 1219200 EMU on a 12192000-wide slide is a tenth of 1920 px.
		`left:0px;top:0px;width:192px;height:96px;background:#0b3041;border:3px solid #e97132;border-radius:16px`,
		`left:384px;top:0px;width:192px;height:96px;background:#e97132`,
		`justify-content:center;color:#ffffff`,
		`<p style="font-size:48px;text-align:center"><strong>Source</strong></p>`,
		`<svg class="ln">`, `stroke="#ff0000" stroke-width="6" stroke-dasharray="24 18"`,
		`<polygon points="384,48 `)
	mustContain(t, "deck.json", files["deck.json"], `"file": "01-source.html"`)
}

func TestRatiosAndStylesheet(t *testing.T) {
	for size, want := range map[string]struct {
		ratio  string
		height string
	}{
		`cx="9144000" cy="6858000"`:  {"4:3", "height: 1440px"},
		`cx="9144000" cy="5715000"`:  {"16:10", "height: 1200px"},
		`cx="18288000" cy="9144000"`: {"16:9", "height: 1080px"}, // 2:1, closest is 16:9
	} {
		p := base(size, 1)
		p["ppt/slides/slide1.xml"] = slideXML("", xph(`type="title"`, xp("", "Un")))
		d := xopen(t, p)
		if d.Ratio != want.ratio {
			t.Errorf("%s: ratio %q, want %q", size, d.Ratio, want.ratio)
		}
		dir, _ := xwrite(t, d, Options{})
		css, err := os.ReadFile(filepath.Join(dir, "assets", "style.css"))
		if err != nil {
			t.Fatal(err)
		}
		mustContain(t, "style.css", string(css), want.height, `font-family: "Aptos", Calibri`)
		if strings.Contains(string(css), "{{") {
			t.Errorf("a template marker is left in style.css")
		}
	}
}

func TestWriteNeverClobbers(t *testing.T) {
	p := base(widescreen, 1)
	p["ppt/slides/slide1.xml"] = slideXML("", xph(`type="title"`, xp("", "Un")))
	d := xopen(t, p)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mine.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.Write(dir, Options{}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("writing into a non-empty directory must be refused, got %v", err)
	}
	if err := d.Write(dir, Options{Force: true, Title: "fallback"}); err != nil {
		t.Fatalf("Write with Force: %v", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "mine.txt")); string(raw) != "keep" {
		t.Errorf("Force must not delete existing files")
	}
}

func TestNotAPowerPoint(t *testing.T) {
	junk := []byte("definitely not a zip")
	if _, err := Read(bytes.NewReader(junk), int64(len(junk))); err == nil {
		t.Error("random bytes must be refused")
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	_, _ = w.Write([]byte("<x/>"))
	_ = zw.Close()
	if _, err := Read(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err == nil || !strings.Contains(err.Error(), "not a PowerPoint file") {
		t.Errorf("a zip without a presentation must be refused, got %v", err)
	}

	p := base(widescreen, 1)
	p["ppt/slides/slide1.xml"] = slideXML(` show="0"`, xph(`type="title"`, xp("", "Cachée")))
	var only bytes.Buffer
	zw = zip.NewWriter(&only)
	for name, content := range p {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(content))
	}
	_ = zw.Close()
	if _, err := Read(bytes.NewReader(only.Bytes()), int64(only.Len())); err == nil || !strings.Contains(err.Error(), "no slide") {
		t.Errorf("a deck with only hidden slides must be refused, got %v", err)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Résultats du trimestre": "resultats-du-trimestre",
		"  Q&A / Questions ?  ":  "q-a-questions",
		"":                       "slide",
		"日本語":                    "slide",
		"A very long title that goes on and on and on forever": "a-very-long-title-that-goes-on-and-on-an",
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestColorModifiers(t *testing.T) {
	d := &Deck{theme: map[string]string{"dk1": "000000", "lt1": "FFFFFF", "accent1": "156082"}}
	for xml, want := range map[string]string{
		`<a:srgbClr val="FF8000"/>`:                                   "#ff8000",
		`<a:schemeClr val="tx1"/>`:                                    "#000000",
		`<a:schemeClr val="bg1"><a:alpha val="50000"/></a:schemeClr>`: "rgba(255,255,255,0.5)",
		`<a:srgbClr val="808080"><a:shade val="50000"/></a:srgbClr>`:  "#404040",
		`<a:srgbClr val="000000"><a:tint val="50000"/></a:srgbClr>`:   "#808080",
		`<a:prstClr val="red"/>`:                                      "#ff0000",
	} {
		holder, err := parseXML(strings.NewReader(`<a:solidFill ` + ns + `>` + xml + `</a:solidFill>`))
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := d.colorOf(holder, nil); !ok || got != want {
			t.Errorf("%s = %q, want %q", xml, got, want)
		}
	}
	// A dark template maps "text 1" to the light slot.
	holder, _ := parseXML(strings.NewReader(`<a:solidFill ` + ns + `><a:schemeClr val="tx1"/></a:solidFill>`))
	if got, _ := d.colorOf(holder, map[string]string{"tx1": "lt1"}); got != "#ffffff" {
		t.Errorf("clrMap not applied: %q", got)
	}
}
