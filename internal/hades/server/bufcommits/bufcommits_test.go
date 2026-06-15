package bufcommits

import (
	"context"
	"testing"

	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	pkgerr "github.com/alipourhabibi/Hades/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCommitsProvider implements commitsProvider for tests.
type fakeCommitsProvider struct {
	commits []*registryv1.Commit
	err     error
}

func (f *fakeCommitsProvider) GetCommits(_ context.Context, _ []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	return f.commits, f.err
}

func newServer(h commitsProvider) *Server {
	return &Server{handler: h}
}

// nameRef builds a ResourceRef with owner/module and no label/ref (valid for GetCommits).
func nameRef(owner, module string) *modulev1.ResourceRef {
	return &modulev1.ResourceRef{
		Value: &modulev1.ResourceRef_Name_{
			Name: &modulev1.ResourceRef_Name{
				Owner:  owner,
				Module: module,
			},
		},
	}
}

// labelRef builds a ResourceRef with a label name (should be rejected).
func labelRef(owner, module, label string) *modulev1.ResourceRef {
	return &modulev1.ResourceRef{
		Value: &modulev1.ResourceRef_Name_{
			Name: &modulev1.ResourceRef_Name{
				Owner:  owner,
				Module: module,
				Child:  &modulev1.ResourceRef_Name_LabelName{LabelName: label},
			},
		},
	}
}

func TestGetCommits_LabelRefRejected(t *testing.T) {
	s := newServer(&fakeCommitsProvider{})
	req := connect.NewRequest(&modulev1.GetCommitsRequest{
		ResourceRefs: []*modulev1.ResourceRef{labelRef("alice", "m", "main")},
	})
	_, err := s.GetCommits(context.Background(), req)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, connect.CodeUnimplemented, ce.Code())
}

func TestGetCommits_HandlerError(t *testing.T) {
	handlerErr := pkgerr.New("not found", pkgerr.NotFound)
	s := newServer(&fakeCommitsProvider{err: handlerErr})
	req := connect.NewRequest(&modulev1.GetCommitsRequest{
		ResourceRefs: []*modulev1.ResourceRef{nameRef("alice", "m")},
	})
	_, err := s.GetCommits(context.Background(), req)
	assert.ErrorIs(t, err, handlerErr)
}

func TestGetCommits_ConvertsResult(t *testing.T) {
	commits := []*registryv1.Commit{{Id: "c1", CommitHash: "abc123", Digest: &registryv1.Digest{}}}
	s := newServer(&fakeCommitsProvider{commits: commits})
	req := connect.NewRequest(&modulev1.GetCommitsRequest{
		ResourceRefs: []*modulev1.ResourceRef{nameRef("alice", "m")},
	})
	resp, err := s.GetCommits(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Commits, 1)
}

func TestGetCommits_EmptyRequest(t *testing.T) {
	s := newServer(&fakeCommitsProvider{commits: nil})
	req := connect.NewRequest(&modulev1.GetCommitsRequest{})
	resp, err := s.GetCommits(context.Background(), req)
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Commits)
}
