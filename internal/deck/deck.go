// Package deck loads and validates Deckhand decks.
//
// A deck is a directory (or a .zip / .tar.gz of that directory) containing one
// HTML file per slide and an optional deck.json. The full format is documented
// in docs/FORMAT.md. This package is the only place that knows the format:
// the local server, the hub and the CLI all go through Load.
package deck

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Limits are the hard limits a deck must respect. They come from the brief
// (200 MB, 500 slides) and can be lowered in tests or by the hub config.
type Limits struct {
	MaxBytes  int64 // total size of all files in the deck
	MaxSlides int   // number of slides
	MaxFiles  int   // number of files (archive bombs)
}

// DefaultLimits are the production values.
var DefaultLimits = Limits{
	MaxBytes:  200 << 20, // 200 MB
	MaxSlides: 500,
	MaxFiles:  5000,
}

// Slide is one slide of a deck.
type Slide struct {
	Index  int    `json:"index"`            // 0-based position in the deck
	File   string `json:"file"`             // path relative to the deck root, forward slashes
	Notes  string `json:"notes,omitempty"`  // presenter notes (may be empty)
	Public bool   `json:"public,omitempty"` // notes are shown to viewers too
}

// Deck is a loaded, validated deck.
type Deck struct {
	Title  string  `json:"title"`
	Ratio  string  `json:"ratio"`  // "16:9", "16:10" or "4:3"
	Width  int     `json:"width"`  // design width in CSS pixels
	Height int     `json:"height"` // derived from Width and Ratio
	Slides []Slide `json:"slides"`

	// Root is the directory the deck files are served from. For archives it is
	// a temporary directory that Close removes.
	Root     string   `json:"-"`
	Warnings []string `json:"-"` // non-fatal findings, for `deckhand validate`

	cleanup func() error
}

// FS returns a read-only view of the deck files, rooted at Root.
func (d *Deck) FS() fs.FS { return os.DirFS(d.Root) }

// Close releases the temporary directory of an archive deck. Safe to call on
// a directory deck (no-op) and more than once.
func (d *Deck) Close() error {
	if d == nil || d.cleanup == nil {
		return nil
	}
	c := d.cleanup
	d.cleanup = nil
	return c()
}

// Option tunes Load.
type Option func(*loader)

// WithLimits overrides DefaultLimits.
func WithLimits(l Limits) Option { return func(ld *loader) { ld.limits = l } }

type loader struct {
	limits Limits
}

// Load opens a deck from a directory, a .zip or a .tar.gz.
//
// On success the returned deck is valid: every slide file exists, every file
// has an allowed extension, limits are respected. On failure the error is a
// *Report listing every problem found (not just the first one), so that
// `deckhand validate` can print them all. Callers that only need a yes/no can
// treat it as a plain error.
func Load(path string, opts ...Option) (*Deck, error) {
	ld := &loader{limits: DefaultLimits}
	for _, o := range opts {
		o(ld)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, &Report{Errors: []string{fmt.Sprintf("cannot open %q: %v", path, err)}}
	}

	if info.IsDir() {
		return ld.loadDir(path, nil)
	}

	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		root, cleanup, err := ld.extractZip(path)
		if err != nil {
			return nil, err
		}
		return ld.loadDir(root, cleanup)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		root, cleanup, err := ld.extractTarGz(path)
		if err != nil {
			return nil, err
		}
		return ld.loadDir(root, cleanup)
	default:
		return nil, &Report{Errors: []string{fmt.Sprintf("%q is neither a directory, a .zip nor a .tar.gz", path)}}
	}
}

// loadDir does the real work once the files are on disk.
func (ld *loader) loadDir(root string, cleanup func() error) (*Deck, error) {
	rep := &Report{}
	fail := func() (*Deck, error) {
		if cleanup != nil {
			_ = cleanup()
		}
		return nil, rep
	}

	// 1. Walk the tree: allowed extensions, total size, file count, root HTML files.
	var total int64
	var nfiles int
	var rootHTML []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && isJunk(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isJunk(d.Name()) {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s: symbolic links are not allowed in a deck", rel))
			return nil
		}
		nfiles++
		fi, err := d.Info()
		if err != nil {
			return err
		}
		total += fi.Size()
		if rel != "deck.json" && !Allowed(d.Name()) {
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s: file type not allowed (allowed: %s)", rel, AllowedList()))
		}
		if !strings.Contains(rel, "/") && isHTML(d.Name()) {
			rootHTML = append(rootHTML, rel)
		}
		return nil
	})
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("cannot read deck: %v", err))
		return fail()
	}
	if nfiles > ld.limits.MaxFiles {
		rep.Errors = append(rep.Errors, fmt.Sprintf("too many files: %d (max %d)", nfiles, ld.limits.MaxFiles))
	}
	if total > ld.limits.MaxBytes {
		rep.Errors = append(rep.Errors, fmt.Sprintf("deck too large: %s (max %s)", humanBytes(total), humanBytes(ld.limits.MaxBytes)))
	}

	// 2. deck.json (optional).
	d := &Deck{
		Title:   filepath.Base(root),
		Ratio:   "16:9",
		Width:   1920,
		Root:    root,
		cleanup: cleanup,
	}
	manifestPath := filepath.Join(root, "deck.json")
	var manifest *Manifest
	if raw, err := os.ReadFile(manifestPath); err == nil {
		m, perr := ParseManifest(raw)
		if perr != nil {
			rep.Errors = append(rep.Errors, "deck.json: "+perr.Error())
			return fail()
		}
		manifest = m
	} else if !errors.Is(err, fs.ErrNotExist) {
		rep.Errors = append(rep.Errors, fmt.Sprintf("deck.json: %v", err))
		return fail()
	} else {
		d.Warnings = append(d.Warnings, "no deck.json: slides are all *.html at the root, in natural order, without notes")
	}

	if manifest != nil {
		if manifest.Title != "" {
			d.Title = manifest.Title
		}
		if manifest.Ratio != "" {
			d.Ratio = manifest.Ratio
		}
		if manifest.Width != 0 {
			d.Width = manifest.Width
		}
	}
	h, err := HeightFor(d.Ratio, d.Width)
	if err != nil {
		rep.Errors = append(rep.Errors, "deck.json: "+err.Error())
	}
	d.Height = h

	// 3. Slides: from the manifest (authoritative) or from the root HTML files.
	if manifest != nil && manifest.Slides != nil {
		for i, s := range manifest.Slides {
			file := strings.TrimSpace(s.File)
			switch {
			case file == "":
				rep.Errors = append(rep.Errors, fmt.Sprintf("deck.json: slides[%d] has no \"file\"", i))
				continue
			case !safeRelPath(file):
				rep.Errors = append(rep.Errors, fmt.Sprintf("deck.json: slides[%d] file %q must be a relative path inside the deck", i, file))
				continue
			case !isHTML(file):
				rep.Errors = append(rep.Errors, fmt.Sprintf("deck.json: slides[%d] file %q is not an .html file", i, file))
				continue
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("deck.json: slides[%d] file %q not found in the deck", i, file))
				continue
			}
			d.Slides = append(d.Slides, Slide{Index: len(d.Slides), File: file, Notes: s.Notes, Public: s.Public})
		}
		if len(manifest.Slides) == 0 {
			rep.Errors = append(rep.Errors, "deck.json: \"slides\" is present but empty")
		}
	} else {
		sort.Sort(natural(rootHTML))
		for _, f := range rootHTML {
			d.Slides = append(d.Slides, Slide{Index: len(d.Slides), File: f})
		}
		if len(rootHTML) == 0 && len(rep.Errors) == 0 {
			rep.Errors = append(rep.Errors, "no slide found: no deck.json and no *.html file at the root of the deck")
		}
	}

	if len(d.Slides) > ld.limits.MaxSlides {
		rep.Errors = append(rep.Errors, fmt.Sprintf("too many slides: %d (max %d)", len(d.Slides), ld.limits.MaxSlides))
	}
	for _, s := range d.Slides {
		if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(s.File))); err == nil && fi.Size() == 0 {
			d.Warnings = append(d.Warnings, fmt.Sprintf("%s: empty file", s.File))
		}
	}

	if len(rep.Errors) > 0 {
		return fail()
	}
	rep.Warnings = d.Warnings
	return d, nil
}

// Manifest is the on-disk deck.json.
type Manifest struct {
	Title  string          `json:"title"`
	Ratio  string          `json:"ratio"`
	Width  int             `json:"width"`
	Slides []ManifestSlide `json:"slides"`
}

// ManifestSlide is one entry of deck.json "slides".
type ManifestSlide struct {
	File   string `json:"file"`
	Notes  string `json:"notes"`
	Public bool   `json:"public"`
}

// ParseManifest decodes deck.json strictly: unknown fields are an error, so a
// typo like "slide" instead of "slides" is caught instead of silently ignored.
func ParseManifest(raw []byte) (*Manifest, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if m.Width < 0 {
		return nil, fmt.Errorf("width must be positive, got %d", m.Width)
	}
	if m.Ratio != "" {
		if _, err := HeightFor(m.Ratio, 1920); err != nil {
			return nil, err
		}
	}
	return &m, nil
}

// HeightFor derives the design height from a ratio and a width.
func HeightFor(ratio string, width int) (int, error) {
	if width <= 0 {
		return 0, fmt.Errorf("width must be positive, got %d", width)
	}
	switch ratio {
	case "16:9":
		return (width*9 + 8) / 16, nil
	case "16:10":
		return (width*10 + 8) / 16, nil
	case "4:3":
		return (width*3 + 2) / 4, nil
	default:
		return 0, fmt.Errorf("unknown ratio %q (use \"16:9\", \"16:10\" or \"4:3\")", ratio)
	}
}

// Report lists everything wrong (Errors) and everything worth knowing
// (Warnings) about a deck. It implements error.
type Report struct {
	Errors   []string
	Warnings []string
}

// Error joins the errors on one line; use the fields for a readable report.
func (r *Report) Error() string {
	if len(r.Errors) == 0 {
		return "deck: no error"
	}
	return "deck: " + strings.Join(r.Errors, "; ")
}

// OK is true when there is no error.
func (r *Report) OK() bool { return len(r.Errors) == 0 }

// AsReport extracts the *Report from an error returned by Load, or wraps a
// plain error into one.
func AsReport(err error) *Report {
	var r *Report
	if errors.As(err, &r) {
		return r
	}
	return &Report{Errors: []string{err.Error()}}
}

// safeRelPath accepts only clean, relative, forward-slash paths that stay
// inside the deck: no "..", no leading "/", no drive letter, no backslash.
func safeRelPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || strings.Contains(p, ":") {
		return false
	}
	p = strings.TrimPrefix(p, "./")
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func isHTML(name string) bool {
	l := strings.ToLower(name)
	return strings.HasSuffix(l, ".html") || strings.HasSuffix(l, ".htm")
}

// isJunk hides OS litter that zip tools add and that nobody wants served.
func isJunk(name string) bool {
	return name == ".DS_Store" || name == "__MACOSX" || name == "Thumbs.db" || strings.HasPrefix(name, "._")
}

func humanBytes(n int64) string {
	const kb, mb = 1 << 10, 1 << 20
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
