package bufupload

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

type fakeUploadProvider struct {
	commits []*registryv1.Commit
	err     error
}

func (f *fakeUploadProvider) Upload(_ context.Context, _ []*registryv1.UploadRequestContent) ([]*registryv1.Commit, error) {
	return f.commits, f.err
}

func newServer(h uploadProvider) *Server {
	return &Server{handler: h}
}

func uploadContent(owner, module string) *modulev1.UploadRequest_Content {
	return &modulev1.UploadRequest_Content{
		ModuleRef: &modulev1.ModuleRef{
			Value: &modulev1.ModuleRef_Name_{
				Name: &modulev1.ModuleRef_Name{
					Owner:  owner,
					Module: module,
				},
			},
		},
	}
}

func TestUpload_HandlerError(t *testing.T) {
	handlerErr := pkgerr.New("permission denied", pkgerr.PermissionDenied)
	s := newServer(&fakeUploadProvider{err: handlerErr})
	req := connect.NewRequest(&modulev1.UploadRequest{
		Contents: []*modulev1.UploadRequest_Content{uploadContent("alice", "m")},
	})
	_, err := s.Upload(context.Background(), req)
	assert.ErrorIs(t, err, handlerErr)
}

func TestUpload_ConvertsResult(t *testing.T) {
	commits := []*registryv1.Commit{{Id: "c1", CommitHash: "abc123", Digest: &registryv1.Digest{}}}
	s := newServer(&fakeUploadProvider{commits: commits})
	req := connect.NewRequest(&modulev1.UploadRequest{
		Contents: []*modulev1.UploadRequest_Content{uploadContent("alice", "m")},
	})
	resp, err := s.Upload(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Commits, 1)
}

func TestUpload_EmptyRequest(t *testing.T) {
	s := newServer(&fakeUploadProvider{commits: nil})
	req := connect.NewRequest(&modulev1.UploadRequest{})
	resp, err := s.Upload(context.Background(), req)
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Commits)
}

func TestUpload_PropagatesAnyError(t *testing.T) {
	handlerErr := errors.New("gitaly unavailable")
	s := newServer(&fakeUploadProvider{err: handlerErr})
	req := connect.NewRequest(&modulev1.UploadRequest{
		Contents: []*modulev1.UploadRequest_Content{uploadContent("alice", "m")},
	})
	_, err := s.Upload(context.Background(), req)
	assert.ErrorIs(t, err, handlerErr)
}
