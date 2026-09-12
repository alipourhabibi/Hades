package identity_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	authorizationengine "github.com/alipourhabibi/Hades/internal/hades/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	authorizationsvc "github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/server/identity"
	"github.com/alipourhabibi/Hades/internal/hades/testsupport"
)

// User and organisation handlers against real storage and the real policy
// engine, so the membership writes and their paired OPA bindings are the ones
// that ship.

type fixture struct {
	env   *testsupport.Env
	h     *identity.Handler
	authz *authorizationsvc.Server
	alice *identityv1.User
	bob   *identityv1.User
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	env := testsupport.NewEnv(t)
	engine, err := authorizationengine.New(context.Background(), env.DB.OPABinding(), cache.NewMemoryCache(), time.Minute)
	require.NoError(t, err)
	authz := authorizationsvc.NewServer(env.Logger, env.DB.User(), env.DB.Session(), engine)

	h := identity.NewHandler(&server.Dependencies{
		Logger:        env.Logger,
		UserDB:        env.DB.User(),
		OrgDB:         env.DB.Org(),
		ModuleDB:      env.DB.Module(),
		Authorization: authz,
		UoW:           env.DB,
	})

	return &fixture{
		env:   env,
		h:     h,
		authz: authz,
		alice: env.CreateUser(t, "alice", "alice@example.com"),
		bob:   env.CreateUser(t, "bob", "bob@example.com"),
	}
}

func ctxFor(user *identityv1.User) context.Context {
	if user == nil {
		return context.Background()
	}
	return context.WithValue(context.Background(), constants.ContextKeyUser, user)
}

// --- users -----------------------------------------------------------------

func TestGetUser_ReturnsProfileAndCounts(t *testing.T) {
	f := newFixture(t)
	f.env.CreateModule(t, "alice/one", f.alice, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)
	f.env.CreateModule(t, "alice/two", f.alice, registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)

	resp, err := f.h.GetUser(ctxFor(f.alice), connect.NewRequest(&identityv1.GetUserRequest{Username: "alice"}))

	require.NoError(t, err)
	assert.Equal(t, "alice", resp.Msg.User.Username)
	assert.Equal(t, int32(2), resp.Msg.ModuleCount, "the count includes private modules")
	assert.Empty(t, resp.Msg.User.Password, "the password hash never leaves the server")
}

func TestGetUser_EmailIsVisibleOnlyToTheAccountItself(t *testing.T) {
	f := newFixture(t)

	own, err := f.h.GetUser(ctxFor(f.alice), connect.NewRequest(&identityv1.GetUserRequest{Username: "alice"}))
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", own.Msg.User.Email)

	other, err := f.h.GetUser(ctxFor(f.bob), connect.NewRequest(&identityv1.GetUserRequest{Username: "alice"}))
	require.NoError(t, err)
	assert.Empty(t, other.Msg.User.Email, "one user must not read another's address")

	anon, err := f.h.GetUser(ctxFor(nil), connect.NewRequest(&identityv1.GetUserRequest{Username: "alice"}))
	require.NoError(t, err)
	assert.Empty(t, anon.Msg.User.Email)
}

func TestGetUser_OrganisationIsNotAUser(t *testing.T) {
	f := newFixture(t)
	f.env.CreateOrg(t, "acme", f.alice)

	_, err := f.h.GetUser(ctxFor(f.alice), connect.NewRequest(&identityv1.GetUserRequest{Username: "acme"}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "organisations are read through GetOrg")
}

func TestListUsers_RequiresAuthenticationAndRedactsEmails(t *testing.T) {
	f := newFixture(t)

	_, err := f.h.ListUsers(ctxFor(nil), connect.NewRequest(&identityv1.ListUsersRequest{}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err),
		"member discovery is for members, not for unauthenticated scraping")

	resp, err := f.h.ListUsers(ctxFor(f.alice), connect.NewRequest(&identityv1.ListUsersRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Users, 2)
	for _, u := range resp.Msg.Users {
		if u.Username == "alice" {
			assert.Equal(t, "alice@example.com", u.Email)
		} else {
			assert.Empty(t, u.Email)
		}
	}
}

func TestListUsers_QueryFiltersAndExcludesOrganisations(t *testing.T) {
	f := newFixture(t)
	f.env.CreateOrg(t, "alicecorp", f.alice)

	resp, err := f.h.ListUsers(ctxFor(f.alice), connect.NewRequest(&identityv1.ListUsersRequest{Query: "alice"}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Users, 1)
	assert.Equal(t, "alice", resp.Msg.Users[0].Username)
}

func TestUpdateUser_WritesTheCallersOwnRowAndClearsOnEmpty(t *testing.T) {
	f := newFixture(t)

	updated, err := f.h.UpdateUser(ctxFor(f.alice), connect.NewRequest(&identityv1.UpdateUserRequest{
		Description: "proto wrangler", Url: "https://example.com",
	}))
	require.NoError(t, err)
	assert.Equal(t, "proto wrangler", updated.Msg.User.Description)

	// Every field is written on every call, so an empty string clears rather
	// than preserves. The proto says so; this is what says so is true.
	cleared, err := f.h.UpdateUser(ctxFor(f.alice), connect.NewRequest(&identityv1.UpdateUserRequest{}))
	require.NoError(t, err)
	assert.Empty(t, cleared.Msg.User.Description)
	assert.Empty(t, cleared.Msg.User.Url)
}

// --- organisations ---------------------------------------------------------

func TestCreateOrg_MakesTheCallerAnAdminWithAnOwnerBinding(t *testing.T) {
	f := newFixture(t)

	resp, err := f.h.CreateOrg(ctxFor(f.alice), connect.NewRequest(&identityv1.CreateOrgRequest{Name: "acme"}))
	require.NoError(t, err)
	assert.Equal(t, identityv1.UserType_USER_TYPE_ORGANIZATION, resp.Msg.Org.Type)

	role, err := f.env.DB.Org().GetMemberRole(context.Background(), resp.Msg.Org.Id, f.alice.Id)
	require.NoError(t, err)
	assert.Equal(t, "admin", role)

	// The membership row and the OPA binding are written together, so the
	// creator can immediately act on the namespace.
	allowed, err := f.authz.Can(ctxFor(f.alice), &constants.Policy{
		Subject: "alice", ResourceType: "module", Action: "create", Domain: "acme/anything",
	})
	require.NoError(t, err)
	assert.True(t, allowed.Allowed)
}

func TestCreateOrg_ReservedNameIsRefused(t *testing.T) {
	// An org name becomes the first path segment of every module it owns, so a
	// name that shadows a route is refused.
	f := newFixture(t)

	for _, name := range []string{"go", "gen", "settings"} {
		_, err := f.h.CreateOrg(ctxFor(f.alice), connect.NewRequest(&identityv1.CreateOrgRequest{Name: name}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "name %q", name)
	}
}

func TestCreateOrg_NameIsLowercasedLikeAUsername(t *testing.T) {
	// Users and organisations share one namespace and Register lowercases, so
	// an org that kept its capitals would be a second identity distinguishable
	// from a user's only by case.
	f := newFixture(t)

	resp, err := f.h.CreateOrg(ctxFor(f.alice), connect.NewRequest(&identityv1.CreateOrgRequest{Name: "  ACME  "}))
	require.NoError(t, err)
	assert.Equal(t, "acme", resp.Msg.Org.Username)

	// And it is the same org when looked up by the normalised name.
	got, err := f.h.GetOrg(ctxFor(f.alice), connect.NewRequest(&identityv1.GetOrgRequest{Name: "acme"}))
	require.NoError(t, err)
	assert.Equal(t, resp.Msg.Org.Id, got.Msg.Org.Id)
}

func TestCreateOrg_NameThatIsNotASafePathSegmentIsRefused(t *testing.T) {
	// An org name is the first path segment of every module it owns, on disk
	// and in every URL. See constants.ValidateName.
	f := newFixture(t)

	for _, name := range []string{"a/b", "..", ".git", "has space", "-leading", "trailing."} {
		_, err := f.h.CreateOrg(ctxFor(f.alice), connect.NewRequest(&identityv1.CreateOrgRequest{Name: name}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "name %q", name)
	}
}

func TestCreateOrg_NameTakenByAUserIsRefused(t *testing.T) {
	// Users and organisations share one namespace.
	f := newFixture(t)

	_, err := f.h.CreateOrg(ctxFor(f.alice), connect.NewRequest(&identityv1.CreateOrgRequest{Name: "bob"}))

	assert.Error(t, err)
}

func TestCreateOrg_AnonymousIsUnauthenticated(t *testing.T) {
	f := newFixture(t)

	_, err := f.h.CreateOrg(ctxFor(nil), connect.NewRequest(&identityv1.CreateOrgRequest{Name: "acme"}))

	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestUpdateOrg_RequiresAdmin(t *testing.T) {
	f := newFixture(t)
	f.mustCreateOrg(t, "acme")

	_, err := f.h.UpdateOrg(ctxFor(f.bob), connect.NewRequest(&identityv1.UpdateOrgRequest{
		OrgName: "acme", Description: "hijacked",
	}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	updated, err := f.h.UpdateOrg(ctxFor(f.alice), connect.NewRequest(&identityv1.UpdateOrgRequest{
		OrgName: "acme", Description: "the real thing",
	}))
	require.NoError(t, err)
	assert.Equal(t, "the real thing", updated.Msg.Org.Description)
}

func TestAddOrgMember_RequiresAdminAndGrantsABinding(t *testing.T) {
	f := newFixture(t)
	org := f.mustCreateOrg(t, "acme")

	_, err := f.h.AddOrgMember(ctxFor(f.bob), connect.NewRequest(&identityv1.AddOrgMemberRequest{
		OrgName: "acme", Username: "bob", Role: "admin",
	}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err),
		"a non-member must not be able to add themselves")

	_, err = f.h.AddOrgMember(ctxFor(f.alice), connect.NewRequest(&identityv1.AddOrgMemberRequest{
		OrgName: "acme", Username: "bob",
	}))
	require.NoError(t, err)

	role, err := f.env.DB.Org().GetMemberRole(context.Background(), org.Id, f.bob.Id)
	require.NoError(t, err)
	assert.Equal(t, "member", role, "the default role is member")

	// "member" maps to the contributor role in the policy: push, not administer.
	push, err := f.authz.Can(ctxFor(f.bob), &constants.Policy{
		Subject: "bob", ResourceType: "module", Action: "push", Domain: "acme/thing",
	})
	require.NoError(t, err)
	assert.True(t, push.Allowed)

	del, err := f.authz.Can(ctxFor(f.bob), &constants.Policy{
		Subject: "bob", ResourceType: "module", Action: "delete", Domain: "acme/thing",
	})
	require.NoError(t, err)
	assert.False(t, del.Allowed)
}

func TestAddOrgMember_UnknownUserIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.mustCreateOrg(t, "acme")

	_, err := f.h.AddOrgMember(ctxFor(f.alice), connect.NewRequest(&identityv1.AddOrgMemberRequest{
		OrgName: "acme", Username: "nosuch",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestRemoveOrgMember_RemovesMembershipAndBindingTogether(t *testing.T) {
	f := newFixture(t)
	org := f.mustCreateOrg(t, "acme")
	require.NoError(t, f.addMember(t, "bob", ""))

	_, err := f.h.RemoveOrgMember(ctxFor(f.alice), connect.NewRequest(&identityv1.RemoveOrgMemberRequest{
		OrgName: "acme", Username: "bob",
	}))
	require.NoError(t, err)

	_, err = f.env.DB.Org().GetMemberRole(context.Background(), org.Id, f.bob.Id)
	assert.Error(t, err, "the membership row is gone")

	push, err := f.authz.Can(ctxFor(f.bob), &constants.Policy{
		Subject: "bob", ResourceType: "module", Action: "push", Domain: "acme/thing",
	})
	require.NoError(t, err)
	assert.False(t, push.Allowed, "leaving the membership behind would leave live permissions")
}

// TestRemoveOrgMember_LastAdminIsRefused covers the guard that keeps an
// organisation administrable: UpdateOrg and AddOrgMember both require an
// existing admin, so removing the last one would leave nobody able to appoint a
// replacement.
func TestRemoveOrgMember_LastAdminIsRefused(t *testing.T) {
	f := newFixture(t)
	f.mustCreateOrg(t, "acme")

	_, err := f.h.RemoveOrgMember(ctxFor(f.alice), connect.NewRequest(&identityv1.RemoveOrgMemberRequest{
		OrgName: "acme", Username: "alice",
	}))

	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestRemoveOrgMember_SecondAdminCanLeave(t *testing.T) {
	f := newFixture(t)
	f.mustCreateOrg(t, "acme")
	require.NoError(t, f.addMember(t, "bob", "admin"))

	_, err := f.h.RemoveOrgMember(ctxFor(f.bob), connect.NewRequest(&identityv1.RemoveOrgMemberRequest{
		OrgName: "acme", Username: "bob",
	}))

	assert.NoError(t, err, "an admin may leave while another remains")
}

func TestRemoveOrgMember_AMemberMayRemoveThemselves(t *testing.T) {
	f := newFixture(t)
	f.mustCreateOrg(t, "acme")
	require.NoError(t, f.addMember(t, "bob", ""))

	_, err := f.h.RemoveOrgMember(ctxFor(f.bob), connect.NewRequest(&identityv1.RemoveOrgMemberRequest{
		OrgName: "acme", Username: "bob",
	}))

	assert.NoError(t, err)
}

func TestRemoveOrgMember_NonMemberIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.mustCreateOrg(t, "acme")

	_, err := f.h.RemoveOrgMember(ctxFor(f.alice), connect.NewRequest(&identityv1.RemoveOrgMemberRequest{
		OrgName: "acme", Username: "bob",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestGetOrg_IsReadableAnonymously(t *testing.T) {
	f := newFixture(t)
	f.mustCreateOrg(t, "acme")
	require.NoError(t, f.addMember(t, "bob", ""))
	f.env.CreateModule(t, "acme/thing", f.alice, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)

	org, err := f.h.GetOrg(ctxFor(nil), connect.NewRequest(&identityv1.GetOrgRequest{Name: "acme"}))
	require.NoError(t, err)
	assert.Equal(t, int32(2), org.Msg.MemberCount)
	assert.Equal(t, int32(0), org.Msg.ModuleCount, "the module is owned by alice, not by the org")
}

// TestListOrgMembers_RefusesAnonymousCallers pins the credential requirement.
//
// The roster used to be readable with no credential at all, which handed an
// unauthenticated visitor the org chart and a target list for credential
// attacks: organisation names are on every public module page, so nothing had
// to be guessed. This is not secrecy, it is attribution.
func TestListOrgMembers_RefusesAnonymousCallers(t *testing.T) {
	f := newFixture(t)
	f.mustCreateOrg(t, "acme")
	require.NoError(t, f.addMember(t, "bob", ""))

	_, err := f.h.ListOrgMembers(ctxFor(nil), connect.NewRequest(&identityv1.ListOrgMembersRequest{OrgName: "acme"}))

	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestListOrgMembers_RedactsOtherMembersAddresses(t *testing.T) {
	f := newFixture(t)
	f.mustCreateOrg(t, "acme")
	require.NoError(t, f.addMember(t, "bob", ""))

	members, err := f.h.ListOrgMembers(ctxFor(f.bob), connect.NewRequest(&identityv1.ListOrgMembersRequest{OrgName: "acme"}))

	require.NoError(t, err)
	require.Len(t, members.Msg.Members, 2)
	for _, m := range members.Msg.Members {
		if m.User.Username == f.bob.Username {
			assert.NotEmpty(t, m.User.Email, "the caller reads their own address")
			continue
		}
		assert.Empty(t, m.User.Email, "another member's address stays redacted")
	}
}

func TestGetUserOrgs_ListsMembershipsNotOwnership(t *testing.T) {
	f := newFixture(t)
	f.mustCreateOrg(t, "acme")
	require.NoError(t, f.addMember(t, "bob", ""))

	resp, err := f.h.GetUserOrgs(ctxFor(f.bob), connect.NewRequest(&identityv1.GetUserOrgsRequest{Username: "bob"}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Organizations, 1)
	assert.Equal(t, "acme", resp.Msg.Organizations[0].Username)
}

func TestListOrganizations_ExcludesUsers(t *testing.T) {
	f := newFixture(t)
	f.mustCreateOrg(t, "acme")

	resp, err := f.h.ListOrganizations(ctxFor(f.alice), connect.NewRequest(&identityv1.ListOrganizationsRequest{}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Organizations, 1)
	assert.Equal(t, "acme", resp.Msg.Organizations[0].Username)
}

// TestCreateUser_IsUnimplemented pins the fix for the nil-interface panic: the
// procedure is mounted and reachable, so it must answer rather than crash.
func TestCreateUser_IsUnimplemented(t *testing.T) {
	f := newFixture(t)

	var err error
	assert.NotPanics(t, func() {
		_, err = f.h.CreateUser(ctxFor(f.alice), connect.NewRequest(&identityv1.CreateUserRequest{
			Username: "carol", Email: "carol@example.com", Password: "hunter2hunter2",
		}))
	})
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

// --- helpers ---------------------------------------------------------------

func (f *fixture) mustCreateOrg(t *testing.T, name string) *identityv1.User {
	t.Helper()
	resp, err := f.h.CreateOrg(ctxFor(f.alice), connect.NewRequest(&identityv1.CreateOrgRequest{Name: name}))
	require.NoError(t, err)
	return resp.Msg.Org
}

func (f *fixture) addMember(t *testing.T, username, role string) error {
	t.Helper()
	_, err := f.h.AddOrgMember(ctxFor(f.alice), connect.NewRequest(&identityv1.AddOrgMemberRequest{
		OrgName: "acme", Username: username, Role: role,
	}))
	return err
}
