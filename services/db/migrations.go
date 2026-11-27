// Package dbfiles embeds the sales schema migrations (owned by the Go side, P2) so every binary
// knows the schema version it expects and cmd/migrate can apply them.
package dbfiles

import (
	"embed"
	"io/fs"
)

//go:embed migrations/*.sql
var embedded embed.FS

// Sales returns the sales migrations as a flat directory.
func Sales() fs.FS {
	sub, err := fs.Sub(embedded, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}
