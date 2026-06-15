package bufmodules

import (
	"context"
	"errors"
	"testing"

	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	pkgerr "github.com/alipourhabibi/Hades/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeModulesProvider implements modulesProvider for tests.
type fakeModulesProvider struct {
	modules []*registryv1.Module
	err     error
}

func (f *fakeModulesProvider) GetModules(_ context.Context, _ []*registryv1.ModuleRef) ([]*registryv1.Module, error) {
	return f.modules, f.err
}

func newServer(h modulesProvider) *Server {
	return &Server{handler: h}
}

func moduleRef(owner, module string) *modulev1.ModuleRef {
	return &modulev1.ModuleRef{
		Value: &modulev1.ModuleRef_Name_{
			Name: &modulev1.ModuleRef_Name{
				Owner:  owner,
				Module: module,
			},
		},
	}
}

func TestGetModules_HandlerError(t *testing.T) {
	handlerErr := pkgerr.New("denied", pkgerr.PermissionDenied)
	s := newServer(&fakeModulesProvider{err: handlerErr})
	req := connect.NewRequest(&modulev1.GetModulesRequest{
		ModuleRefs: []*modulev1.ModuleRef{moduleRef("alice", "m")},
	})
	_, err := s.GetModules(context.Background(), req)
	assert.ErrorIs(t, err, handlerErr)
}

func TestGetModules_ConvertsResult(t *testing.T) {
	modules := []*registryv1.Module{{Id: "m1", Name: "alice/repo"}}
	s := newServer(&fakeModulesProvider{modules: modules})
	req := connect.NewRequest(&modulev1.GetModulesRequest{
		ModuleRefs: []*modulev1.ModuleRef{moduleRef("alice", "repo")},
	})
	resp, err := s.GetModules(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, resp.Msg.Modules, 1)
	// ToBufModulePB maps Id → Id in the buf proto
	assert.NotEmpty(t, resp.Msg.Modules[0])
}

func TestGetModules_EmptyRequest(t *testing.T) {
	s := newServer(&fakeModulesProvider{modules: nil})
	req := connect.NewRequest(&modulev1.GetModulesRequest{})
	resp, err := s.GetModules(context.Background(), req)
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Modules)
}

func TestGetModules_PropagatesError(t *testing.T) {
	handlerErr := errors.New("storage failure")
	s := newServer(&fakeModulesProvider{err: handlerErr})
	req := connect.NewRequest(&modulev1.GetModulesRequest{
		ModuleRefs: []*modulev1.ModuleRef{moduleRef("alice", "m")},
	})
	_, err := s.GetModules(context.Background(), req)
	assert.ErrorIs(t, err, handlerErr)
}
