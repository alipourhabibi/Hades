package bufgraph

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

type fakeGraphProvider struct {
	commits []*registryv1.Commit
	err     error
}

func (f *fakeGraphProvider) GetGraph(_ context.Context, _ []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	return f.commits, f.err
}

func newServer(h graphProvider) *Server {
	return &Server{handler: h}
}

func resourceRef(owner, module string) *modulev1.ResourceRef {
	return &modulev1.ResourceRef{
		Value: &modulev1.ResourceRef_Name_{
			Name: &modulev1.ResourceRef_Name{
				Owner:  owner,
				Module: module,
			},
		},
	}
}

func TestGetGraph_HandlerError(t *testing.T) {
	handlerErr := pkgerr.New("not found", pkgerr.NotFound)
	s := newServer(&fakeGraphProvider{err: handlerErr})
	req := connect.NewRequest(&modulev1.GetGraphRequest{
		ResourceRefs: []*modulev1.ResourceRef{resourceRef("alice", "m")},
	})
	_, err := s.GetGraph(context.Background(), req)
	assert.ErrorIs(t, err, handlerErr)
}

func TestGetGraph_ConvertsResult(t *testing.T) {
	commits := []*registryv1.Commit{{Id: "c1", CommitHash: "abc123", Digest: &registryv1.Digest{}}}
	s := newServer(&fakeGraphProvider{commits: commits})
	req := connect.NewRequest(&modulev1.GetGraphRequest{
		ResourceRefs: []*modulev1.ResourceRef{resourceRef("alice", "m")},
	})
	resp, err := s.GetGraph(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Graph)
	assert.Len(t, resp.Msg.Graph.Commits, 1)
}

func TestGetGraph_EmptyRequest(t *testing.T) {
	s := newServer(&fakeGraphProvider{commits: nil})
	req := connect.NewRequest(&modulev1.GetGraphRequest{})
	resp, err := s.GetGraph(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Graph)
	assert.Empty(t, resp.Msg.Graph.Commits)
}

func TestGetGraph_PropagatesAnyError(t *testing.T) {
	handlerErr := errors.New("upstream failure")
	s := newServer(&fakeGraphProvider{err: handlerErr})
	req := connect.NewRequest(&modulev1.GetGraphRequest{
		ResourceRefs: []*modulev1.ResourceRef{resourceRef("alice", "m")},
	})
	_, err := s.GetGraph(context.Background(), req)
	assert.ErrorIs(t, err, handlerErr)
}
