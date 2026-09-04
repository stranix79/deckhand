// Package site embeds the landing page (site/index.html, provided as-is) and
// its Open Graph image.
package site

import _ "embed"

// Index is the bilingual landing page.
//
//go:embed index.html
var Index []byte

// OG is the 1200×630 preview image referenced by the Open Graph tags.
//
//go:embed og.png
var OG []byte
