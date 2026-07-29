package bufdownload

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

type fakeDownloadProvider struct {
	contents []*registryv1.DownloadResponseContent
	err      error
}

func (f *fakeDownloadProvider) Download(_ context.Context, _ []string, _ []*registryv1.ModuleRef) ([]*registryv1.DownloadResponseContent, error) {
	return f.contents, f.err
}

type fakeResourceResolver struct {
	rt  resource.ResourceType
	err error
}

func (f *fakeResourceResolver) ResolveType(_ context.Context, _ string) (resource.ResourceType, error) {
	return f.rt, f.err
}

func newServer(h downloadProvider) *Server {
	return &Server{handler: h, resolver: &fakeResourceResolver{rt: resource.ResourceTypeCommit}}
}

func newServerWithResolver(h downloadProvider, r resourceResolver) *Server {
	return &Server{handler: h, resolver: r}
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

func downloadIDValue(id string) *modulev1.DownloadRequest_Value {
	return &modulev1.DownloadRequest_Value{
		ResourceRef: &modulev1.ResourceRef{
			Value: &modulev1.ResourceRef_Id{Id: id},
		},
	}
}

func TestDownload_HandlerError(t *testing.T) {
	handlerErr := errors.New("not found")
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

func TestDownload_CommitIDRef_RoutedCorrectly(t *testing.T) {
	contents := []*registryv1.DownloadResponseContent{
		{Commit: &registryv1.Commit{Id: "c1", Digest: &registryv1.Digest{}}},
	}
	s := newServerWithResolver(
		&fakeDownloadProvider{contents: contents},
		&fakeResourceResolver{rt: resource.ResourceTypeCommit},
	)
	req := connect.NewRequest(&modulev1.DownloadRequest{
		Values: []*modulev1.DownloadRequest_Value{downloadIDValue("some-commit-uuid")},
	})
	resp, err := s.Download(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Contents, 1)
}

func TestDownload_LabelIDRef_ReturnsUnimplemented(t *testing.T) {
	s := newServerWithResolver(
		&fakeDownloadProvider{},
		&fakeResourceResolver{rt: resource.ResourceTypeLabel},
	)
	req := connect.NewRequest(&modulev1.DownloadRequest{
		Values: []*modulev1.DownloadRequest_Value{downloadIDValue("some-label-uuid")},
	})
	_, err := s.Download(context.Background(), req)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, connect.CodeUnimplemented, ce.Code())
}
