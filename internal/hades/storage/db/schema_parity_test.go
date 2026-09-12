package db_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
)

// Schema parity.
//
// The SQLite schema was one file against 31 PostgreSQL migrations and had
// drifted: tables with no indexes, missing UNIQUE constraints on credential
// hashes, no ON DELETE actions, and two columns whose defaults differed. Each
// of those was a silent behaviour difference between the default backend and
// the production one.
//
// This test diffs the two live schemas and fails on divergence. It runs only
// when HADES_TEST_POSTGRES_DSN names a database, because there is nothing to
// compare against otherwise.
//
// Divergence that is deliberate goes in the waiver maps below, with the reason.
// An empty waiver is the goal; a waiver with a reason is a decision; an
// undocumented difference is a bug.

// tablesOnlyInPostgres lists tables that legitimately do not exist on SQLite.
var tablesOnlyInPostgres = map[string]string{
	"schema_migrations": "golang-migrate's own bookkeeping table; SQLite has its own runner with a table of the same name but a different shape",
}

// tablesOnlyInSQLite lists tables that legitimately do not exist on PostgreSQL.
var tablesOnlyInSQLite = map[string]string{
	"schema_migrations": "the SQLite migration runner's bookkeeping table",
	"sqlite_sequence":   "SQLite's internal AUTOINCREMENT bookkeeping",
}

// columnsWaived lists table.column differences that are accepted, with why.
var columnsWaived = map[string]string{}

func TestSchemaParity(t *testing.T) {
	dsn := os.Getenv("HADES_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HADES_TEST_POSTGRES_DSN is unset; there is nothing to compare the SQLite schema against")
	}
	ctx := context.Background()

	// Both schemas are built by the production migration paths, not by reading
	// the migration files: what matters is the schema a deployment ends up
	// with, including anything a later migration altered.
	sqliteCfg := config.Config{SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "hades.db")}}
	_, err := db.NewFromConfig(sqliteCfg, testLogger(t))
	require.NoError(t, err)

	sqlDB, err := sql.Open("sqlite", sqliteCfg.SQLite.Path)
	require.NoError(t, err)
	defer sqlDB.Close()

	pgCfg := config.Config{
		Backends: config.BackendsConfig{Database: config.DatabasePostgres},
		DB:       config.DB{ConnectionString: dsn},
	}
	_, err = db.NewFromConfig(pgCfg, testLogger(t))
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	sqliteTables := sqliteSchema(t, sqlDB)
	pgTables := postgresSchema(t, ctx, pool)

	t.Run("tables", func(t *testing.T) {
		for name := range pgTables {
			if _, ok := sqliteTables[name]; ok {
				continue
			}
			if reason, waived := tablesOnlyInPostgres[name]; waived {
				t.Logf("waived: %s exists only on PostgreSQL (%s)", name, reason)
				continue
			}
			t.Errorf("table %q exists on PostgreSQL and not on SQLite", name)
		}
		for name := range sqliteTables {
			if _, ok := pgTables[name]; ok {
				continue
			}
			if reason, waived := tablesOnlyInSQLite[name]; waived {
				t.Logf("waived: %s exists only on SQLite (%s)", name, reason)
				continue
			}
			t.Errorf("table %q exists on SQLite and not on PostgreSQL", name)
		}
	})

	t.Run("columns", func(t *testing.T) {
		for name, pgCols := range pgTables {
			sqliteCols, ok := sqliteTables[name]
			if !ok {
				continue // reported by the tables subtest
			}
			missing := difference(pgCols, sqliteCols)
			extra := difference(sqliteCols, pgCols)
			for _, c := range missing {
				if reason, waived := columnsWaived[name+"."+c]; waived {
					t.Logf("waived: %s.%s missing on SQLite (%s)", name, c, reason)
					continue
				}
				assert.Failf(t, "column missing on SQLite",
					"%s.%s exists on PostgreSQL and not on SQLite", name, c)
			}
			for _, c := range extra {
				if reason, waived := columnsWaived[name+"."+c]; waived {
					t.Logf("waived: %s.%s missing on PostgreSQL (%s)", name, c, reason)
					continue
				}
				assert.Failf(t, "column missing on PostgreSQL",
					"%s.%s exists on SQLite and not on PostgreSQL", name, c)
			}
		}
	})
}

// sqliteSchema returns table name -> sorted column names.
func sqliteSchema(t *testing.T, sqlDB *sql.DB) map[string][]string {
	t.Helper()
	rows, err := sqlDB.Query(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	require.NoError(t, err)
	var names []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		names = append(names, n)
	}
	require.NoError(t, rows.Err())
	rows.Close()

	out := map[string][]string{}
	for _, table := range names {
		colRows, err := sqlDB.Query(`SELECT name FROM pragma_table_info(?)`, table)
		require.NoError(t, err)
		var cols []string
		for colRows.Next() {
			var c string
			require.NoError(t, colRows.Scan(&c))
			cols = append(cols, strings.ToLower(c))
		}
		require.NoError(t, colRows.Err())
		colRows.Close()
		sort.Strings(cols)
		out[table] = cols
	}
	return out
}

// postgresSchema returns table name -> sorted column names.
func postgresSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string][]string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'`)
	require.NoError(t, err)
	defer rows.Close()

	out := map[string][]string{}
	for rows.Next() {
		var table, column string
		require.NoError(t, rows.Scan(&table, &column))
		out[table] = append(out[table], strings.ToLower(column))
	}
	require.NoError(t, rows.Err())
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

// difference returns the elements of a that are not in b.
func difference(a, b []string) []string {
	inB := make(map[string]struct{}, len(b))
	for _, s := range b {
		inB[s] = struct{}{}
	}
	var out []string
	for _, s := range a {
		if _, ok := inB[s]; !ok {
			out = append(out, s)
		}
	}
	return out
}
