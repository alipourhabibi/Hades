package module

import (
	"context"
	"testing"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	pkgerr "github.com/alipourhabibi/Hades/internal/errors"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes ---

type fakeModuleStorage struct {
	modules []*registrypbv1.Module
	err     error
}

func (f *fakeModuleStorage) GetModulesByRefs(_ context.Context, _ ...*registrypbv1.ModuleRef) ([]*registrypbv1.Module, error) {
	return f.modules, f.err
}

func (f *fakeModuleStorage) ListModules(_ context.Context, _ string) ([]*registrypbv1.Module, error) {
	return f.modules, f.err
}

func (f *fakeModuleStorage) GetModuleByOwnerAndName(_ context.Context, _, _ string) (*registrypbv1.Module, error) {
	if len(f.modules) == 0 {
		return nil, f.err
	}
	return f.modules[0], f.err
}

func (f *fakeModuleStorage) WithTx(_ pgx.Tx) *moduledb.ModuleStorage {
	return nil // never called in GetModules tests
}

type fakeAuth struct{ err error }

func (f *fakeAuth) CheckReadAccess(_ context.Context, _ *registrypbv1.User, _ []*registrypbv1.Module) error {
	return f.err
}

func (f *fakeAuth) Can(_ context.Context, _ *constants.Policy) (*constants.CanResponse, error) {
	return &constants.CanResponse{Allowed: true}, nil
}

func (f *fakeAuth) AddBasicRolesInTx(_ context.Context, _ pgx.Tx, _ string) error { return nil }

func (f *fakeAuth) ReloadPolicy() error { return nil }

// --- helpers ---

func newGetModulesServer(ms moduleStorage, auth authService) *Server {
	return &Server{moduleDBStorage: ms, authorization: auth, logger: log.DefaultLogger()}
}

func ctxWithUser(user *registrypbv1.User) context.Context {
	return context.WithValue(context.Background(), constants.ContextKeyUser, user)
}

var testUser = &registrypbv1.User{Id: "uid-1", Username: "alice"}

// --- tests ---

// TestGetModules_AnonymousAccess verifies that calling GetModules without a
// user in context (anonymous) succeeds - no user is required for public modules.
func TestGetModules_AnonymousAccess(t *testing.T) {
	s := newGetModulesServer(&fakeModuleStorage{}, &fakeAuth{})
	got, err := s.GetModules(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGetModules_StorageError(t *testing.T) {
	dbErr := pkgerr.New("not found", pkgerr.NotFound)
	s := newGetModulesServer(&fakeModuleStorage{err: dbErr}, &fakeAuth{})
	_, err := s.GetModules(ctxWithUser(testUser), []*registrypbv1.ModuleRef{{Owner: "alice", Module: "m"}})
	assert.ErrorIs(t, err, dbErr)
}

func TestGetModules_AccessDenied(t *testing.T) {
	authErr := pkgerr.New("denied", pkgerr.PermissionDenied)
	s := newGetModulesServer(
		&fakeModuleStorage{modules: []*registrypbv1.Module{{Id: "m1"}}},
		&fakeAuth{err: authErr},
	)
	_, err := s.GetModules(ctxWithUser(testUser), []*registrypbv1.ModuleRef{{Owner: "alice", Module: "m"}})
	assert.ErrorIs(t, err, authErr)
}

func TestGetModules_Success(t *testing.T) {
	want := []*registrypbv1.Module{{Id: "m1", Name: "alice/repo"}}
	s := newGetModulesServer(&fakeModuleStorage{modules: want}, &fakeAuth{})
	got, err := s.GetModules(ctxWithUser(testUser), []*registrypbv1.ModuleRef{{Owner: "alice", Module: "repo"}})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
