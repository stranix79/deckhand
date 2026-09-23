// Package deckhand is the module root. It only embeds files that live at the
// repository root and that other packages need at run time; a Go embed
// directive cannot reach a parent directory, so this is the one place that
// can carry CHANGELOG.md into the binary (served by the hub at /changelog).
package deckhand

import _ "embed"

// Changelog is CHANGELOG.md as committed, Markdown.
//
//go:embed CHANGELOG.md
var Changelog []byte
