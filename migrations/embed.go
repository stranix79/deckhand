// Package migrations embeds the SQL migrations of the hub.
package migrations

import "embed"

// FS holds the *.up.sql / *.down.sql files, applied by golang-migrate.
//
//go:embed *.sql
var FS embed.FS
