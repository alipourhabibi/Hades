package db_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// Setup for the PostgreSQL half of these suites.
//
// SQLite gets a new temporary file per subtest, and NewFromConfig migrates it
// on the way up. PostgreSQL got neither: every subtest opened the same database
// and kept whatever the one before it wrote. On an empty database all 36 failed
// with `relation "users" does not exist`, and on a migrated one all 36 failed on
// duplicate keys. So the suite has never actually run against PostgreSQL.
//
// preparePostgres gives it the same start: the schema the migrations produce,
// with no rows in it.
//
// The migrations run once per `go test` invocation, not once per subtest. 36
// subtests times 36 migrations is about two minutes and buys nothing, because
// truncating is enough to undo what a subtest did.

// pgMigrationDir is relative to this package. The PostgreSQL migrations are not
// embedded the way the SQLite ones are, and go:embed cannot reach above the
// package directory.
const pgMigrationDir = "../../../../migration"

var (
	pgOnce sync.Once
	pgErr  error
)

// preparePostgres readies the database named by dsn and leaves it empty.
// Call it before opening a store for a subtest.
func preparePostgres(t *testing.T, dsn string) {
	t.Helper()

	pgOnce.Do(func() { pgErr = migratePostgres(dsn) })
	require.NoError(t, pgErr, "could not migrate the PostgreSQL test database")
	require.NoError(t, truncatePostgres(dsn), "could not empty the PostgreSQL test database")
}

// connect opens a pool that can run more than one statement per call. Migration
// 017 defines a function, so its file is not a single statement, and pgx uses
// the extended protocol by default, which allows only one.
func connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	return pgxpool.NewWithConfig(ctx, cfg)
}

// migratePostgres drops the schema and applies every *.up.sql in version order.
//
// It drops first so a second run starts where the first one did. Without that,
// running the suite twice fails on the tables the first run created, and
// "passes once, fails after" is the shape this whole file exists to remove.
func migratePostgres(dsn string) error {
	ctx := context.Background()

	pool, err := connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		return err
	}

	files, err := filepath.Glob(filepath.Join(pgMigrationDir, "*.up.sql"))
	if err != nil {
		return err
	}
	// The names are zero padded, so sorting them sorts by version.
	sort.Strings(files)

	for _, f := range files {
		sqlText, err := os.ReadFile(f) // #nosec G304 -- a path from our own migration directory.
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sqlText)); err != nil {
			return err
		}
	}
	return nil
}

// truncatePostgres empties every table, keeping the schema.
func truncatePostgres(dsn string) error {
	ctx := context.Background()

	pool, err := connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	rows, err := pool.Query(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public'`)
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, `"`+name+`"`)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(tables) == 0 {
		return nil
	}

	// One statement, because the tables reference each other.
	_, err = pool.Exec(ctx,
		`TRUNCATE TABLE `+strings.Join(tables, ", ")+` RESTART IDENTITY CASCADE`)
	return err
}
