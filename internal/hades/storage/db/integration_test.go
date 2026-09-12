package db_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authv1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/google/uuid"
)

// Integration tests for the storage layer. They open a real database through
// the production constructor, run the real migrations, and exercise the domain
// storages against the resulting schema.
//
// Nothing here is mocked, which is the point. A fake returning canned rows
// cannot catch a JOIN against a renamed column, an UPDATE missing its owner
// predicate, a constraint the code violates, or a place where the two backends
// disagree. Every storage bug found in this codebase so far was of that kind.
//
// SQLite runs by default because it needs nothing installed. PostgreSQL runs
// the same suite when HADES_TEST_POSTGRES_DSN names a database, and is skipped
// otherwise rather than failing, so `go test ./...` stays green on a laptop
// with no server running.

// backend names a database the suite can run against.
type backend struct {
	name string
	open func(t *testing.T) db.Store
}

func backends(t *testing.T) []backend {
	t.Helper()

	out := []backend{{
		name: "sqlite",
		open: func(t *testing.T) db.Store {
			// A file rather than :memory:, because the shared in-memory DSN is
			// process-global: two tests would otherwise share one database.
			cfg := config.Config{SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "hades.db")}}
			store, err := db.NewFromConfig(cfg, testLogger(t))
			require.NoError(t, err)
			return store
		},
	}}

	if dsn := os.Getenv("HADES_TEST_POSTGRES_DSN"); dsn != "" {
		out = append(out, backend{
			name: "postgres",
			open: func(t *testing.T) db.Store {
				cfg := config.Config{
					Backends: config.BackendsConfig{Database: config.DatabasePostgres},
					DB:       config.DB{ConnectionString: dsn},
				}
				store, err := db.NewFromConfig(cfg, testLogger(t))
				require.NoError(t, err)
				return store
			},
		})
	}
	return out
}

func testLogger(t *testing.T) *log.LoggerWrapper {
	t.Helper()
	logger, err := log.NewWithConfig(config.Logger{Level: "error", Output: "stdout"})
	require.NoError(t, err)
	return logger
}

// eachBackend runs fn against every available backend as a subtest.
func eachBackend(t *testing.T, fn func(t *testing.T, store db.Store)) {
	t.Helper()
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) {
			fn(t, b.open(t))
		})
	}
}

func TestUserLifecycle(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		users := store.User()

		require.NoError(t, users.Create(ctx, "alice", "alice@example.com", "hash",
			identityv1.UserType_USER_TYPE_USER, identityv1.UserState_USER_STATE_ACTIVE, "desc", "https://example.com"))

		byName, err := users.GetByUsername(ctx, "alice")
		require.NoError(t, err)
		assert.Equal(t, "alice@example.com", byName.Email)
		assert.Empty(t, byName.Password, "read paths must not carry the password hash")

		byID, err := users.GetByID(ctx, byName.Id)
		require.NoError(t, err)
		assert.Equal(t, byName.Id, byID.Id)

		byEmail, err := users.GetByEmail(ctx, "alice@example.com")
		require.NoError(t, err)
		assert.Equal(t, byName.Id, byEmail.Id)

		auth, err := users.GetAuthFieldsByUsername(ctx, "alice")
		require.NoError(t, err)
		assert.Equal(t, "hash", auth.PasswordHash, "the hash is still reachable where passwords are checked")
	})
}

// TestSecondOrganisationCanBeCreated is the regression test for the partial
// unique index added in 8.7.5. Organisations are user rows with an empty email
// and users.email carried a plain UNIQUE constraint, so the second organisation
// ever created violated it: only one could exist per deployment.
func TestSecondOrganisationCanBeCreated(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		require.NoError(t, store.User().Create(ctx, "alice", "alice@example.com", "hash",
			identityv1.UserType_USER_TYPE_USER, identityv1.UserState_USER_STATE_ACTIVE, "", ""))
		alice, err := store.User().GetByUsername(ctx, "alice")
		require.NoError(t, err)

		first, err := store.Org().Create(ctx, "acme", "", "", alice.Id)
		require.NoError(t, err)
		second, err := store.Org().Create(ctx, "globex", "", "", alice.Id)
		require.NoError(t, err, "a second organisation must not collide on the empty email")

		assert.NotEqual(t, first.Id, second.Id)
		assert.Equal(t, identityv1.UserType_USER_TYPE_ORGANIZATION, second.Type)
	})
}

// TestTwoAccountsCannotShareAnEmail is the other half of that index: dropping
// the constraint entirely would have let two accounts register the same
// address, which registration and the OAuth email link both rely on.
func TestTwoAccountsCannotShareAnEmail(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		require.NoError(t, store.User().Create(ctx, "alice", "shared@example.com", "hash",
			identityv1.UserType_USER_TYPE_USER, identityv1.UserState_USER_STATE_ACTIVE, "", ""))

		err := store.User().Create(ctx, "bob", "shared@example.com", "hash",
			identityv1.UserType_USER_TYPE_USER, identityv1.UserState_USER_STATE_ACTIVE, "", "")

		assert.Error(t, err, "a real address must stay unique across accounts")
	})
}

func TestOrgMembershipAndModuleVisibility(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		bob := mustUser(t, store, "bob", "bob@example.com")

		org, err := store.Org().Create(ctx, "acme", "", "", alice.Id)
		require.NoError(t, err)

		// Create writes the creator's admin membership as part of the same call.
		role, err := store.Org().GetMemberRole(ctx, org.Id, alice.Id)
		require.NoError(t, err)
		assert.Equal(t, "admin", role)

		require.NoError(t, store.Org().AddMember(ctx, org.Id, bob.Id, "member"))
		members, err := store.Org().ListMembers(ctx, org.Id)
		require.NoError(t, err)
		assert.Len(t, members, 2)

		// AddMember is an upsert: adding an existing member changes their role.
		require.NoError(t, store.Org().AddMember(ctx, org.Id, bob.Id, "admin"))
		role, err = store.Org().GetMemberRole(ctx, org.Id, bob.Id)
		require.NoError(t, err)
		assert.Equal(t, "admin", role)

		count, err := store.Org().CountMembers(ctx, org.Id)
		require.NoError(t, err)
		assert.Equal(t, int32(2), count, "the upsert must not have inserted a duplicate row")

		require.NoError(t, store.Org().RemoveMember(ctx, org.Id, bob.Id))
		count, err = store.Org().CountMembers(ctx, org.Id)
		require.NoError(t, err)
		assert.Equal(t, int32(1), count)
	})
}

func TestModuleVisibilityFilterAcrossBackends(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		bob := mustUser(t, store, "bob", "bob@example.com")

		mustModule(t, store, "alice/public", alice.Id, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)
		mustModule(t, store, "alice/private", alice.Id, registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)
		mustModule(t, store, "bob/private", bob.Id, registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)

		anon, err := store.Module().ListVisibleModules(ctx, "", "", "", 50, 0)
		require.NoError(t, err)
		assert.Equal(t, []string{"alice/public"}, moduleNames(anon),
			"an anonymous caller sees public modules only")

		owned, err := store.Module().ListVisibleModules(ctx, "", "alice", alice.Id, 50, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"alice/public", "alice/private"}, moduleNames(owned))
		assert.NotContains(t, moduleNames(owned), "bob/private")
	})
}

func TestCommitJoinsOwnerAndModule(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		module := mustModule(t, store, "alice/mymod", alice.Id, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)

		id := uuid.New()
		hash := "abcdef1234567890abcdef1234567890abcdef12"
		require.NoError(t, store.Commit().Create(ctx, id, hash, alice.Id, module.Id,
			registryv1.DigestType_DIGEST_TYPE_B5, "digestvalue", alice.Id, ""))

		byHash, err := store.Commit().GetByHash(ctx, hash)
		require.NoError(t, err)
		assert.Equal(t, "alice", byHash.Owner.Username, "the owner join must populate the username")
		assert.Equal(t, "alice/mymod", byHash.Module.Name)
		assert.Equal(t, alice.Id, byHash.CreatedByUserId)

		byPrefix, err := store.Commit().GetByHashPrefix(ctx, hash[:8])
		require.NoError(t, err)
		assert.Equal(t, byHash.Id, byPrefix.Id)

		byDigest, err := store.Commit().GetCommitByDigest(ctx, module.Id, "digestvalue")
		require.NoError(t, err)
		require.NotNil(t, byDigest, "dedup lookup must find the commit it just wrote")
		assert.Equal(t, byHash.Id, byDigest.Id)

		missing, err := store.Commit().GetCommitByDigest(ctx, module.Id, "nosuchdigest")
		require.NoError(t, err, "an absent digest is not an error, it is a cache miss")
		assert.Nil(t, missing)
	})
}

func TestCommitListPagination(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		module := mustModule(t, store, "alice/mymod", alice.Id, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)

		for i := 0; i < 3; i++ {
			require.NoError(t, store.Commit().Create(ctx, uuid.New(),
				commitHash(i), alice.Id, module.Id,
				registryv1.DigestType_DIGEST_TYPE_B5, commitHash(i), alice.Id, ""))
			// create_time has one-second resolution on SQLite; without a gap the
			// ordering between rows is not observable.
			time.Sleep(5 * time.Millisecond)
		}

		first, err := store.Commit().ListByModule(ctx, module.Id, 2, 0)
		require.NoError(t, err)
		require.Len(t, first, 2)

		second, err := store.Commit().ListByModule(ctx, module.Id, 2, 2)
		require.NoError(t, err)
		require.Len(t, second, 1)

		seen := map[string]bool{}
		for _, c := range append(first, second...) {
			assert.False(t, seen[c.Id], "commit %s appeared on two pages", c.Id)
			seen[c.Id] = true
		}
	})
}

func TestAPITokenLifecycle(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		tokens := store.APIToken()

		expires := time.Now().Add(24 * time.Hour)
		row, err := tokens.Create(ctx, alice.Id, "ci", "hades1_abcd1234", "tokenhash",
			[]string{"module:read", "module:push:alice/mymod"}, &expires)
		require.NoError(t, err)
		assert.Equal(t, []string{"module:read", "module:push:alice/mymod"}, row.Scopes,
			"scopes must survive the round trip intact, including the domain part")

		byHash, err := tokens.GetByTokenHash(ctx, "tokenhash")
		require.NoError(t, err)
		assert.Equal(t, row.ID, byHash.ID)
		assert.Nil(t, byHash.RevokedAt)

		listed, err := tokens.ListByUserID(ctx, alice.Id, 50, 0)
		require.NoError(t, err)
		require.Len(t, listed, 1)

		require.NoError(t, tokens.RevokeByOwner(ctx, row.ID, alice.Id))

		listed, err = tokens.ListByUserID(ctx, alice.Id, 50, 0)
		require.NoError(t, err)
		require.Len(t, listed, 1, "a revoked token stays in the listing so its status can be reported")
		assert.NotNil(t, listed[0].RevokedAt)

		byHash, err = tokens.GetByTokenHash(ctx, "tokenhash")
		require.NoError(t, err, "the row survives revocation so the interceptor can reject it by name")
		assert.NotNil(t, byHash.RevokedAt)
	})
}

// TestAPITokenRevokeIsScopedToTheOwner is the ownership boundary: a caller must
// not be able to revoke a token by id alone.
func TestAPITokenRevokeIsScopedToTheOwner(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		bob := mustUser(t, store, "bob", "bob@example.com")

		row, err := store.APIToken().Create(ctx, alice.Id, "ci", "hades1_abcd1234", "tokenhash",
			[]string{"module:read"}, nil)
		require.NoError(t, err)

		err = store.APIToken().RevokeByOwner(ctx, row.ID, bob.Id)
		assert.Error(t, err, "bob must not be able to revoke alice's token")

		byHash, err := store.APIToken().GetByTokenHash(ctx, "tokenhash")
		require.NoError(t, err)
		assert.Nil(t, byHash.RevokedAt, "the token is still live")
	})
}

func TestSessionLifecycle(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		sessions := store.Session()

		idle := time.Now().Add(time.Hour)
		absolute := time.Now().Add(24 * time.Hour)
		keepID, err := sessions.CreateWithToken(ctx, alice.Id, "session", "hash-keep", "10.0.0.1", "curl", idle, absolute)
		require.NoError(t, err)
		_, err = sessions.CreateWithToken(ctx, alice.Id, "session", "hash-drop", "10.0.0.2", "curl", idle, absolute)
		require.NoError(t, err)

		listed, err := sessions.ListByUserID(ctx, alice.Id)
		require.NoError(t, err)
		assert.Len(t, listed, 2)

		require.NoError(t, sessions.RevokeAllForUser(ctx, alice.Id, keepID))

		listed, err = sessions.ListByUserID(ctx, alice.Id)
		require.NoError(t, err)
		require.Len(t, listed, 1, "revoke-all-others must spare the named session")
		assert.Equal(t, keepID, listed[0].ID)
	})
}

// TestExpiredSessionIsNotListed covers the expiry predicate rather than the
// revoked one: a session that simply timed out must also disappear.
func TestExpiredSessionIsNotListed(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")

		past := time.Now().Add(-time.Hour)
		_, err := store.Session().CreateWithToken(ctx, alice.Id, "session", "hash-old", "10.0.0.1", "curl", past, past)
		require.NoError(t, err)

		listed, err := store.Session().ListByUserID(ctx, alice.Id)
		require.NoError(t, err)
		assert.Empty(t, listed)
	})
}

func TestAuditLogIsScopedToOneUser(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		bob := mustUser(t, store, "bob", "bob@example.com")

		require.NoError(t, store.AuditLog().Create(ctx, &alice.Id,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_SUCCESS, "10.0.0.1", "curl", map[string]any{"totp": true}))
		require.NoError(t, store.AuditLog().Create(ctx, &bob.Id,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_FAILED, "10.0.0.2", "curl", nil))

		rows, err := store.AuditLog().List(ctx, alice.Id, 50, 0)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, authv1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_SUCCESS, rows[0].EventType)
		assert.Equal(t, "10.0.0.1", rows[0].IPAddress)
		assert.Equal(t, true, rows[0].Metadata["totp"], "metadata must survive the JSON round trip")
	})
}

// TestUnitOfWorkRollsBackEveryWrite covers the transaction boundary itself: a
// failure part way through must leave nothing behind, on either backend.
func TestUnitOfWorkRollsBackEveryWrite(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")

		_, err := store.Do(ctx, func(txCtx context.Context) (interface{}, error) {
			if _, err := store.Module().Create(txCtx, "alice/doomed", alice.Id,
				registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC,
				registryv1.ModuleState_MODULE_STATE_ACTIVE, "", "", "", "main",
				registryv1.LintPreset_LINT_PRESET_DEFAULT, false); err != nil {
				return nil, err
			}
			return nil, assertErr("the caller changed its mind")
		}, 10*time.Second)
		require.Error(t, err)

		_, err = store.Module().GetModuleByOwnerAndName(ctx, "alice", "doomed")
		assert.Error(t, err, "the module written inside the failed transaction must not survive")
	})
}

func TestUnitOfWorkCommitsEveryWrite(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")

		_, err := store.Do(ctx, func(txCtx context.Context) (interface{}, error) {
			module, err := store.Module().Create(txCtx, "alice/kept", alice.Id,
				registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC,
				registryv1.ModuleState_MODULE_STATE_ACTIVE, "", "", "", "main",
				registryv1.LintPreset_LINT_PRESET_DEFAULT, false)
			if err != nil {
				return nil, err
			}
			return nil, store.Commit().Create(txCtx, uuid.New(), commitHash(9), alice.Id, module.Id,
				registryv1.DigestType_DIGEST_TYPE_B5, "digest", alice.Id, "")
		}, 10*time.Second)
		require.NoError(t, err)

		module, err := store.Module().GetModuleByOwnerAndName(ctx, "alice", "kept")
		require.NoError(t, err)
		commits, err := store.Commit().ListByModule(ctx, module.Id, 50, 0)
		require.NoError(t, err)
		assert.Len(t, commits, 1, "both writes in the unit of work must be visible after commit")
	})
}

// --- helpers ---------------------------------------------------------------

func mustUser(t *testing.T, store db.Store, username, email string) *identityv1.User {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, store.User().Create(ctx, username, email, "hash",
		identityv1.UserType_USER_TYPE_USER, identityv1.UserState_USER_STATE_ACTIVE, "", ""))
	user, err := store.User().GetByUsername(ctx, username)
	require.NoError(t, err)
	return user
}

func mustModule(t *testing.T, store db.Store, name, ownerID string, visibility registryv1.ModuleVisibility) *registryv1.Module {
	t.Helper()
	module, err := store.Module().Create(context.Background(), name, ownerID, visibility,
		registryv1.ModuleState_MODULE_STATE_ACTIVE, "", "", "", "main",
		registryv1.LintPreset_LINT_PRESET_DEFAULT, false)
	require.NoError(t, err)
	return module
}

func moduleNames(modules []*registryv1.Module) []string {
	out := make([]string, 0, len(modules))
	for _, m := range modules {
		out = append(out, m.Name)
	}
	return out
}

// commitHash returns a distinct 40-character hex string per index.
func commitHash(i int) string {
	const base = "abcdef1234567890abcdef1234567890abcdef"
	return base + string(rune('0'+i)) + "0"
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

// TestStaleJobRecoveryLeavesFreshJobsAlone is the regression test for the
// SQLite datetime comparison in RecoverStaleJobs.
//
// started_at is written as "2026-08-07 13:55:49" and the staleness threshold
// formatted as RFC3339 with a 'T'. SQLite compared them as strings and ' '
// sorts below 'T', so every running job matched however recently it had
// started. The recovery pass ran once a minute and reset live jobs to pending,
// so any job outliving one tick was claimed again and generated twice in
// parallel.
func TestStaleJobRecoveryLeavesFreshJobsAlone(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		module := mustModule(t, store, "alice/mymod", alice.Id, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)

		commitID := uuid.New()
		require.NoError(t, store.Commit().Create(ctx, commitID, commitHash(1), alice.Id, module.Id,
			registryv1.DigestType_DIGEST_TYPE_B5, "digest", alice.Id, ""))
		require.NoError(t, store.SDKJob().CreateBatch(ctx, commitID.String(), module.Id,
			[]config.GeneratorConfig{{Language: "go", Plugin: "buf.build/protocolbuffers/go"}}))

		claimed, err := store.SDKJob().ClaimPending(ctx, 10)
		require.NoError(t, err)
		require.Len(t, claimed, 1)
		assert.Equal(t, "running", claimed[0].Status)

		recovered, err := store.SDKJob().RecoverStaleJobs(ctx, 5*time.Minute)
		require.NoError(t, err)
		assert.Equal(t, int64(0), recovered, "a job that started moments ago is not stale")

		jobs, err := store.SDKJob().ListByModule(ctx, module.Id)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, "running", jobs[0].Status, "a live job must not be handed to a second worker")

		// A negative timeout puts the threshold in the future, so the job is
		// past it. This is what proves the comparison matches when it should,
		// rather than never matching. A zero timeout would not do: started_at
		// has one-second resolution, so a job claimed in this same second is
		// not strictly older than "now".
		recovered, err = store.SDKJob().RecoverStaleJobs(ctx, -time.Minute)
		require.NoError(t, err)
		assert.Equal(t, int64(1), recovered)
	})
}

func TestSDKJobTerminalStates(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		module := mustModule(t, store, "alice/mymod", alice.Id, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)

		commitID := uuid.New()
		require.NoError(t, store.Commit().Create(ctx, commitID, commitHash(2), alice.Id, module.Id,
			registryv1.DigestType_DIGEST_TYPE_B5, "digest", alice.Id, ""))
		require.NoError(t, store.SDKJob().CreateBatch(ctx, commitID.String(), module.Id,
			[]config.GeneratorConfig{
				{Language: "go", Plugin: "buf.build/protocolbuffers/go"},
				{Language: "python", Plugin: "buf.build/protocolbuffers/python"},
			}))

		claimed, err := store.SDKJob().ClaimPending(ctx, 10)
		require.NoError(t, err)
		require.Len(t, claimed, 2)

		require.NoError(t, store.SDKJob().MarkSucceeded(ctx, claimed[0].ID, "s3://bucket/key"))
		// attempts below the maximum means retryable, so "failed" not "dead".
		require.NoError(t, store.SDKJob().MarkFailed(ctx, claimed[1].ID, "protoc exploded", 1))

		jobs, err := store.SDKJob().ListByModule(ctx, module.Id)
		require.NoError(t, err)
		byID := map[string]string{}
		for _, j := range jobs {
			byID[j.ID] = j.Status
			assert.NotNil(t, j.FinishedAt, "a terminal job records when it finished")
		}
		assert.Equal(t, "succeeded", byID[claimed[0].ID])
		assert.Equal(t, "failed", byID[claimed[1].ID])

		succeeded, err := store.SDKJob().GetByCommitAndLang(ctx, commitID.String(), claimed[0].Language)
		require.NoError(t, err)
		require.NotNil(t, succeeded)
		assert.Equal(t, "s3://bucket/key", succeeded.OutputLocation)
	})
}

// TestMarkFailedAtTheRetryLimitIsDead pins the one status transition a caller
// cannot ask for directly.
func TestMarkFailedAtTheRetryLimitIsDead(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		module := mustModule(t, store, "alice/mymod", alice.Id, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)

		commitID := uuid.New()
		require.NoError(t, store.Commit().Create(ctx, commitID, commitHash(3), alice.Id, module.Id,
			registryv1.DigestType_DIGEST_TYPE_B5, "digest", alice.Id, ""))
		require.NoError(t, store.SDKJob().CreateBatch(ctx, commitID.String(), module.Id,
			[]config.GeneratorConfig{{Language: "go", Plugin: "buf.build/protocolbuffers/go"}}))

		claimed, err := store.SDKJob().ClaimPending(ctx, 1)
		require.NoError(t, err)
		require.NoError(t, store.SDKJob().MarkFailed(ctx, claimed[0].ID, "gave up", sdkjob.MaxAttempts))

		jobs, err := store.SDKJob().ListByModule(ctx, module.Id)
		require.NoError(t, err)
		assert.Equal(t, "dead", jobs[0].Status)
	})
}
