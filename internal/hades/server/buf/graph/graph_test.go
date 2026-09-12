package bufgraph

import (
	"context"
	"errors"
	"testing"

	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGraphProvider struct {
	commits []*registryv1.Commit
	err     error
}

func (f *fakeGraphProvider) GetGraph(_ context.Context, _ []string, _ []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	return f.commits, f.err
}

type fakeResourceResolver struct {
	rt  resource.ResourceType
	err error
}

func (f *fakeResourceResolver) ResolveType(_ context.Context, _ string) (resource.ResourceType, error) {
	return f.rt, f.err
}

func newServer(h graphProvider) *Server {
	return &Server{handler: h, resolver: &fakeResourceResolver{rt: resource.ResourceTypeCommit}}
}

func newServerWithResolver(h graphProvider, r resourceResolver) *Server {
	return &Server{handler: h, resolver: r}
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

func idRef(id string) *modulev1.ResourceRef {
	return &modulev1.ResourceRef{
		Value: &modulev1.ResourceRef_Id{Id: id},
	}
}

func TestGetGraph_HandlerError(t *testing.T) {
	handlerErr := errors.New("not found")
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

func TestGetGraph_IDRefCommitType(t *testing.T) {
	commits := []*registryv1.Commit{{Id: "c1", Digest: &registryv1.Digest{}}}
	s := newServerWithResolver(
		&fakeGraphProvider{commits: commits},
		&fakeResourceResolver{rt: resource.ResourceTypeCommit},
	)
	req := connect.NewRequest(&modulev1.GetGraphRequest{
		ResourceRefs: []*modulev1.ResourceRef{idRef("some-commit-uuid")},
	})
	resp, err := s.GetGraph(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Graph.Commits, 1)
}

func TestGetGraph_IDRefModuleType(t *testing.T) {
	commits := []*registryv1.Commit{{Id: "c1", Digest: &registryv1.Digest{}}}
	s := newServerWithResolver(
		&fakeGraphProvider{commits: commits},
		&fakeResourceResolver{rt: resource.ResourceTypeModule},
	)
	req := connect.NewRequest(&modulev1.GetGraphRequest{
		ResourceRefs: []*modulev1.ResourceRef{idRef("some-module-uuid")},
	})
	resp, err := s.GetGraph(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Graph.Commits, 1)
}

func TestGetGraph_IDRefLabelType(t *testing.T) {
	s := newServerWithResolver(
		&fakeGraphProvider{},
		&fakeResourceResolver{rt: resource.ResourceTypeLabel},
	)
	req := connect.NewRequest(&modulev1.GetGraphRequest{
		ResourceRefs: []*modulev1.ResourceRef{idRef("some-label-uuid")},
	})
	_, err := s.GetGraph(context.Background(), req)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, connect.CodeUnimplemented, ce.Code())
}

// TestGetGraph_IDRefResolverError pins that a resolver failure is translated at
// this boundary rather than returned raw.
//
// The adapter used to return the resolver's error unchanged, and the error
// interceptor then flattened anything that was not already a connect error to
// Internal: an unregistered id came back as a 500, and a driver error came back
// as whatever text the driver produced.
func TestGetGraph_IDRefResolverError(t *testing.T) {
	s := newServerWithResolver(
		&fakeGraphProvider{},
		&fakeResourceResolver{err: resource.ErrNotFound},
	)
	req := connect.NewRequest(&modulev1.GetGraphRequest{
		ResourceRefs: []*modulev1.ResourceRef{idRef("unknown-uuid")},
	})
	_, err := s.GetGraph(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
