package goproxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/utils/log"
)

// fakeAuthorizer records what it was asked and returns canned answers.
type fakeAuthorizer struct {
	user      *identityv1.User
	scopes    []string
	tokenErr  error
	readErr   error
	gotUser   *identityv1.User
	gotCalled bool
}

func (f *fakeAuthorizer) UserFromToken(_ context.Context, _ string) (*identityv1.User, []string, error) {
	if f.tokenErr != nil {
		return nil, nil, f.tokenErr
	}
	return f.user, f.scopes, nil
}

func (f *fakeAuthorizer) CheckReadAccess(_ context.Context, user *identityv1.User, _ []*registryv1.Module) error {
	f.gotCalled = true
	f.gotUser = user
	return f.readErr
}

// fakeModuleDB implements moduledb.Storage; only GetModuleByOwnerAndName is used.
type fakeModuleDB struct {
	mod *registryv1.Module
	err error
}

func (f *fakeModuleDB) Create(context.Context, string, string, registryv1.ModuleVisibility, registryv1.ModuleState, string, string, string, string, registryv1.LintPreset, bool) (*registryv1.Module, error) {
	return nil, nil
}
func (f *fakeModuleDB) Update(context.Context, *registryv1.UpdateModuleRequest) (*registryv1.Module, error) {
	return nil, nil
}
func (f *fakeModuleDB) ListModules(context.Context, string, int, int) ([]*registryv1.Module, error) {
	return nil, nil
}
func (f *fakeModuleDB) ListVisibleModules(context.Context, string, string, string, int, int) ([]*registryv1.Module, error) {
	return nil, nil
}
func (f *fakeModuleDB) GetModuleByOwnerAndName(context.Context, string, string) (*registryv1.Module, error) {
	return f.mod, f.err
}
func (f *fakeModuleDB) GetModulesByRefs(context.Context, ...*registryv1.ModuleRef) ([]*registryv1.Module, error) {
	return nil, nil
}
func (f *fakeModuleDB) CountByOwner(context.Context, string) (int32, error) { return 0, nil }

// fakeCommitDB implements commitdb.Storage; only GetByHashPrefix is used.
type fakeCommitDB struct {
	commit *registryv1.Commit
	err    error
}

func (f *fakeCommitDB) Create(context.Context, uuid.UUID, string, string, string, registryv1.DigestType, string, string, string) error {
	return nil
}
func (f *fakeCommitDB) GetCommitById(context.Context, string) (*registryv1.Commit, error) {
	return nil, nil
}
func (f *fakeCommitDB) GetCommitByDigest(context.Context, string, string) (*registryv1.Commit, error) {
	return nil, nil
}
func (f *fakeCommitDB) GetCommitByOwnerModule(context.Context, []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	return nil, nil
}
func (f *fakeCommitDB) ListByModule(context.Context, string, int, int) ([]*registryv1.Commit, error) {
	return nil, nil
}
func (f *fakeCommitDB) GetByHash(context.Context, string) (*registryv1.Commit, error) {
	return nil, nil
}
func (f *fakeCommitDB) GetByHashPrefix(context.Context, string) (*registryv1.Commit, error) {
	return f.commit, f.err
}
func (f *fakeCommitDB) DeleteByIds(context.Context, []string) error { return nil }

func testLogger(t *testing.T) *log.LoggerWrapper {
	t.Helper()
	l, err := log.NewWithConfig(config.Logger{Engine: "slog", Level: "error", Format: "text"})
	require.NoError(t, err)
	return l
}

func newTestHandler(t *testing.T, authz *fakeAuthorizer, mod *registryv1.Module) *Handler {
	t.Helper()
	return &Handler{
		moduleDB:     &fakeModuleDB{mod: mod},
		authz:        authz,
		registryHost: "example.com",
		logger:       testLogger(t),
	}
}

func request(t *testing.T, auth func(*http.Request)) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/go/example.com/gen/go/alice/mymod/@v/list", nil)
	if auth != nil {
		auth(r)
	}
	return r
}

func TestAuthorize_AnonymousPublicModuleAllowed(t *testing.T) {
	mod := &registryv1.Module{Id: "mod-1", Name: "alice/mymod"}
	authz := &fakeAuthorizer{}
	h := newTestHandler(t, authz, mod)

	got, status := h.authorize(request(t, nil), "alice", "mymod")

	assert.Equal(t, 0, status)
	assert.Equal(t, mod, got)
	assert.True(t, authz.gotCalled, "read access must always be checked")
	assert.Nil(t, authz.gotUser, "anonymous caller must reach CheckReadAccess as nil")
}

func TestAuthorize_AnonymousPrivateModuleIsNotFound(t *testing.T) {
	// CheckReadAccess reports private modules as NotFound so their existence
	// is never revealed; the proxy must surface that as 404, not 403.
	authz := &fakeAuthorizer{readErr: errors.New("not found")}
	h := newTestHandler(t, authz, &registryv1.Module{Id: "mod-1", Name: "alice/secret"})

	_, status := h.authorize(request(t, nil), "alice", "secret")

	assert.Equal(t, http.StatusNotFound, status)
}

func TestAuthorize_InvalidCredentialRejected(t *testing.T) {
	authz := &fakeAuthorizer{tokenErr: errors.New("invalid token")}
	h := newTestHandler(t, authz, &registryv1.Module{Id: "mod-1"})

	r := request(t, func(r *http.Request) { r.SetBasicAuth("user", "hades1_deadbeef_nope") })
	_, status := h.authorize(r, "alice", "mymod")

	assert.Equal(t, http.StatusUnauthorized, status)
	assert.False(t, authz.gotCalled, "a rejected credential must not reach the read check")
}

func TestAuthorize_ScopedTokenWithoutModuleReadForbidden(t *testing.T) {
	authz := &fakeAuthorizer{
		user:   &identityv1.User{Id: "u-1", Username: "alice"},
		scopes: []string{"module:push"},
	}
	h := newTestHandler(t, authz, &registryv1.Module{Id: "mod-1"})

	r := request(t, func(r *http.Request) { r.Header.Set("Authorization", "Bearer hades1_deadbeef_tok") })
	_, status := h.authorize(r, "alice", "mymod")

	assert.Equal(t, http.StatusForbidden, status)
}

func TestAuthorize_UnrestrictedTokenPassesUserThrough(t *testing.T) {
	user := &identityv1.User{Id: "u-1", Username: "alice"}
	authz := &fakeAuthorizer{user: user} // empty scopes = unrestricted
	mod := &registryv1.Module{Id: "mod-1"}
	h := newTestHandler(t, authz, mod)

	r := request(t, func(r *http.Request) { r.SetBasicAuth("alice", "hades1_deadbeef_tok") })
	got, status := h.authorize(r, "alice", "mymod")

	assert.Equal(t, 0, status)
	assert.Equal(t, mod, got)
	assert.Equal(t, user, authz.gotUser)
}

func TestAuthorize_MissingAuthorizerFailsClosed(t *testing.T) {
	h := newTestHandler(t, nil, &registryv1.Module{Id: "mod-1"})
	h.authz = nil

	_, status := h.authorize(request(t, nil), "alice", "mymod")

	assert.Equal(t, http.StatusServiceUnavailable, status)
}

func TestResolveCommit_RejectsCommitFromAnotherModule(t *testing.T) {
	// A caller authorised for mod-1 asks for a version whose commit belongs to
	// mod-2. Hashes resolve globally, so without the pin this would serve
	// content from a module the caller was never authorised for.
	h := newTestHandler(t, &fakeAuthorizer{}, nil)
	h.commitDB = &fakeCommitDB{commit: &registryv1.Commit{Id: "c-1", ModuleId: "mod-2"}}

	r := httptest.NewRequest(http.MethodGet, "/go/x/@v/v0.0.0-20240101000000-abcdef123456.info", nil)
	commit, status := h.resolveCommit(r, &registryv1.Module{Id: "mod-1"}, "v0.0.0-20240101000000-abcdef123456")

	assert.Nil(t, commit)
	assert.Equal(t, http.StatusNotFound, status)
}

func TestResolveCommit_AcceptsCommitFromSameModule(t *testing.T) {
	want := &registryv1.Commit{Id: "c-1", ModuleId: "mod-1"}
	h := newTestHandler(t, &fakeAuthorizer{}, nil)
	h.commitDB = &fakeCommitDB{commit: want}

	r := httptest.NewRequest(http.MethodGet, "/go/x/@v/v0.0.0-20240101000000-abcdef123456.info", nil)
	commit, status := h.resolveCommit(r, &registryv1.Module{Id: "mod-1"}, "v0.0.0-20240101000000-abcdef123456")

	assert.Equal(t, 0, status)
	assert.Equal(t, want, commit)
}

func TestCredentialFromRequest(t *testing.T) {
	cases := []struct {
		name string
		set  func(*http.Request)
		want string
	}{
		{"anonymous", nil, ""},
		{"basic auth password carries the token", func(r *http.Request) { r.SetBasicAuth("anyuser", "hades1_ab_tok") }, "hades1_ab_tok"},
		{"bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer hds_sess_tok") }, "hds_sess_tok"},
		{"bearer is case-insensitive and trimmed", func(r *http.Request) { r.Header.Set("Authorization", "bearer  hds_sess_tok ") }, "hds_sess_tok"},
		{"basic auth with empty password is anonymous", func(r *http.Request) { r.SetBasicAuth("anyuser", "") }, ""},
		{"unknown scheme", func(r *http.Request) { r.Header.Set("Authorization", "Digest abc") }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, credentialFromRequest(request(t, tc.set)))
		})
	}
}
