package deck

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The extractors below write an archive to a temporary directory so that the
// rest of the package can treat every deck as a directory. Extracted files
// are private to the process (0700/0600): they are only ever read by it. They are the
// security boundary for untrusted uploads (hub) and downloads (CLI):
//
//   - every entry name is checked with safeRelPath (no "..", no absolute path,
//     no backslash) and the final path is re-checked to be inside the target;
//   - symlinks, devices and other special entries are refused;
//   - the total uncompressed size and the number of files are capped while
//     extracting, so a zip bomb stops early instead of filling the disk;
//   - a single top-level directory (what you get when you zip a folder) is
//     stripped, so "talk.zip" containing "talk/01.html" works as expected.

func (ld *loader) extractZip(path string) (string, func() error, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", nil, &Report{Errors: []string{fmt.Sprintf("cannot read zip %q: %v", path, err)}}
	}
	defer func() { _ = zr.Close() }()

	dest, cleanup, err := tempDir()
	if err != nil {
		return "", nil, &Report{Errors: []string{err.Error()}}
	}
	ex := &extractor{dest: dest, limits: ld.limits}
	for _, f := range zr.File {
		mode := f.Mode()
		if err := ex.entry(f.Name, mode.IsDir(), mode.IsRegular(), int64(f.UncompressedSize64), func() (io.ReadCloser, error) { return f.Open() }); err != nil {
			_ = cleanup()
			return "", nil, err
		}
	}
	root := ex.root()
	return root, cleanup, nil
}

func (ld *loader) extractTarGz(path string) (string, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil, &Report{Errors: []string{fmt.Sprintf("cannot read %q: %v", path, err)}}
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", nil, &Report{Errors: []string{fmt.Sprintf("%q is not a valid gzip file: %v", path, err)}}
	}
	defer func() { _ = gz.Close() }()

	dest, cleanup, err := tempDir()
	if err != nil {
		return "", nil, &Report{Errors: []string{err.Error()}}
	}
	ex := &extractor{dest: dest, limits: ld.limits}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = cleanup()
			return "", nil, &Report{Errors: []string{fmt.Sprintf("%q is not a valid tar archive: %v", path, err)}}
		}
		isDir := hdr.Typeflag == tar.TypeDir
		isReg := hdr.Typeflag == tar.TypeReg
		if err := ex.entry(hdr.Name, isDir, isReg, hdr.Size, func() (io.ReadCloser, error) { return io.NopCloser(tr), nil }); err != nil {
			_ = cleanup()
			return "", nil, err
		}
	}
	return ex.root(), cleanup, nil
}

type extractor struct {
	dest   string
	limits Limits
	total  int64
	nfiles int
	tops   map[string]bool // top-level names seen, to detect a single wrapping folder
}

// entry validates and writes one archive entry.
func (ex *extractor) entry(name string, isDir, isReg bool, size int64, open func() (io.ReadCloser, error)) error {
	clean := strings.TrimPrefix(filepath.ToSlash(name), "./")
	clean = strings.TrimSuffix(clean, "/")
	if clean == "" || clean == "." {
		return nil
	}
	// OS litter inside archives is skipped, not refused.
	for _, seg := range strings.Split(clean, "/") {
		if isJunk(seg) {
			return nil
		}
	}
	if !safeRelPath(clean) {
		return &Report{Errors: []string{fmt.Sprintf("archive entry %q refused: path traversal or absolute path", name)}}
	}
	target := filepath.Join(ex.dest, filepath.FromSlash(clean))
	rel, err := filepath.Rel(ex.dest, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return &Report{Errors: []string{fmt.Sprintf("archive entry %q refused: escapes the deck", name)}}
	}
	if ex.tops == nil {
		ex.tops = map[string]bool{}
	}
	ex.tops[strings.SplitN(clean, "/", 2)[0]] = true

	switch {
	case isDir:
		return os.MkdirAll(target, 0o700)
	case !isReg:
		return &Report{Errors: []string{fmt.Sprintf("archive entry %q refused: only regular files and directories are allowed (no symlinks)", name)}}
	}

	ex.nfiles++
	if ex.nfiles > ex.limits.MaxFiles {
		return &Report{Errors: []string{fmt.Sprintf("too many files in archive (max %d)", ex.limits.MaxFiles)}}
	}
	if size > ex.limits.MaxBytes-ex.total {
		return &Report{Errors: []string{fmt.Sprintf("archive too large: more than %s uncompressed", humanBytes(ex.limits.MaxBytes))}}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return &Report{Errors: []string{err.Error()}}
	}
	rc, err := open()
	if err != nil {
		return &Report{Errors: []string{fmt.Sprintf("archive entry %q: %v", name, err)}}
	}
	defer func() { _ = rc.Close() }()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return &Report{Errors: []string{err.Error()}}
	}
	// LimitReader: the header size is not trusted, the real bytes are counted.
	n, err := io.Copy(out, io.LimitReader(rc, ex.limits.MaxBytes-ex.total+1))
	_ = out.Close()
	if err != nil {
		return &Report{Errors: []string{fmt.Sprintf("archive entry %q: %v", name, err)}}
	}
	ex.total += n
	if ex.total > ex.limits.MaxBytes {
		return &Report{Errors: []string{fmt.Sprintf("archive too large: more than %s uncompressed", humanBytes(ex.limits.MaxBytes))}}
	}
	return nil
}

// root returns the directory to load: dest, or dest/<folder> when the archive
// wraps everything in a single folder.
func (ex *extractor) root() string {
	if len(ex.tops) == 1 {
		for top := range ex.tops {
			p := filepath.Join(ex.dest, top)
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				return p
			}
		}
	}
	return ex.dest
}

func tempDir() (string, func() error, error) {
	dir, err := os.MkdirTemp("", "deckhand-deck-")
	if err != nil {
		return "", nil, fmt.Errorf("cannot create temporary directory: %w", err)
	}
	return dir, func() error { return os.RemoveAll(dir) }, nil
}
