// Package docs embeds the Markdown documentation rendered by the hub.
package docs

import "embed"

// FS holds FORMAT.md, CLI.md, HUB.md, PROTOCOL.md, SECURITY.md.
//
//go:embed *.md
var FS embed.FS
