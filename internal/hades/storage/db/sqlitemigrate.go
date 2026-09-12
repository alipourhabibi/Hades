package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	sqlitemigrations "github.com/alipourhabibi/Hades/migration/sqlite"
)

// legacyBaselineVersion is the migration version that a database created by the
// pre-migration startup code is already at.
//
// That code executed the full embedded schema plus two ALTER TABLE statements
// adding modules.lint_preset and modules.breaking_enabled, which is exactly the
// state reached by migrations 001 and 002. Such a database is stamped at this
// version instead of having them re-run, because replaying 002 against a table
// that already has the columns would fail.
const legacyBaselineVersion = 2

// sqliteMigration is one embedded migration file.
type sqliteMigration struct {
	version int
	name    string
	body    string
}

// migrateSQLite brings db up to the latest schema version.
//
// This is the only mechanism that changes the SQLite schema. It replaces the
// previous arrangement, where startup executed a full embedded schema and then
// a hardcoded list of ALTER statements whose errors were matched by substring:
// that could not tell a migration that had already been applied from one that
// had genuinely failed, and it left no record of what a given database had run.
func migrateSQLite(sqlDB *sql.DB) error {
	migrations, err := loadSQLiteMigrations()
	if err != nil {
		return err
	}

	fresh, err := ensureSchemaMigrationsTable(sqlDB)
	if err != nil {
		return err
	}

	// A database that predates this runner has no version table but does have
	// the schema. Stamp it rather than replaying migrations it already ran.
	if !fresh {
		if err := stampLegacyBaseline(sqlDB); err != nil {
			return err
		}
	}

	applied, err := appliedVersions(sqlDB)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		if err := applySQLiteMigration(sqlDB, m); err != nil {
			return err
		}
	}
	return nil
}

// loadSQLiteMigrations reads the embedded migration files in ascending version
// order.
func loadSQLiteMigrations() ([]sqliteMigration, error) {
	entries, err := fs.ReadDir(sqlitemigrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("db: sqlite: read migrations: %w", err)
	}

	out := make([]sqliteMigration, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		prefix, _, found := strings.Cut(e.Name(), "_")
		if !found {
			return nil, fmt.Errorf("db: sqlite: migration %q is not named NNN_description.up.sql", e.Name())
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("db: sqlite: migration %q has a non-numeric version prefix: %w", e.Name(), err)
		}
		body, err := fs.ReadFile(sqlitemigrations.FS, e.Name())
		if err != nil {
			return nil, fmt.Errorf("db: sqlite: read migration %q: %w", e.Name(), err)
		}
		out = append(out, sqliteMigration{version: version, name: e.Name(), body: string(body)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i := 1; i < len(out); i++ {
		if out[i].version == out[i-1].version {
			return nil, fmt.Errorf("db: sqlite: duplicate migration version %d (%q and %q)",
				out[i].version, out[i-1].name, out[i].name)
		}
	}
	return out, nil
}

// ensureSchemaMigrationsTable creates the version table if absent and reports
// whether the database is brand new (no user tables yet).
func ensureSchemaMigrationsTable(sqlDB *sql.DB) (fresh bool, err error) {
	var existing string
	err = sqlDB.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`,
	).Scan(&existing)
	hadVersionTable := err == nil
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("db: sqlite: inspect schema_migrations: %w", err)
	}

	if _, err := sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		return false, fmt.Errorf("db: sqlite: create schema_migrations: %w", err)
	}
	if hadVersionTable {
		return true, nil // already managed by this runner
	}

	// No version table. Fresh if there is no schema at all, legacy otherwise.
	var users string
	err = sqlDB.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'users'`,
	).Scan(&users)
	switch {
	case err == sql.ErrNoRows:
		return true, nil
	case err != nil:
		return false, fmt.Errorf("db: sqlite: inspect users table: %w", err)
	default:
		return false, nil
	}
}

// stampLegacyBaseline records the migrations a pre-runner database already has.
func stampLegacyBaseline(sqlDB *sql.DB) error {
	for v := 1; v <= legacyBaselineVersion; v++ {
		if _, err := sqlDB.Exec(
			`INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, v,
		); err != nil {
			return fmt.Errorf("db: sqlite: stamp baseline version %d: %w", v, err)
		}
	}
	return nil
}

func appliedVersions(sqlDB *sql.DB) (map[int]bool, error) {
	rows, err := sqlDB.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("db: sqlite: read applied migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("db: sqlite: scan applied migration: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// applySQLiteMigration runs one migration and records its version.
//
// The body and the version row are written in a single transaction, so a
// migration can never be recorded as applied unless it fully succeeded.
//
// Foreign key enforcement is owned here rather than by the migration files.
// SQLite cannot drop a column constraint, so changing one means rebuilding the
// table: create a copy, move the rows, drop the original, rename. Dropping a
// table other tables reference is a foreign key violation while enforcement is
// on, so it is switched off for the duration and the result is checked with
// PRAGMA foreign_key_check before the transaction commits.
//
// Everything runs on one dedicated connection. PRAGMA foreign_keys is
// connection state and is silently ignored inside a transaction, so a pragma
// issued against the pool would be setting it on whichever connection happened
// to serve that call.
func applySQLiteMigration(sqlDB *sql.DB, m sqliteMigration) error {
	ctx := context.Background()

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("db: sqlite: migration %s: acquire connection: %w", m.name, err)
	}
	defer func() {
		// Restore the default before handing the connection back to the pool.
		// Leaving enforcement off would silently disable it for the rest of the
		// process, since the pool is capped at one connection.
		_, _ = conn.ExecContext(ctx, `PRAGMA foreign_keys=ON`)
		_ = conn.Close()
	}()

	body, pragmas := splitPragmas(m.body)
	for _, p := range pragmas {
		if _, err := conn.ExecContext(ctx, p); err != nil {
			return fmt.Errorf("db: sqlite: migration %s: %q: %w", m.name, p, err)
		}
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return fmt.Errorf("db: sqlite: migration %s: disable foreign_keys: %w", m.name, err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: sqlite: migration %s: begin: %w", m.name, err)
	}
	if _, err := tx.ExecContext(ctx, body); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("db: sqlite: migration %s: %w", m.name, err)
	}
	if err := checkForeignKeys(ctx, tx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("db: sqlite: migration %s: %w", m.name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, m.version); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("db: sqlite: migration %s: record version: %w", m.name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: sqlite: migration %s: commit: %w", m.name, err)
	}
	return nil
}

// checkForeignKeys reports whether the migration left any row pointing at a
// parent that is not there.
//
// This is the safety net for running with enforcement off: a rebuild that
// dropped rows the rest of the schema still references must not be recorded as
// applied. It runs inside the transaction so a violation rolls the whole thing
// back.
func checkForeignKeys(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign key check: %w", err)
	}
	defer rows.Close()

	var offenders []string
	for rows.Next() {
		// The pragma returns (table, rowid, parent, fkid). rowid is NULL for a
		// WITHOUT ROWID table, so it is scanned as a nullable value.
		var table, parent string
		var rowid sql.NullInt64
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return fmt.Errorf("foreign key check: scan: %w", err)
		}
		offenders = append(offenders, fmt.Sprintf("%s references missing %s", table, parent))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("foreign key check: %w", err)
	}
	if len(offenders) > 0 {
		return fmt.Errorf("left %d foreign key violations: %s",
			len(offenders), strings.Join(dedupe(offenders), "; "))
	}
	return nil
}

// dedupe collapses repeated offender descriptions, since one broken reference
// usually produces a row per orphaned record.
func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// splitPragmas separates PRAGMA statements from the rest of a migration body,
// so they run outside the transaction where SQLite honours them.
//
// foreign_keys pragmas are dropped rather than executed. A migration file that
// wrapped a table rebuild in OFF and ON had both hoisted and run back to back
// before the transaction started, which re-enabled enforcement immediately and
// made the rebuild fail on any database that actually had rows. Enforcement is
// the runner's business; see applySQLiteMigration.
func splitPragmas(body string) (rest string, pragmas []string) {
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), "PRAGMA ") {
			if !strings.Contains(strings.ToLower(trimmed), "foreign_keys") {
				pragmas = append(pragmas, trimmed)
			}
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n"), pragmas
}
