// Package tree wraps Gitaly gRPC clients to serve directory listings and
// file content from module repositories.
package tree

import (
	"fmt"

	"github.com/alipourhabibi/Hades/config"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TreeService wraps Gitaly's CommitServiceClient and BlobServiceClient to
// serve directory listings and file content.
type TreeService struct {
	commitClient       pb.CommitServiceClient
	blobClient         pb.BlobServiceClient
	defaultStorageName string
}

// NewDefault dials the Gitaly server and returns a TreeService.
func NewDefault(c config.Gitaly) (*TreeService, error) {
	conn, err := grpc.NewClient(fmt.Sprintf("%s:%d", func() string { if c.Host != "" { return c.Host }; return "localhost" }(), c.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &TreeService{
		commitClient:       pb.NewCommitServiceClient(conn),
		blobClient:         pb.NewBlobServiceClient(conn),
		defaultStorageName: c.DefaultStorageName,
	}, nil
}
