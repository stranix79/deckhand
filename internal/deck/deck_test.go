package deck

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// --- helpers ---------------------------------------------------------------

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// makeZip builds a zip in a temp dir from name→content, keeping entry names
// verbatim (so tests can put "../" in them).
func makeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entries[n])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "deck.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func makeTarGz(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for n, c := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(c)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(c)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "deck.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const slide = "<!doctype html><html><body><h1>hi</h1></body></html>"

// --- natural sort -------------------------------------------------------------

func TestNaturalLess(t *testing.T) {
	in := []string{"10-end.html", "2-intro.html", "1-title.html", "02-b.html", "a.html", "B.html", "1-title2.html"}
	want := []string{"1-title.html", "1-title2.html", "02-b.html", "2-intro.html", "10-end.html", "a.html", "B.html"}
	sort.Sort(natural(in))
	if strings.Join(in, ",") != strings.Join(want, ",") {
		t.Fatalf("natural sort\n got %v\nwant %v", in, want)
	}
}

// --- ratios ---------------------------------------------------------------------

func TestHeightFor(t *testing.T) {
	cases := []struct {
		ratio string
		w, h  int
	}{{"16:9", 1920, 1080}, {"16:10", 1920, 1200}, {"4:3", 1920, 1440}, {"16:9", 1280, 720}}
	for _, c := range cases {
		h, err := HeightFor(c.ratio, c.w)
		if err != nil || h != c.h {
			t.Errorf("HeightFor(%s, %d) = %d, %v; want %d", c.ratio, c.w, h, err, c.h)
		}
	}
	if _, err := HeightFor("21:9", 1920); err == nil {
		t.Error("unknown ratio accepted")
	}
}

// --- directories -----------------------------------------------------------------

func TestLoadDirWithoutManifest(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"10-end.html", "2-body.html", "1-title.html"} {
		writeFile(t, root, n, slide)
	}
	writeFile(t, root, "assets/style.css", "body{}")
	writeFile(t, root, ".DS_Store", "junk")

	d, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer func() { _ = d.Close() }()
	got := []string{}
	for _, s := range d.Slides {
		got = append(got, s.File)
	}
	want := "1-title.html,2-body.html,10-end.html"
	if strings.Join(got, ",") != want {
		t.Fatalf("slides = %v, want %s", got, want)
	}
	if d.Ratio != "16:9" || d.Width != 1920 || d.Height != 1080 {
		t.Fatalf("defaults: %s %dx%d", d.Ratio, d.Width, d.Height)
	}
	if len(d.Warnings) == 0 || !strings.Contains(d.Warnings[0], "no deck.json") {
		t.Fatalf("expected a no-deck.json warning, got %v", d.Warnings)
	}
}

func TestLoadDirWithManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "01-title.html", slide)
	writeFile(t, root, "02-skip.html", slide)
	writeFile(t, root, "03-demo.html", slide)
	writeFile(t, root, "deck.json", `{
	  "title": "Ship it",
	  "ratio": "4:3",
	  "width": 1600,
	  "slides": [
	    {"file": "01-title.html", "notes": "wait", "public": true},
	    {"file": "03-demo.html", "notes": "two minutes"}
	  ]
	}`)
	d, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if d.Title != "Ship it" || d.Ratio != "4:3" || d.Width != 1600 || d.Height != 1200 {
		t.Fatalf("manifest not applied: %+v", d)
	}
	if len(d.Slides) != 2 || d.Slides[0].File != "01-title.html" || d.Slides[1].File != "03-demo.html" {
		t.Fatalf("slides = %+v", d.Slides)
	}
	if !d.Slides[0].Public || d.Slides[0].Notes != "wait" || d.Slides[1].Public {
		t.Fatalf("notes/public not applied: %+v", d.Slides)
	}
	if d.Slides[1].Index != 1 {
		t.Fatalf("index not renumbered: %+v", d.Slides[1])
	}
}

func TestManifestMissingFileIsNamed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "01-title.html", slide)
	writeFile(t, root, "deck.json", `{"slides":[{"file":"01-title.html"},{"file":"99-missing.html"}]}`)
	_, err := Load(root)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "99-missing.html") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error should name the missing file: %v", err)
	}
}

func TestManifestUnknownFieldAndBadRatio(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.html", slide)
	writeFile(t, root, "deck.json", `{"slide":[{"file":"a.html"}]}`)
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("typo in deck.json must be reported, got %v", err)
	}
	writeFile(t, root, "deck.json", `{"ratio":"21:9"}`)
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "ratio") {
		t.Fatalf("bad ratio must be reported, got %v", err)
	}
}

func TestManifestPathTraversal(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.html", slide)
	writeFile(t, root, "deck.json", `{"slides":[{"file":"../a.html"}]}`)
	_, err := Load(root)
	if err == nil || !strings.Contains(err.Error(), "relative path inside the deck") {
		t.Fatalf("traversal in deck.json must be refused, got %v", err)
	}
}

func TestDisallowedExtensionAndLimits(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.html", slide)
	writeFile(t, root, "tool.exe", "MZ")
	_, err := Load(root)
	if err == nil || !strings.Contains(err.Error(), "tool.exe") || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("exe must be refused, got %v", err)
	}
	if err := os.Remove(filepath.Join(root, "tool.exe")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "b.html", slide)
	writeFile(t, root, "c.html", slide)
	_, err = Load(root, WithLimits(Limits{MaxBytes: 1 << 20, MaxSlides: 2, MaxFiles: 100}))
	if err == nil || !strings.Contains(err.Error(), "too many slides") {
		t.Fatalf("slide limit, got %v", err)
	}
	_, err = Load(root, WithLimits(Limits{MaxBytes: 10, MaxSlides: 500, MaxFiles: 100}))
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("size limit, got %v", err)
	}
}

func TestReportCollectsEverything(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.html", slide)
	writeFile(t, root, "x.exe", "MZ")
	writeFile(t, root, "deck.json", `{"slides":[{"file":"nope.html"},{"file":"a.html"}]}`)
	_, err := Load(root)
	rep := AsReport(err)
	if len(rep.Errors) != 2 {
		t.Fatalf("want 2 errors (exe + missing), got %v", rep.Errors)
	}
}

// --- archives ----------------------------------------------------------------------

func TestLoadZipWithWrappingFolder(t *testing.T) {
	p := makeZip(t, map[string]string{
		"talk/1-a.html":            slide,
		"talk/2-b.html":            slide,
		"talk/assets/style.css":    "body{}",
		"__MACOSX/talk/._1-a.html": "junk",
	})
	d, err := Load(p)
	if err != nil {
		t.Fatalf("Load zip: %v", err)
	}
	defer func() { _ = d.Close() }()
	if len(d.Slides) != 2 || d.Slides[0].File != "1-a.html" {
		t.Fatalf("slides = %+v", d.Slides)
	}
	if _, err := os.Stat(filepath.Join(d.Root, "assets", "style.css")); err != nil {
		t.Fatalf("assets not extracted: %v", err)
	}
	root := d.Root
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("Close must remove the temporary directory")
	}
}

func TestZipSlipRefused(t *testing.T) {
	p := makeZip(t, map[string]string{"1-a.html": slide, "../evil.html": slide})
	_, err := Load(p)
	if err == nil {
		t.Fatal("zip slip must be refused")
	}
	if !strings.Contains(err.Error(), "../evil.html") || !strings.Contains(err.Error(), "path traversal") {
		t.Fatalf("error must name the entry and the reason: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p), "evil.html")); err == nil {
		t.Fatal("evil.html was written outside the deck")
	}
}

func TestZipAbsoluteAndBackslashRefused(t *testing.T) {
	for _, bad := range []string{"/etc/passwd.html", `dir\..\x.html`, "c:/x.html"} {
		p := makeZip(t, map[string]string{"1-a.html": slide, bad: slide})
		if _, err := Load(p); err == nil {
			t.Errorf("entry %q must be refused", bad)
		}
	}
}

func TestZipBombStopsEarly(t *testing.T) {
	big := strings.Repeat("x", 4096)
	p := makeZip(t, map[string]string{"1-a.html": slide, "assets/big.txt": big})
	_, err := Load(p, WithLimits(Limits{MaxBytes: 1024, MaxSlides: 10, MaxFiles: 10}))
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("size limit during extraction, got %v", err)
	}
}

func TestLoadTarGz(t *testing.T) {
	p := makeTarGz(t, map[string]string{
		"2-b.html":  slide,
		"1-a.html":  slide,
		"deck.json": `{"title":"tarred","slides":[{"file":"2-b.html","notes":"n"},{"file":"1-a.html"}]}`,
	})
	d, err := Load(p)
	if err != nil {
		t.Fatalf("Load tar.gz: %v", err)
	}
	defer func() { _ = d.Close() }()
	if d.Title != "tarred" || len(d.Slides) != 2 || d.Slides[0].File != "2-b.html" {
		t.Fatalf("deck = %+v", d)
	}
}

func TestUnknownFormat(t *testing.T) {
	p := filepath.Join(t.TempDir(), "deck.rar")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "neither") {
		t.Fatalf("unknown format, got %v", err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing path must fail")
	}
}
