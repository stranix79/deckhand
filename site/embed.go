// Package site embeds the landing page (site/index.html, provided as-is) and
// its Open Graph image.
package site

import _ "embed"

// Index is the bilingual landing page.
//
//go:embed index.html
var Index []byte

// Demo is the 15 second recording of the three screens (stage, remote, viewer),
// shown on the landing page. 1600×1040, H.264, no audio.
//
//go:embed demo.mp4
var Demo []byte

// Poster is the first frame of Demo, shown before the video loads or when
// autoplay is refused (low power mode, data saver).
//
//go:embed demo-poster.jpg
var Poster []byte

// OG is the 1200×630 preview image referenced by the Open Graph tags.
//
//go:embed og.png
var OG []byte
