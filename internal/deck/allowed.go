package deck

import (
	"path/filepath"
	"sort"
	"strings"
)

// allowedExt is the closed list of file types a deck may contain. Anything
// else is rejected at load time (brief §2). Content types are what the servers
// use when they serve the file; .svg is special-cased there (attachment unless
// requested by an <img>).
var allowedExt = map[string]string{
	".html":  "text/html; charset=utf-8",
	".htm":   "text/html; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".json":  "application/json",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".avif":  "image/avif",
	".svg":   "image/svg+xml",
	".ico":   "image/x-icon",
	".mp4":   "video/mp4",
	".webm":  "video/webm",
	".mov":   "video/quicktime",
	".mp3":   "audio/mpeg",
	".ogg":   "audio/ogg",
	".wav":   "audio/wav",
	".m4a":   "audio/mp4",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".pdf":   "application/pdf",
	".txt":   "text/plain; charset=utf-8",
	".md":    "text/markdown; charset=utf-8",
	".vtt":   "text/vtt",
}

// Allowed reports whether a file name has an accepted extension.
func Allowed(name string) bool {
	_, ok := allowedExt[strings.ToLower(filepath.Ext(name))]
	return ok
}

// ContentType returns the MIME type to serve a deck file with, or "" if the
// extension is not allowed.
func ContentType(name string) string {
	return allowedExt[strings.ToLower(filepath.Ext(name))]
}

// AllowedList is the extensions, sorted, for error messages and docs.
func AllowedList() string {
	exts := make([]string, 0, len(allowedExt))
	for e := range allowedExt {
		exts = append(exts, e)
	}
	sort.Strings(exts)
	return strings.Join(exts, " ")
}
