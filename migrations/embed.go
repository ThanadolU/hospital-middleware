// Package migrations embeds the schema SQL.
//
// The declaration lives here rather than in internal/database because go:embed
// cannot reach into a parent directory, and the SQL is worth keeping at a
// top-level path where a reviewer will find it.
package migrations

import "embed"

// FS holds every .sql migration, so the compiled binary carries its own schema
// and nothing has to be copied into the Docker image beside it.
//
//go:embed *.sql
var FS embed.FS
