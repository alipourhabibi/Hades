// Package sqlitemigrations exposes the numbered SQLite migration files as an
// embedded filesystem so the server can apply them at startup.
//
// The files stay in migration/sqlite/ rather than being copied next to the
// storage code so that both backends keep their migrations in one documented
// place (see CLAUDE.md). PostgreSQL applies the sibling migration/ files with
// golang-migrate; SQLite applies these with the runner in
// internal/hades/storage/db.
package sqlitemigrations

import "embed"

// FS holds every NNN_name.up.sql migration, applied in ascending numeric order.
//
//go:embed *.up.sql
var FS embed.FS
