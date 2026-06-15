package graph

import (
	"context"
	"errors"
	"testing"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	pkgerr "github.com/alipourhabibi/Hades/internal/errors"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes ---

type fakeModuleDB struct {
	modules []*registryv1.Module
	err     error
}

func (f *fakeModuleDB) GetModulesByRefs(_ context.Context, _ ...*registryv1.ModuleRef) ([]*registryv1.Module, error) {
	return f.modules, f.err
}

type fakeCommitDB struct {
	commits []*registryv1.Commit
	err     error
}

func (f *fakeCommitDB) GetCommitByOwnerModule(_ context.Context, _ []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	return f.commits, f.err
}

type fakeAuthz struct{ err error }

func (f *fakeAuthz) CheckReadAccess(_ context.Context, _ *registryv1.User, _ []*registryv1.Module) error {
	return f.err
}

// --- helpers ---

func newHandler(moduleDB moduleQuerier, commitDB commitQuerier, authz readAccessChecker) *Handler {
	return &Handler{moduleDB: moduleDB, commitDB: commitDB, authz: authz}
}

func ctxWithUser(user *registryv1.User) context.Context {
	return context.WithValue(context.Background(), constants.ContextKeyUser, user)
}

var testUser = &registryv1.User{Id: "uid-1", Username: "alice"}

// --- tests ---

// TestGetGraph_AnonymousAccess verifies that calling GetGraph without a user
// in context (anonymous) succeeds - no user is required for public modules.
func TestGetGraph_AnonymousAccess(t *testing.T) {
	h := newHandler(&fakeModuleDB{}, &fakeCommitDB{}, &fakeAuthz{})
	got, err := h.GetGraph(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGetGraph_ModuleDBError(t *testing.T) {
	dbErr := pkgerr.New("not found", pkgerr.NotFound)
	h := newHandler(
		&fakeModuleDB{err: dbErr},
		&fakeCommitDB{},
		&fakeAuthz{},
	)
	_, err := h.GetGraph(ctxWithUser(testUser), []*registryv1.ModuleRef{{Owner: "alice", Module: "m"}})
	assert.ErrorIs(t, err, dbErr)
}

func TestGetGraph_AccessDenied(t *testing.T) {
	authErr := pkgerr.New("denied", pkgerr.PermissionDenied)
	h := newHandler(
		&fakeModuleDB{modules: []*registryv1.Module{{Id: "m1"}}},
		&fakeCommitDB{},
		&fakeAuthz{err: authErr},
	)
	_, err := h.GetGraph(ctxWithUser(testUser), []*registryv1.ModuleRef{{Owner: "alice", Module: "m"}})
	assert.ErrorIs(t, err, authErr)
}

func TestGetGraph_CommitDBError(t *testing.T) {
	dbErr := errors.New("db down")
	h := newHandler(
		&fakeModuleDB{modules: []*registryv1.Module{{Id: "m1"}}},
		&fakeCommitDB{err: dbErr},
		&fakeAuthz{},
	)
	_, err := h.GetGraph(ctxWithUser(testUser), []*registryv1.ModuleRef{{Owner: "alice", Module: "m"}})
	assert.ErrorIs(t, err, dbErr)
}

func TestGetGraph_Success(t *testing.T) {
	want := []*registryv1.Commit{{Id: "c1"}, {Id: "c2"}}
	h := newHandler(
		&fakeModuleDB{modules: []*registryv1.Module{{Id: "m1"}}},
		&fakeCommitDB{commits: want},
		&fakeAuthz{},
	)
	got, err := h.GetGraph(ctxWithUser(testUser), []*registryv1.ModuleRef{{Owner: "alice", Module: "m"}})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGetGraph_EmptyRefs(t *testing.T) {
	h := newHandler(
		&fakeModuleDB{},
		&fakeCommitDB{commits: nil},
		&fakeAuthz{},
	)
	got, err := h.GetGraph(ctxWithUser(testUser), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
