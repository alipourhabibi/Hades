package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

// The visibility pre-filter is security-adjacent: if it admits a module the
// caller has no binding for, the OPA check behind it is the only thing left.
// These tests pin the predicate's behaviour against a real database.

func newVisibilityDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE users (
			id       TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL
		);
		CREATE TABLE modules (
			id                 TEXT PRIMARY KEY,
			create_time        DATETIME NOT NULL DEFAULT (datetime('now')),
			update_time        DATETIME NOT NULL DEFAULT (datetime('now')),
			name               TEXT NOT NULL,
			owner_id           TEXT NOT NULL,
			visibility         INTEGER NOT NULL,
			state              INTEGER NOT NULL DEFAULT 1,
			description        TEXT DEFAULT '',
			url                TEXT DEFAULT '',
			default_label_name TEXT DEFAULT '',
			default_branch     TEXT NOT NULL DEFAULT 'main',
			lint_preset        INTEGER NOT NULL DEFAULT 1,
			breaking_enabled   INTEGER NOT NULL DEFAULT 1
		);
		CREATE TABLE opa_role_bindings (
			id      TEXT PRIMARY KEY,
			subject TEXT NOT NULL,
			role    TEXT NOT NULL,
			domain  TEXT NOT NULL
		);
	`)
	require.NoError(t, err)
	return db
}

const (
	visPublic  = int(registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)
	visPrivate = int(registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)
)

func seedVisibilityFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO users (id, username) VALUES
			('u-alice', 'alice'), ('u-bob', 'bob'), ('u-org', 'acme');

		INSERT INTO modules (id, name, owner_id, visibility) VALUES
			('m-pub',        'alice/public',  'u-alice', ?),
			('m-alice-priv', 'alice/private', 'u-alice', ?),
			('m-bob-priv',   'bob/private',   'u-bob',   ?),
			('m-org-priv',   'acme/private',  'u-org',   ?);

		INSERT INTO opa_role_bindings (id, subject, role, domain) VALUES
			('b-alice', 'alice', 'owner', 'alice/*');
	`, visPublic, visPrivate, visPrivate, visPrivate)
	require.NoError(t, err)
}

func names(mods []*registryv1.Module) []string {
	out := make([]string, 0, len(mods))
	for _, m := range mods {
		out = append(out, m.Name)
	}
	return out
}

func TestListVisibleModules_AnonymousSeesOnlyPublic(t *testing.T) {
	db := newVisibilityDB(t)
	seedVisibilityFixture(t, db)
	store := NewModule(db, nil)

	got, err := store.ListVisibleModules(context.Background(), "", "", "", 50, 0)

	require.NoError(t, err)
	assert.Equal(t, []string{"alice/public"}, names(got))
}

func TestListVisibleModules_NamespaceBindingGrantsPrivate(t *testing.T) {
	db := newVisibilityDB(t)
	seedVisibilityFixture(t, db)
	store := NewModule(db, nil)

	got, err := store.ListVisibleModules(context.Background(), "", "alice", "u-alice", 50, 0)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alice/public", "alice/private"}, names(got))
	assert.NotContains(t, names(got), "bob/private", "a binding on alice/* must not reach bob's modules")
}

func TestListVisibleModules_UnrelatedUserSeesOnlyPublic(t *testing.T) {
	db := newVisibilityDB(t)
	seedVisibilityFixture(t, db)
	store := NewModule(db, nil)

	got, err := store.ListVisibleModules(context.Background(), "", "bob", "u-bob", 50, 0)

	require.NoError(t, err)
	// bob owns bob/private, so ownership alone admits it even with no binding.
	assert.ElementsMatch(t, []string{"alice/public", "bob/private"}, names(got))
	assert.NotContains(t, names(got), "alice/private")
}

func TestListVisibleModules_ExactDomainBinding(t *testing.T) {
	db := newVisibilityDB(t)
	seedVisibilityFixture(t, db)
	_, err := db.Exec(`INSERT INTO opa_role_bindings (id, subject, role, domain)
		VALUES ('b-exact', 'carol', 'reader', 'bob/private')`)
	require.NoError(t, err)
	store := NewModule(db, nil)

	got, err := store.ListVisibleModules(context.Background(), "", "carol", "u-carol", 50, 0)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alice/public", "bob/private"}, names(got))
}

func TestListVisibleModules_OrgMembershipBinding(t *testing.T) {
	db := newVisibilityDB(t)
	seedVisibilityFixture(t, db)
	_, err := db.Exec(`INSERT INTO opa_role_bindings (id, subject, role, domain)
		VALUES ('b-org', 'dave', 'contributor', 'acme/*')`)
	require.NoError(t, err)
	store := NewModule(db, nil)

	got, err := store.ListVisibleModules(context.Background(), "", "dave", "u-dave", 50, 0)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alice/public", "acme/private"}, names(got))
}

func TestListVisibleModules_FiltersByOwner(t *testing.T) {
	db := newVisibilityDB(t)
	seedVisibilityFixture(t, db)
	store := NewModule(db, nil)

	got, err := store.ListVisibleModules(context.Background(), "alice", "alice", "u-alice", 50, 0)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alice/public", "alice/private"}, names(got))
}

func TestListVisibleModules_EmptySubjectIDDoesNotMatchOwner(t *testing.T) {
	// An empty subject id must not accidentally match a row whose owner_id is
	// somehow empty; the predicate guards both parameters explicitly.
	db := newVisibilityDB(t)
	seedVisibilityFixture(t, db)
	_, err := db.Exec(`INSERT INTO modules (id, name, owner_id, visibility)
		VALUES ('m-orphan', 'ghost/private', '', ?)`, visPrivate)
	require.NoError(t, err)
	store := NewModule(db, nil)

	got, err := store.ListVisibleModules(context.Background(), "", "", "", 50, 0)

	require.NoError(t, err)
	assert.NotContains(t, names(got), "ghost/private")
}

func TestListVisibleModules_Pagination(t *testing.T) {
	db := newVisibilityDB(t)
	seedVisibilityFixture(t, db)
	store := NewModule(db, nil)

	first, err := store.ListVisibleModules(context.Background(), "", "alice", "u-alice", 1, 0)
	require.NoError(t, err)
	require.Len(t, first, 1)

	second, err := store.ListVisibleModules(context.Background(), "", "alice", "u-alice", 1, 1)
	require.NoError(t, err)
	require.Len(t, second, 1)

	assert.NotEqual(t, first[0].Name, second[0].Name)
}
