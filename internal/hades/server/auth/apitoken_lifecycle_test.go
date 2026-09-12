package auth_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	authorizationengine "github.com/alipourhabibi/Hades/internal/hades/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/auth"
	authorizationsvc "github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/testsupport"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
)

// The full personal API token lifecycle, and what a token's scopes actually
// permit, against real storage and the real policy engine.
//
// See docs/api-tokens.md for the worked examples these tests correspond to.

type tokenFixture struct {
	env   *testsupport.Env
	srv   *auth.Server
	authz *authorizationsvc.Server
	alice *identityv1.User
	ctx   context.Context
}

func newTokenFixture(t *testing.T) *tokenFixture {
	t.Helper()

	env := testsupport.NewEnv(t)
	engine, err := authorizationengine.New(context.Background(), env.DB.OPABinding(), cache.NewMemoryCache(), time.Minute)
	require.NoError(t, err)
	authz := authorizationsvc.NewServer(env.Logger, env.DB.User(), env.DB.Session(), engine)

	alice := env.CreateUser(t, "alice", "alice@example.com")
	require.NoError(t, authz.AddBasicRoles(context.Background(), "alice"))

	srv := auth.NewServer(&server.Dependencies{
		Logger:        env.Logger,
		UserDB:        env.DB.User(),
		SessionDB:     env.DB.Session(),
		APITokenDB:    env.DB.APIToken(),
		AuditLogDB:    env.DB.AuditLog(),
		Authorization: authz,
		UoW:           env.DB,
	})

	return &tokenFixture{
		env:   env,
		srv:   srv,
		authz: authz,
		alice: alice,
		ctx:   context.WithValue(context.Background(), constants.ContextKeyUser, alice),
	}
}

// --- creation --------------------------------------------------------------

func TestCreateAPIToken_ReturnsThePlaintextOnceAndStoresOnlyTheHash(t *testing.T) {
	f := newTokenFixture(t)

	resp, err := f.srv.CreateAPIToken(f.ctx, connect.NewRequest(&authv1.CreateAPITokenRequest{
		Name:   "ci",
		Scopes: []string{"module:read", "module:push"},
	}))
	require.NoError(t, err)

	plaintext := resp.Msg.Token
	assert.True(t, strings.HasPrefix(plaintext, utilscrypto.APITokenPrefix),
		"the prefix is what routes the credential to the API token path")
	assert.True(t, strings.HasPrefix(plaintext, resp.Msg.Prefix),
		"the displayed prefix must be a prefix of the real token")

	// The stored row carries the hash, and the token is findable by it.
	row, err := f.env.DB.APIToken().GetByTokenHash(context.Background(), utilscrypto.HashToken(plaintext))
	require.NoError(t, err)
	assert.Equal(t, resp.Msg.Id, row.ID.String())

	// The plaintext is never returned again.
	listed, err := f.srv.ListAPITokens(f.ctx, connect.NewRequest(&authv1.ListAPITokensRequest{}))
	require.NoError(t, err)
	require.Len(t, listed.Msg.Tokens, 1)
	assert.Equal(t, resp.Msg.Prefix, listed.Msg.Tokens[0].Prefix)
	assert.NotContains(t, listed.Msg.Tokens[0].String(), plaintext)
}

func TestCreateAPIToken_RejectsAnEmptyScopeList(t *testing.T) {
	// An empty list means unrestricted in the enforcement path, so a client that
	// simply omitted the field would receive a full-authority credential.
	f := newTokenFixture(t)

	_, err := f.srv.CreateAPIToken(f.ctx, connect.NewRequest(&authv1.CreateAPITokenRequest{Name: "ci"}))

	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestCreateAPIToken_RejectsUnknownAndDuplicateScopes(t *testing.T) {
	f := newTokenFixture(t)

	for name, scopes := range map[string][]string{
		"unknown action":     {"module:teleport"},
		"unknown resource":   {"commit:read"},
		"malformed":          {"module"},
		"empty domain":       {"module:read:"},
		"owner wildcard":     {"module:read:*/x"},
		"duplicate":          {"module:read", "module:read"},
		"bare wildcard":      {"*"},
		"three part garbage": {"module:read:a/b/c"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.srv.CreateAPIToken(f.ctx, connect.NewRequest(&authv1.CreateAPITokenRequest{
				Name: "ci", Scopes: scopes,
			}))
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

func TestCreateAPIToken_AcceptsEveryDocumentedScopeForm(t *testing.T) {
	f := newTokenFixture(t)

	for name, scopes := range map[string][]string{
		"action":            {"module:read"},
		"action wildcard":   {"module:*"},
		"single module":     {"module:read:alice/mymod"},
		"whole namespace":   {"module:push:alice/*"},
		"wildcard on scope": {"module:*:alice/mymod"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.srv.CreateAPIToken(f.ctx, connect.NewRequest(&authv1.CreateAPITokenRequest{
				Name: "ci-" + name, Scopes: scopes,
			}))
			assert.NoError(t, err)
		})
	}
}

func TestCreateAPIToken_RejectsAnExpiryInThePastOrTooFarOut(t *testing.T) {
	f := newTokenFixture(t)

	past := timestamppb.New(time.Now().Add(-time.Hour))
	_, err := f.srv.CreateAPIToken(f.ctx, connect.NewRequest(&authv1.CreateAPITokenRequest{
		Name: "ci", Scopes: []string{"module:read"}, ExpiresAt: past,
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	tooFar := timestamppb.New(time.Now().Add(400 * 24 * time.Hour))
	_, err = f.srv.CreateAPIToken(f.ctx, connect.NewRequest(&authv1.CreateAPITokenRequest{
		Name: "ci", Scopes: []string{"module:read"}, ExpiresAt: tooFar,
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestCreateAPIToken_CapsTheNumberOfLiveTokens(t *testing.T) {
	// A compromised session must not be able to mint credentials without bound.
	f := newTokenFixture(t)

	for i := 0; i < 50; i++ {
		_, err := f.srv.CreateAPIToken(f.ctx, connect.NewRequest(&authv1.CreateAPITokenRequest{
			Name: "ci", Scopes: []string{"module:read"},
		}))
		require.NoError(t, err, "token %d", i)
	}

	_, err := f.srv.CreateAPIToken(f.ctx, connect.NewRequest(&authv1.CreateAPITokenRequest{
		Name: "one too many", Scopes: []string{"module:read"},
	}))
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
}

func TestCreateAPIToken_AnonymousIsUnauthenticated(t *testing.T) {
	f := newTokenFixture(t)

	_, err := f.srv.CreateAPIToken(context.Background(), connect.NewRequest(&authv1.CreateAPITokenRequest{
		Name: "ci", Scopes: []string{"module:read"},
	}))

	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

// --- listing and revocation ------------------------------------------------

func TestListAPITokens_ReportsStatusAndHidesRevoked(t *testing.T) {
	f := newTokenFixture(t)

	live, err := f.srv.CreateAPIToken(f.ctx, connect.NewRequest(&authv1.CreateAPITokenRequest{
		Name: "live", Scopes: []string{"module:read"},
	}))
	require.NoError(t, err)
	doomed, err := f.srv.CreateAPIToken(f.ctx, connect.NewRequest(&authv1.CreateAPITokenRequest{
		Name: "doomed", Scopes: []string{"module:read"},
	}))
	require.NoError(t, err)

	_, err = f.srv.RevokeAPIToken(f.ctx, connect.NewRequest(&authv1.RevokeAPITokenRequest{Id: doomed.Msg.Id}))
	require.NoError(t, err)

	listed, err := f.srv.ListAPITokens(f.ctx, connect.NewRequest(&authv1.ListAPITokensRequest{}))
	require.NoError(t, err)
	require.Len(t, listed.Msg.Tokens, 1)
	assert.Equal(t, live.Msg.Id, listed.Msg.Tokens[0].Id)
	assert.Equal(t, authv1.APITokenStatus_API_TOKEN_STATUS_ACTIVE, listed.Msg.Tokens[0].Status)
}

func TestListAPITokens_ExpiredTokenIsListedAsExpired(t *testing.T) {
	f := newTokenFixture(t)
	expired := time.Now().Add(-time.Hour)
	_, err := f.env.DB.APIToken().Create(context.Background(), f.alice.Id, "old",
		"hades1_deadbeef", "oldhash", []string{"module:read"}, &expired)
	require.NoError(t, err)

	listed, err := f.srv.ListAPITokens(f.ctx, connect.NewRequest(&authv1.ListAPITokensRequest{}))

	require.NoError(t, err)
	require.Len(t, listed.Msg.Tokens, 1)
	assert.Equal(t, authv1.APITokenStatus_API_TOKEN_STATUS_EXPIRED, listed.Msg.Tokens[0].Status)
}

func TestRevokeAPIToken_AnotherUsersTokenIsNotFound(t *testing.T) {
	f := newTokenFixture(t)
	bob := f.env.CreateUser(t, "bob", "bob@example.com")
	bobToken, err := f.env.DB.APIToken().Create(context.Background(), bob.Id, "bob's",
		"hades1_deadbeef", "bobhash", []string{"module:read"}, nil)
	require.NoError(t, err)

	_, err = f.srv.RevokeAPIToken(f.ctx, connect.NewRequest(&authv1.RevokeAPITokenRequest{
		Id: bobToken.ID.String(),
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err),
		"a token the caller does not own is reported missing, not forbidden")

	row, err := f.env.DB.APIToken().GetByTokenHash(context.Background(), "bobhash")
	require.NoError(t, err)
	assert.Nil(t, row.RevokedAt, "bob's token is untouched")
}

func TestRevokeAPIToken_MalformedIdIsInvalidArgument(t *testing.T) {
	f := newTokenFixture(t)

	_, err := f.srv.RevokeAPIToken(f.ctx, connect.NewRequest(&authv1.RevokeAPITokenRequest{Id: "not-a-uuid"}))

	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// --- what a scope actually permits -----------------------------------------

// scopedCtx is what the interceptor builds for a request carrying a scoped
// personal API token.
func (f *tokenFixture) scopedCtx(scopes ...string) context.Context {
	return context.WithValue(f.ctx, constants.ContextKeyTokenScopes, scopes)
}

func (f *tokenFixture) module(t *testing.T, name string, visibility registryv1.ModuleVisibility) *registryv1.Module {
	t.Helper()
	return f.env.CreateModule(t, name, f.alice, visibility)
}

// TestScopedTokenCannotPushWithAReadScope is the case that motivated scopes in
// the first place: before they were enforced, any token could do anything its
// owner could.
func TestScopedTokenCannotPushWithAReadScope(t *testing.T) {
	f := newTokenFixture(t)

	push := &constants.Policy{
		Subject: "alice", ResourceType: "module", Action: "push", Domain: "alice/mymod",
	}

	unscoped, err := f.authz.Can(f.ctx, push)
	require.NoError(t, err)
	assert.True(t, unscoped.Allowed, "alice owns the namespace, so she may push")

	scoped, err := f.authz.Can(f.scopedCtx("module:read"), push)
	require.NoError(t, err)
	assert.False(t, scoped.Allowed, "a read-only token must not push on its owner's behalf")
}

// TestScopedTokenIsLimitedToTheNamedModule covers the three-part grammar: a
// token issued for one module must not reach a sibling.
func TestScopedTokenIsLimitedToTheNamedModule(t *testing.T) {
	f := newTokenFixture(t)
	mine := f.module(t, "alice/mymod", registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)
	other := f.module(t, "alice/other", registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)

	ctx := f.scopedCtx("module:read:alice/mymod")

	assert.NoError(t, f.authz.CheckReadAccess(ctx, f.alice, []*registryv1.Module{mine}))
	assert.Error(t, f.authz.CheckReadAccess(ctx, f.alice, []*registryv1.Module{other}),
		"a token naming alice/mymod must not read alice/other")
}

func TestScopedTokenNamespaceWildcardCoversTheWholeNamespace(t *testing.T) {
	f := newTokenFixture(t)
	first := f.module(t, "alice/one", registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)
	second := f.module(t, "alice/two", registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)

	ctx := f.scopedCtx("module:read:alice/*")

	assert.NoError(t, f.authz.CheckReadAccess(ctx, f.alice, []*registryv1.Module{first, second}))
}

// TestScopedTokenCannotExceedItsOwner is the rule that makes scopes safe to
// hand out: a scope narrows, it never grants.
func TestScopedTokenCannotExceedItsOwner(t *testing.T) {
	f := newTokenFixture(t)

	// alice holds no binding over bob's namespace, so no scope can help.
	allowed, err := f.authz.Can(f.scopedCtx("module:*:bob/*"), &constants.Policy{
		Subject: "alice", ResourceType: "module", Action: "push", Domain: "bob/theirs",
	})

	require.NoError(t, err)
	assert.False(t, allowed.Allowed)
}

// TestScopedTokenReadIsEnforcedOnPrivateAndPublicAlike covers the asymmetry in
// CheckReadAccess: a public module the token may not read is refused outright,
// while a private one is reported missing so its existence stays hidden.
func TestScopedTokenReadIsEnforcedOnPrivateAndPublicAlike(t *testing.T) {
	f := newTokenFixture(t)
	public := f.module(t, "alice/public", registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)
	private := f.module(t, "alice/private", registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)

	ctx := f.scopedCtx("module:read:alice/elsewhere")

	err := f.authz.CheckReadAccess(ctx, f.alice, []*registryv1.Module{public})
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	err = f.authz.CheckReadAccess(ctx, f.alice, []*registryv1.Module{private})
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// TestUnscopedTokenBehavesLikeTheOwner records the compatibility rule: tokens
// issued before scopes were required carry an empty list and are unrestricted.
func TestUnscopedTokenBehavesLikeTheOwner(t *testing.T) {
	f := newTokenFixture(t)
	private := f.module(t, "alice/private", registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)

	assert.NoError(t, f.authz.CheckReadAccess(f.scopedCtx(), f.alice, []*registryv1.Module{private}))
}
