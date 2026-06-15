// Package commit wraps the Gitaly CommitService gRPC client.
package commit

import (
	"fmt"

	"github.com/alipourhabibi/Hades/config"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// CommitService wraps the Gitaly CommitService gRPC client.
type CommitService struct {
	client             pb.CommitServiceClient
	defaultStorageName string
}

// NewDefault dials the Gitaly server and returns a CommitService.
func NewDefault(c config.Gitaly) (*CommitService, error) {
	conn, err := grpc.NewClient(fmt.Sprintf(":%d", c.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	client := pb.NewCommitServiceClient(conn)
	return &CommitService{
		client:             client,
		defaultStorageName: c.DefaultStorageName,
	}, nil
}
