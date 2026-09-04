// Package web embeds the three screens (stage, remote, viewer) and their
// shared assets. Plain HTML/CSS/JS, no build step: edit, rebuild, done.
package web

import "embed"

// FS holds stage/, remote/, viewer/ and shared/.
//
//go:embed stage remote viewer shared
var FS embed.FS
