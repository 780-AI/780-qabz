// Package migrations embeds the SQL migration files.
package migrations

import "embed"

// FS holds the *.sql files: NNNNNN_name.up.sql. There are no down files: a
// change is rolled forward with a new migration.
//
//go:embed *.sql
var FS embed.FS
