package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authv1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	commitpkg "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/devicegrant"
)

// Backend-parity tests.
//
// Every one of these covers a defect that existed on exactly one backend and
// was therefore invisible: the two implementations were tested separately,
// mostly not at all, and never against each other. They run under eachBackend,
// so SQLite runs everywhere and PostgreSQL runs when HADES_TEST_POSTGRES_DSN
// names a database.
//
// The rule this file encodes: a storage method must behave the same way on
// both backends, including which error it returns and how many rows it
// touched. Where they cannot agree, that is a finding, not a test to relax.

// TestSessionRevocationActuallyRevokes is the regression test for the worst
// bug in the review: Revoke reported success while updating zero rows, because
// SQLite compared a hyphenated id against a dashless column. The ownership
// check passed, the update matched nothing, Exec returned no error, and a user
// who revoked a session they believed compromised was told it worked.
func TestSessionRevocationActuallyRevokes(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		sessions := store.Session()

		id, err := sessions.CreateWithToken(ctx, alice.Id, "session", "hash-1", "10.0.0.1", "curl",
			time.Now().Add(time.Hour), time.Now().Add(24*time.Hour))
		require.NoError(t, err)

		require.NoError(t, sessions.Revoke(ctx, id))

		got, err := sessions.GetByID(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, got.RevokedAt, "Revoke must actually revoke, not report success and change nothing")

		live, err := sessions.ListByUserID(ctx, alice.Id)
		require.NoError(t, err)
		assert.Empty(t, live, "a revoked session must not still be listed")
	})
}

// TestSessionTouchAndTOTPFlagPersist covers the same class as Revoke: two more
// methods that took an id and quietly matched nothing.
func TestSessionTouchAndTOTPFlagPersist(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		sessions := store.Session()

		idle := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
		id, err := sessions.CreateWithToken(ctx, alice.Id, "session", "hash-2", "", "",
			idle, time.Now().Add(24*time.Hour))
		require.NoError(t, err)

		newIdle := idle.Add(2 * time.Hour)
		require.NoError(t, sessions.Touch(ctx, id, newIdle))
		require.NoError(t, sessions.MarkTOTPVerified(ctx, id))

		got, err := sessions.GetByID(ctx, id)
		require.NoError(t, err)
		assert.True(t, got.TOTPVerified, "MarkTOTPVerified must persist")
		assert.WithinDuration(t, newIdle, got.IdleExpiresAt, 2*time.Second,
			"Touch must slide the idle expiry")
	})
}

// TestSessionIDsRoundTripThroughTheAPI pins the shape of an id as the API
// hands it out. A caller must be able to take an id from one call and pass it
// to another, on either backend, and the text must not change between the two.
func TestSessionIDsRoundTripThroughTheAPI(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")

		_, err := uuid.Parse(alice.Id)
		require.NoError(t, err, "a user id handed to a caller must parse as a UUID")

		module := mustModule(t, store, "alice/mymod", alice.Id, moduleVisibilityPublic)
		_, err = uuid.Parse(module.Id)
		require.NoError(t, err, "a module id handed to a caller must parse as a UUID")

		fetched, err := store.Module().GetModulesByRefs(ctx, moduleRefByID(module.Id))
		require.NoError(t, err)
		require.Len(t, fetched, 1)
		assert.Equal(t, module.Id, fetched[0].Id,
			"the id a caller received must be the id it gets back")
	})
}

// TestAuditLogAcceptsEveryEventType is the regression test for every audit
// write failing on PostgreSQL: the SQL enum used lower_snake_case values while
// the code inserted the generated protobuf name, so no value ever matched. The
// column is free TEXT on SQLite, which is why the suite stayed green.
func TestAuditLogAcceptsEveryEventType(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		audit := store.AuditLog()

		events := []authv1.AuditEventType{
			authv1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_SUCCESS,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_FAILED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_ACCOUNT_LOCKED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_EMAIL_VERIFIED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_LOGOUT,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_PASSWORD_RESET,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_PASSWORD_CHANGED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_API_TOKEN_CREATED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_API_TOKEN_REVOKED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_MODULE_CREATED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_MODULE_UPDATED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_SESSION_REVOKED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_TOTP_ENABLED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_TOTP_DISABLED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_OAUTH_LINKED,
			authv1.AuditEventType_AUDIT_EVENT_TYPE_OAUTH_UNLINKED,
		}
		for _, e := range events {
			require.NoErrorf(t, audit.Create(ctx, &alice.Id, e, "10.0.0.1", "curl", nil),
				"writing %s must succeed on every backend", e)
		}

		rows, err := audit.List(ctx, alice.Id, 100, 0)
		require.NoError(t, err)
		require.Len(t, rows, len(events), "every event written must be readable back")

		seen := map[authv1.AuditEventType]bool{}
		for _, r := range rows {
			seen[r.EventType] = true
		}
		for _, e := range events {
			assert.Truef(t, seen[e], "%s must round-trip as itself, not as UNSPECIFIED", e)
		}
	})
}

// TestAuditLogPageSizeIsClamped covers the one paginated method that had no
// upper bound.
func TestAuditLogPageSizeIsClamped(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		for i := 0; i < 5; i++ {
			require.NoError(t, store.AuditLog().Create(ctx, &alice.Id,
				authv1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_SUCCESS, "", "", nil))
		}
		rows, err := store.AuditLog().List(ctx, alice.Id, 1_000_000, 0)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(rows), 100)
	})
}

// TestAPITokenLookupAndScopes covers the token hash lookup that runs on every
// authenticated request, and the scope encoding that differed between a
// PostgreSQL TEXT[] and a SQLite comma-joined string.
func TestAPITokenLookupAndScopes(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		tokens := store.APIToken()

		// A scope with a comma in it. The comma-joined SQLite encoding could
		// not represent this, so it round-tripped as two scopes there and as
		// one on PostgreSQL.
		scopes := []string{"module:read:alice/mymod", "module:push", "weird:scope,with-comma"}
		created, err := tokens.Create(ctx, alice.Id, "ci", "hades1_abc", "tokenhash-1", scopes, nil)
		require.NoError(t, err)
		assert.Equal(t, scopes, created.Scopes, "scopes must round-trip exactly")

		byHash, err := tokens.GetByTokenHash(ctx, "tokenhash-1")
		require.NoError(t, err)
		assert.Equal(t, created.ID, byHash.ID)
		assert.Equal(t, scopes, byHash.Scopes)
		assert.Equal(t, alice.Id, byHash.UserID, "the owner id must come back in the form it went in")

		require.NoError(t, tokens.RevokeByOwner(ctx, created.ID, alice.Id))
		after, err := tokens.GetByTokenHash(ctx, "tokenhash-1")
		require.NoError(t, err)
		require.NotNil(t, after.RevokedAt, "RevokeByOwner must actually revoke")
	})
}

// TestAPITokenRevokeByOwnerRefusesAnotherOwner pins the ownership predicate,
// which is what stops one user revoking another's credential.
func TestAPITokenRevokeByOwnerRefusesAnotherOwner(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		bob := mustUser(t, store, "bob", "bob@example.com")

		created, err := store.APIToken().Create(ctx, alice.Id, "ci", "hades1_x", "tokenhash-2", nil, nil)
		require.NoError(t, err)

		err = store.APIToken().RevokeByOwner(ctx, created.ID, bob.Id)
		require.Error(t, err, "bob must not be able to revoke alice's token")

		still, err := store.APIToken().GetByTokenHash(ctx, "tokenhash-2")
		require.NoError(t, err)
		assert.Nil(t, still.RevokedAt)
	})
}

// TestDeviceGrantApprovalIsOnceOnly covers the re-approval hijack: approval was
// unconditional, so a second caller who learned the (short, human-readable)
// user code could re-point an issued grant at their own account.
func TestDeviceGrantApprovalIsOnceOnly(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		bob := mustUser(t, store, "bob", "bob@example.com")
		grants := store.DeviceGrant()

		id, err := grants.Create(ctx, "devicehash-1", "ABCD-1234", time.Now().Add(time.Hour))
		require.NoError(t, err)

		require.NoError(t, grants.Approve(ctx, id, alice.Id))

		err = grants.Approve(ctx, id, bob.Id)
		require.ErrorIs(t, err, devicegrant.ErrAlreadyApproved,
			"an approved grant must not be re-approvable onto another account")

		got, err := grants.GetByUserCode(ctx, "ABCD-1234")
		require.NoError(t, err)
		require.NotNil(t, got.UserID)
		assert.Equal(t, alice.Id, *got.UserID, "the first approver keeps the grant")
	})
}

// TestDeviceGrantTokenIsIssuedOnce covers the double-mint race: the check and
// the insert were separate statements with no constraint between them, so two
// interleaved polls could both mint a personal access token.
func TestDeviceGrantTokenIsIssuedOnce(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		grants := store.DeviceGrant()

		id, err := grants.Create(ctx, "devicehash-2", "EFGH-5678", time.Now().Add(time.Hour))
		require.NoError(t, err)
		require.NoError(t, grants.Approve(ctx, id, alice.Id))

		first, err := store.APIToken().Create(ctx, alice.Id, "device-flow", "hades1_a", "tokenhash-3", nil, nil)
		require.NoError(t, err)
		require.NoError(t, grants.AttachToken(ctx, id, first.ID))

		second, err := store.APIToken().Create(ctx, alice.Id, "device-flow", "hades1_b", "tokenhash-4", nil, nil)
		require.NoError(t, err)
		err = grants.AttachToken(ctx, id, second.ID)
		require.ErrorIs(t, err, devicegrant.ErrTokenAlreadyIssued,
			"a device grant must yield exactly one token")

		got, err := grants.GetByDeviceCodeHash(ctx, "devicehash-2")
		require.NoError(t, err)
		require.NotNil(t, got.APITokenID)
		assert.Equal(t, first.ID, *got.APITokenID,
			"the stored token link must be the one that won, and must join api_tokens")

		// The join is the point: on SQLite the column stored a hyphenated id
		// while api_tokens.id was dashless, so this lookup returned nothing.
		linked, err := store.APIToken().GetByID(ctx, *got.APITokenID)
		require.NoError(t, err, "device_grants.api_token_id must join api_tokens.id")
		assert.Equal(t, "tokenhash-3", linked.TokenHash)
	})
}

// TestSDKJobClaimHandsEachJobOutOnce covers the SQLite claim path, which did an
// unguarded SELECT then an unguarded UPDATE per row and could hand one job to
// two workers.
func TestSDKJobClaimHandsEachJobOutOnce(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		module := mustModule(t, store, "alice/mymod", alice.Id, moduleVisibilityPublic)

		commitID := uuid.New()
		require.NoError(t, store.Commit().Create(ctx, commitID, "aaaa000000000000000000000000000000000001",
			alice.Id, module.Id, digestTypeB5, "digest-1", alice.Id, ""))

		require.NoError(t, store.SDKJob().CreateBatch(ctx, commitID.String(), module.Id, goGenerator()))

		first, err := store.SDKJob().ClaimPending(ctx, 10)
		require.NoError(t, err)
		require.Len(t, first, 1)

		second, err := store.SDKJob().ClaimPending(ctx, 10)
		require.NoError(t, err)
		assert.Empty(t, second, "a claimed job must not be claimable again")

		// The commit link must resolve, which it did not when commits.id and
		// sdk_jobs.commit_id were written in different formats.
		assert.Equal(t, commitID.String(), first[0].CommitID)
		assert.Equal(t, module.Id, first[0].ModuleID)
	})
}

// TestOrgMembershipUpsertPreservesTheRow covers INSERT OR REPLACE on SQLite
// versus ON CONFLICT DO UPDATE on PostgreSQL: REPLACE is delete-then-insert, so
// it discarded the row and created a new one.
func TestOrgMembershipUpsertPreservesTheRow(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		bob := mustUser(t, store, "bob", "bob@example.com")

		org, err := store.Org().Create(ctx, "acme", "", "", alice.Id)
		require.NoError(t, err)

		require.NoError(t, store.Org().AddMember(ctx, org.Id, bob.Id, "member"))
		require.NoError(t, store.Org().AddMember(ctx, org.Id, bob.Id, "admin"))

		role, err := store.Org().GetMemberRole(ctx, org.Id, bob.Id)
		require.NoError(t, err)
		assert.Equal(t, "admin", role, "re-adding a member updates the role")

		count, err := store.Org().CountMembers(ctx, org.Id)
		require.NoError(t, err)
		assert.Equal(t, int32(2), count, "the upsert must not have duplicated the row")

		members, err := store.Org().ListMembers(ctx, org.Id)
		require.NoError(t, err)
		require.Len(t, members, 2)

		orgs, err := store.Org().GetUserOrgs(ctx, bob.Id)
		require.NoError(t, err)
		require.Len(t, orgs, 1, "the membership must join back to the org")
		assert.Equal(t, org.Id, orgs[0].Id)
	})
}

// TestGetMemberRoleDistinguishesAbsenceFromFailure pins the error a missing
// membership produces, because the handler branches on it.
func TestGetMemberRoleDistinguishesAbsenceFromFailure(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		bob := mustUser(t, store, "bob", "bob@example.com")
		org, err := store.Org().Create(ctx, "acme", "", "", alice.Id)
		require.NoError(t, err)

		_, err = store.Org().GetMemberRole(ctx, org.Id, bob.Id)
		require.Error(t, err, "a non-member must be an error, not an empty role")
	})
}

// TestVerificationAndResetTokensAreUnique covers the UNIQUE constraints SQLite
// was missing on the token hashes: without them two rows could share a hash and
// the lookup would match an arbitrary one.
func TestVerificationAndResetTokensAreUnique(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		expires := time.Now().Add(time.Hour)

		require.NoError(t, store.EmailVerification().Create(ctx, alice.Id, "verify-hash", expires))
		require.Error(t, store.EmailVerification().Create(ctx, alice.Id, "verify-hash", expires),
			"a duplicate verification token hash must be refused")

		require.NoError(t, store.PasswordReset().Create(ctx, alice.Id, "reset-hash", expires))
		require.Error(t, store.PasswordReset().Create(ctx, alice.Id, "reset-hash", expires),
			"a duplicate reset token hash must be refused")

		row, err := store.EmailVerification().GetByTokenHash(ctx, "verify-hash")
		require.NoError(t, err)
		assert.Equal(t, alice.Id, row.UserID)
		require.NoError(t, store.EmailVerification().MarkUsed(ctx, row.ID))

		used, err := store.EmailVerification().GetByTokenHash(ctx, "verify-hash")
		require.NoError(t, err)
		require.NotNil(t, used.UsedAt, "MarkUsed must persist")
	})
}

// TestDeviceCodeAndUserCodeAreUnique covers the other pair of UNIQUE
// constraints SQLite lacked. GetByUserCode decides which account a device-flow
// token is minted for, so matching an arbitrary row among duplicates matters.
func TestDeviceCodeAndUserCodeAreUnique(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		expires := time.Now().Add(time.Hour)
		_, err := store.DeviceGrant().Create(ctx, "dup-device", "DUP1-1111", expires)
		require.NoError(t, err)

		_, err = store.DeviceGrant().Create(ctx, "dup-device", "DUP2-2222", expires)
		require.Error(t, err, "a duplicate device code hash must be refused")

		_, err = store.DeviceGrant().Create(ctx, "other-device", "DUP1-1111", expires)
		require.Error(t, err, "a duplicate user code must be refused")
	})
}

// TestTOTPSecretAndBackupCodes covers the two stores that carry a second
// factor, neither of which had any test on either backend.
func TestTOTPSecretAndBackupCodes(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")

		require.NoError(t, store.TOTPSecret().Upsert(ctx, alice.Id, "enc-1"))
		row, err := store.TOTPSecret().GetByUserID(ctx, alice.Id)
		require.NoError(t, err)
		assert.Equal(t, "enc-1", row.SecretEnc)
		assert.False(t, row.Enabled, "a freshly enrolled secret is not yet enabled")

		require.NoError(t, store.TOTPSecret().Enable(ctx, alice.Id))
		row, err = store.TOTPSecret().GetByUserID(ctx, alice.Id)
		require.NoError(t, err)
		assert.True(t, row.Enabled)

		require.NoError(t, store.BackupCode().CreateBatch(ctx, alice.Id, []string{"c1", "c2"}))
		codes, err := store.BackupCode().ListByUserID(ctx, alice.Id)
		require.NoError(t, err)
		require.Len(t, codes, 2)

		unused, err := store.BackupCode().GetUnused(ctx, alice.Id, "c1")
		require.NoError(t, err)
		require.NoError(t, store.BackupCode().MarkUsed(ctx, unused.ID))

		_, err = store.BackupCode().GetUnused(ctx, alice.Id, "c1")
		require.Error(t, err, "a used backup code must not be usable again")

		require.NoError(t, store.TOTPSecret().Delete(ctx, alice.Id))
		_, err = store.TOTPSecret().GetByUserID(ctx, alice.Id)
		require.Error(t, err)
	})
}

// TestOAuthIdentityLinkAndUnlink covers the identity link table, which decides
// which account an OAuth login lands on.
func TestOAuthIdentityLinkAndUnlink(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		identities := store.OAuthIdentity()

		require.NoError(t, identities.Create(ctx, alice.Id, "github", "12345", "alice@example.com"))
		require.Error(t, identities.Create(ctx, alice.Id, "github", "12345", "alice@example.com"),
			"one provider identity must map to one account")

		got, err := identities.GetByProviderUID(ctx, "github", "12345")
		require.NoError(t, err)
		assert.Equal(t, alice.Id, got.UserID)

		list, err := identities.GetByUserID(ctx, alice.Id)
		require.NoError(t, err)
		require.Len(t, list, 1)

		require.NoError(t, identities.DeleteByUserAndProvider(ctx, alice.Id, "github"))
		_, err = identities.GetByProviderUID(ctx, "github", "12345")
		require.Error(t, err)
	})
}

// TestOPABindingsPerSubject covers the table every authorization decision reads.
func TestOPABindingsPerSubject(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		bindings := store.OPABinding()

		require.NoError(t, bindings.Create(ctx, "alice", "owner", "alice/*"))
		require.NoError(t, bindings.Create(ctx, "bob", "reader", "alice/mymod"))

		alice, err := bindings.ListBySubject(ctx, "alice")
		require.NoError(t, err)
		require.Len(t, alice, 1)
		assert.Equal(t, "owner", alice[0].Role)

		require.NoError(t, bindings.DeleteBySubjectDomain(ctx, "alice", "alice/*"))
		alice, err = bindings.ListBySubject(ctx, "alice")
		require.NoError(t, err)
		assert.Empty(t, alice, "a deleted binding must not still be returned")

		bob, err := bindings.ListBySubject(ctx, "bob")
		require.NoError(t, err)
		assert.Len(t, bob, 1, "deleting one subject's binding must not touch another's")
	})
}

// TestNotificationsAreScopedToTheirUser covers a table written on every push.
func TestNotificationsAreScopedToTheirUser(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		bob := mustUser(t, store, "bob", "bob@example.com")
		notifications := store.Notification()

		require.NoError(t, notifications.Create(ctx, alice.Id, "commit.pushed", "t", "b", ""))
		require.NoError(t, notifications.Create(ctx, bob.Id, "commit.pushed", "t", "b", ""))

		mine, err := notifications.ListForUser(ctx, alice.Id, 50, 0)
		require.NoError(t, err)
		require.Len(t, mine, 1, "a user sees only their own notifications")

		require.NoError(t, notifications.MarkRead(ctx, mine[0].Id, alice.Id))
		after, err := notifications.ListForUser(ctx, alice.Id, 50, 0)
		require.NoError(t, err)
		require.Len(t, after, 1)
		assert.True(t, after[0].Read, "MarkRead must persist")

		// Marking someone else's notification read must not work. It returned
		// success regardless of whether any row matched.
		bobs, err := notifications.ListForUser(ctx, bob.Id, 50, 0)
		require.NoError(t, err)
		require.Len(t, bobs, 1)
		_ = notifications.MarkRead(ctx, bobs[0].Id, alice.Id)
		bobs, err = notifications.ListForUser(ctx, bob.Id, 50, 0)
		require.NoError(t, err)
		assert.False(t, bobs[0].Read, "one user must not be able to read another's notification")
	})
}

// TestCommitDigestIsUniquePerModule pins the constraint that makes the upload
// dedup check meaningful. PostgreSQL has had it since migration 027; SQLite had
// no uniqueness on commits at all.
func TestCommitDigestIsUniquePerModule(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		one := mustModule(t, store, "alice/one", alice.Id, moduleVisibilityPublic)
		two := mustModule(t, store, "alice/two", alice.Id, moduleVisibilityPublic)

		require.NoError(t, store.Commit().Create(ctx, uuid.New(), "aaaa000000000000000000000000000000000001",
			alice.Id, one.Id, digestTypeB5, "shared-digest", alice.Id, ""))

		err := store.Commit().Create(ctx, uuid.New(), "aaaa000000000000000000000000000000000002",
			alice.Id, one.Id, digestTypeB5, "shared-digest", alice.Id, "")
		require.Error(t, err, "one module must not hold two commits with the same digest")

		require.NoError(t, store.Commit().Create(ctx, uuid.New(), "aaaa000000000000000000000000000000000003",
			alice.Id, two.Id, digestTypeB5, "shared-digest", alice.Id, ""),
			"two modules holding identical files legitimately share a digest")
	})
}

// TestCommitNotFoundIsTheSameErrorOnBothBackends pins the error a missing
// commit produces. SQLite returned a bare fmt.Errorf, which the handler
// boundary flattened to Internal, while PostgreSQL returned NotFound: the same
// call produced a different HTTP status depending on the configured backend.
func TestCommitNotFoundIsTheSameErrorOnBothBackends(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()

		_, err := store.Commit().GetByHash(ctx, "aaaa00000000000000000000000000000000dead")
		require.Error(t, err)
		assert.True(t, errors.Is(err, commitNotFound()),
			"a missing commit must be the shared not-found sentinel, got %v", err)

		_, err = store.Commit().GetByHashPrefix(ctx, "aaaa0000")
		require.Error(t, err)
		assert.True(t, errors.Is(err, commitNotFound()))
	})
}

// TestHashPrefixWildcardIsNotAWildcard covers the unescaped LIKE pattern: a
// caller supplying "%" matched every row and got an arbitrary commit back.
func TestHashPrefixWildcardIsNotAWildcard(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")
		module := mustModule(t, store, "alice/mymod", alice.Id, moduleVisibilityPublic)
		require.NoError(t, store.Commit().Create(ctx, uuid.New(), "aaaa000000000000000000000000000000000001",
			alice.Id, module.Id, digestTypeB5, "digest-w", alice.Id, ""))

		_, err := store.Commit().GetByHashPrefix(ctx, "%")
		require.Error(t, err, "a literal percent must match nothing, not everything")
	})
}

// TestGitalyOpLogExistsOnBothBackends covers the crash-compensation log, which
// was PostgreSQL-only and silently absent on the default backend.
func TestGitalyOpLogExistsOnBothBackends(t *testing.T) {
	eachBackend(t, func(t *testing.T, store db.Store) {
		ctx := context.Background()
		alice := mustUser(t, store, "alice", "alice@example.com")

		oplog := store.GitalyOpLog()
		require.NotNil(t, oplog, "every backend must have an operation log")

		id, err := oplog.CreatePending(ctx, "commit_files", "alice/mymod", alice.Id)
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, id)

		require.NoError(t, oplog.UpdateStatus(ctx, id, "completed", "abc123", ""))
		require.NoError(t, oplog.UpdateStatus(ctx, id, "failed", "", "gitaly unavailable"))
	})
}

// --- helpers ---------------------------------------------------------------

const (
	moduleVisibilityPublic = registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC
	digestTypeB5           = registryv1.DigestType_DIGEST_TYPE_B5
)

func moduleRefByID(id string) *registryv1.ModuleRef {
	return &registryv1.ModuleRef{Id: id}
}

func commitNotFound() error { return commitpkg.ErrNotFound }

func goGenerator() []config.GeneratorConfig {
	return []config.GeneratorConfig{{Language: "go", Plugin: "protoc-gen-go", Options: "paths=source_relative"}}
}
