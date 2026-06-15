// Package diff wraps the Gitaly DiffService and CommitService gRPC clients
// to compute per-file commit diffs.
package diff

import (
	"fmt"

	"github.com/alipourhabibi/Hades/config"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// DiffService wraps Gitaly's DiffService and CommitService clients to compute
// commit diffs via the Gitaly RPC API.
type DiffService struct {
	diffClient         pb.DiffServiceClient
	commitClient       pb.CommitServiceClient
	defaultStorageName string
}

// NewDefault dials the Gitaly server and returns a DiffService.
func NewDefault(c config.Gitaly) (*DiffService, error) {
	conn, err := grpc.NewClient(fmt.Sprintf("%s:%d", func() string { if c.Host != "" { return c.Host }; return "localhost" }(), c.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &DiffService{
		diffClient:         pb.NewDiffServiceClient(conn),
		commitClient:       pb.NewCommitServiceClient(conn),
		defaultStorageName: c.DefaultStorageName,
	}, nil
}
