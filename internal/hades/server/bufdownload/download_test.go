package bufdownload

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

type fakeDownloadProvider struct {
	contents []*registryv1.DownloadResponseContent
	err      error
}

func (f *fakeDownloadProvider) Download(_ context.Context, _ []*registryv1.ModuleRef) ([]*registryv1.DownloadResponseContent, error) {
	return f.contents, f.err
}

func newServer(h downloadProvider) *Server {
	return &Server{handler: h}
}

func downloadValue(owner, module string) *modulev1.DownloadRequest_Value {
	return &modulev1.DownloadRequest_Value{
		ResourceRef: &modulev1.ResourceRef{
			Value: &modulev1.ResourceRef_Name_{
				Name: &modulev1.ResourceRef_Name{
					Owner:  owner,
					Module: module,
				},
			},
		},
	}
}

func TestDownload_HandlerError(t *testing.T) {
	handlerErr := pkgerr.New("not found", pkgerr.NotFound)
	s := newServer(&fakeDownloadProvider{err: handlerErr})
	req := connect.NewRequest(&modulev1.DownloadRequest{
		Values: []*modulev1.DownloadRequest_Value{downloadValue("alice", "m")},
	})
	_, err := s.Download(context.Background(), req)
	assert.ErrorIs(t, err, handlerErr)
}

func TestDownload_ConvertsResult(t *testing.T) {
	contents := []*registryv1.DownloadResponseContent{
		{
			Commit: &registryv1.Commit{Id: "c1", Digest: &registryv1.Digest{}},
			Files:  []*registryv1.File{{Path: "a.proto", Content: []byte("syntax=\"proto3\";")}}},
	}
	s := newServer(&fakeDownloadProvider{contents: contents})
	req := connect.NewRequest(&modulev1.DownloadRequest{
		Values: []*modulev1.DownloadRequest_Value{downloadValue("alice", "m")},
	})
	resp, err := s.Download(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, resp.Msg.Contents, 1)
}

func TestDownload_EmptyRequest(t *testing.T) {
	s := newServer(&fakeDownloadProvider{})
	req := connect.NewRequest(&modulev1.DownloadRequest{})
	resp, err := s.Download(context.Background(), req)
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Contents)
}

func TestDownload_PropagatesAnyError(t *testing.T) {
	handlerErr := errors.New("storage unavailable")
	s := newServer(&fakeDownloadProvider{err: handlerErr})
	req := connect.NewRequest(&modulev1.DownloadRequest{
		Values: []*modulev1.DownloadRequest_Value{downloadValue("alice", "m")},
	})
	_, err := s.Download(context.Background(), req)
	assert.ErrorIs(t, err, handlerErr)
}
