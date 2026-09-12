package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	sqlitemigrations "github.com/alipourhabibi/Hades/migration/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_time_format=sqlite")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	require.NoError(t, err)
	defer rows.Close()
	found := false
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		if name == column {
			found = true
		}
	}
	require.NoError(t, rows.Err())
	return found
}

func maxVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v sql.NullInt64
	require.NoError(t, db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&v))
	return int(v.Int64)
}

func TestMigrateSQLite_FreshDatabase(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, migrateSQLite(db))

	assert.True(t, columnExists(t, db, "modules", "lint_preset"))
	assert.True(t, columnExists(t, db, "modules", "breaking_enabled"))

	migrations, err := loadSQLiteMigrations()
	require.NoError(t, err)
	assert.Equal(t, migrations[len(migrations)-1].version, maxVersion(t, db))
}

func TestMigrateSQLite_IsIdempotent(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, migrateSQLite(db))
	before := maxVersion(t, db)

	// Startup runs this on every boot, so a second pass must be a no-op rather
	// than replaying statements that would now fail.
	require.NoError(t, migrateSQLite(db))
	require.NoError(t, migrateSQLite(db))

	assert.Equal(t, before, maxVersion(t, db))
}

func TestMigrateSQLite_LegacyDatabaseIsStampedNotReplayed(t *testing.T) {
	db := openTestDB(t)

	// Reproduce what the previous startup code produced: the baseline schema
	// plus the two ALTER statements, and no version table. Replaying migration
	// 002 against this would fail with "duplicate column name".
	migrations, err := loadSQLiteMigrations()
	require.NoError(t, err)
	body, pragmas := splitPragmas(migrations[0].body)
	for _, p := range pragmas {
		_, err := db.Exec(p)
		require.NoError(t, err)
	}
	_, err = db.Exec(body)
	require.NoError(t, err)
	_, err = db.Exec(`ALTER TABLE modules ADD COLUMN lint_preset INTEGER NOT NULL DEFAULT 1`)
	require.NoError(t, err)
	_, err = db.Exec(`ALTER TABLE modules ADD COLUMN breaking_enabled INTEGER NOT NULL DEFAULT 1`)
	require.NoError(t, err)

	require.NoError(t, migrateSQLite(db), "a legacy database must migrate without replaying what it already has")

	assert.Equal(t, migrations[len(migrations)-1].version, maxVersion(t, db))
	assert.True(t, columnExists(t, db, "modules", "lint_preset"))
}

func TestMigrateSQLite_AllowsManyOrgsWithEmptyEmail(t *testing.T) {
	// Organisations are user rows with an empty email. The original column-level
	// UNIQUE constraint meant the second organisation ever created failed, so
	// only one org could exist.
	db := openTestDB(t)
	require.NoError(t, migrateSQLite(db))

	_, err := db.Exec(`INSERT INTO users (username, email, type) VALUES ('org-one', '', 1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO users (username, email, type) VALUES ('org-two', '', 1)`)
	require.NoError(t, err, "a second organisation must be creatable")
	_, err = db.Exec(`INSERT INTO users (username, email, type) VALUES ('org-three', '', 1)`)
	require.NoError(t, err)
}

func TestMigrateSQLite_StillRejectsDuplicateRealEmails(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, migrateSQLite(db))

	_, err := db.Exec(`INSERT INTO users (username, email) VALUES ('alice', 'a@example.com')`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO users (username, email) VALUES ('bob', 'a@example.com')`)
	require.Error(t, err, "two accounts must not share a real email address")
}

func TestLoadSQLiteMigrations_OrderedAndUnique(t *testing.T) {
	migrations, err := loadSQLiteMigrations()
	require.NoError(t, err)
	require.NotEmpty(t, migrations)

	for i := 1; i < len(migrations); i++ {
		assert.Greater(t, migrations[i].version, migrations[i-1].version,
			"migrations must be strictly ascending: %s then %s", migrations[i-1].name, migrations[i].name)
	}
	assert.Equal(t, 1, migrations[0].version, "the baseline migration must be version 1")
}

func TestSplitPragmas(t *testing.T) {
	body := "PRAGMA journal_mode=WAL;\nCREATE TABLE t (a INT);\n"
	rest, pragmas := splitPragmas(body)

	assert.Equal(t, []string{"PRAGMA journal_mode=WAL;"}, pragmas,
		"pragmas run outside the transaction, where SQLite honours them")
	assert.Contains(t, rest, "CREATE TABLE t")
	assert.NotContains(t, rest, "PRAGMA")
}

// TestSplitPragmas_DropsForeignKeyPragmas pins the fix for migration 003.
//
// A file wrapping a table rebuild in OFF and ON had both hoisted and executed
// back to back before the transaction began, so enforcement was re-enabled
// immediately and the rebuild failed on any database with rows in it. Foreign
// key state is the runner's business now, so these are dropped rather than run.
func TestSplitPragmas_DropsForeignKeyPragmas(t *testing.T) {
	body := "PRAGMA foreign_keys=OFF;\nDROP TABLE t;\nPRAGMA foreign_keys=ON;\n"
	rest, pragmas := splitPragmas(body)

	assert.Empty(t, pragmas, "a migration must not be able to re-enable enforcement mid-rebuild")
	assert.Contains(t, rest, "DROP TABLE t")
	assert.NotContains(t, rest, "PRAGMA")
}

// TestMigrateSQLite_RebuildSurvivesReferencedRows is the regression test for
// migration 003 failing with "FOREIGN KEY constraint failed" on any database
// that had actual data in it.
//
// 003 rebuilds the users table, which means dropping it, and sessions, modules
// and the rest reference it. The migration wrapped itself in PRAGMA
// foreign_keys=OFF and =ON, but the runner hoisted every pragma out of the
// transaction and ran them in file order, so OFF was undone by ON before the
// rebuild started. Enforcement was therefore on for the DROP.
//
// It passed on a fresh database because there were no referencing rows to
// violate anything, which is exactly why this test seeds some first.
func TestMigrateSQLite_RebuildSurvivesReferencedRows(t *testing.T) {
	sqlDB := openTestDB(t)

	// A database at the pre-runner baseline: schema present, no version table.
	schema, err := sqlitemigrations.FS.ReadFile("001_schema.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.Exec(string(schema))
	require.NoError(t, err)
	// legacyBaselineVersion is 2, which means the pre-runner startup code had
	// already applied 002's ALTER statements. The fixture has to match, or the
	// stamped version claims columns that are not there.
	_, err = sqlDB.Exec(`
		ALTER TABLE modules ADD COLUMN lint_preset INTEGER NOT NULL DEFAULT 1;
		ALTER TABLE modules ADD COLUMN breaking_enabled INTEGER NOT NULL DEFAULT 1;`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`
		INSERT INTO users (id, username, email) VALUES
			('u-alice', 'alice', 'alice@example.com'),
			('u-acme',  'acme',  '');
		INSERT INTO sessions (user_id, expires_at) VALUES ('u-alice', datetime('now', '+1 day'));
		INSERT INTO modules (name, owner_id, visibility) VALUES ('alice/mymod', 'u-alice', 1);
	`)
	require.NoError(t, err)

	require.NoError(t, migrateSQLite(sqlDB), "the rebuild must survive rows that reference users")

	// Every row is still there, and still joined up.
	var users, sessions, modules int
	require.NoError(t, sqlDB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users))
	require.NoError(t, sqlDB.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions))
	require.NoError(t, sqlDB.QueryRow(`
		SELECT COUNT(*) FROM modules m JOIN users u ON u.id = m.owner_id`).Scan(&modules))
	assert.Equal(t, 2, users)
	assert.Equal(t, 1, sessions)
	assert.Equal(t, 1, modules, "the module still resolves to its owner after the rebuild")

	// No orphans left behind.
	rows, err := sqlDB.Query(`PRAGMA foreign_key_check`)
	require.NoError(t, err)
	defer rows.Close()
	assert.False(t, rows.Next(), "the rebuilt schema must have no foreign key violations")
	require.NoError(t, rows.Err())

	// Enforcement is back on for ordinary work, not left off by the migration.
	_, err = sqlDB.Exec(`INSERT INTO sessions (user_id, expires_at) VALUES ('nobody', datetime('now'))`)
	assert.Error(t, err, "foreign keys must be enforced again after migrating")
}
