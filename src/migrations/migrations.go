// Package migrations embeds the SQL migration files.
package migrations

import "embed"

// FS holds the *.sql files: NNNNNN_name.up.sql and NNNNNN_name.down.sql.
//
//go:embed *.sql
var FS embed.FS
