package gitaly

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
)

// CommitService wraps the Gitaly CommitService gRPC client.
type CommitService struct {
	client             pb.CommitServiceClient
	defaultStorageName string
}

func newCommitService(conn *grpc.ClientConn, c config.Gitaly) *CommitService {
	return &CommitService{
		client:             pb.NewCommitServiceClient(conn),
		defaultStorageName: c.DefaultStorageName,
	}
}

// ListFiles returns the paths of all files at HEAD of the module referenced
// by the upload request.
//
// Deprecated: use ListFilesAtRef, which honours a revision.
func (c *CommitService) ListFiles(ctx context.Context, content *registryv1.UploadRequestContent) ([]string, error) {
	moduleName := fmt.Sprintf("%s/%s", content.ModuleRef.Owner, content.ModuleRef.Module)
	repo := &pb.Repository{
		StorageName:  c.defaultStorageName,
		RelativePath: moduleName,
		GlRepository: moduleName,
	}

	stream, err := c.client.ListFiles(ctx, &pb.ListFilesRequest{Repository: repo})
	if err != nil {
		return nil, err
	}
	var paths []string
	for {
		files, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		for _, p := range files.Paths {
			paths = append(paths, string(p))
		}
	}
	return paths, nil
}

// ListFilesAtRef returns the paths of all files at ref in repoPath.
//
// An empty ref reads HEAD. The old ListFiles ignored the revision entirely, so
// a caller asking for a specific commit was silently answered from the default
// branch.
func (c *CommitService) ListFilesAtRef(ctx context.Context, repoPath, ref string) ([]string, error) {
	repo := &pb.Repository{
		StorageName:  c.defaultStorageName,
		RelativePath: repoPath,
		GlRepository: repoPath,
	}

	stream, err := c.client.ListFiles(ctx, &pb.ListFilesRequest{
		Repository: repo,
		Revision:   revisionOrHead(ref),
	})
	if err != nil {
		return nil, err
	}
	var out []string
	for {
		files, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		for _, p := range files.Paths {
			out = append(out, string(p))
		}
	}
	return out, nil
}

// ListCommits returns up to limit commits reachable from ref, newest first.
func (c *CommitService) ListCommits(ctx context.Context, repoPath, ref string, limit int) ([]*git.CommitInfo, error) {
	repo := &pb.Repository{
		StorageName:  c.defaultStorageName,
		RelativePath: repoPath,
		GlRepository: repoPath,
	}

	stream, err := c.client.FindCommits(ctx, &pb.FindCommitsRequest{
		Repository: repo,
		Revision:   revisionOrHead(ref),
		// Clamped rather than converted: a caller-supplied limit that overflows
		// int32 would wrap negative and Gitaly would read that as unbounded.
		Limit: int32(min(limit, math.MaxInt32)), // #nosec G115 -- clamped on the line above.
	})
	if err != nil {
		return nil, err
	}

	out := make([]*git.CommitInfo, 0, limit)
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		for _, c := range msg.GetCommits() {
			info := &git.CommitInfo{
				SHA:     c.GetId(),
				Message: string(c.GetSubject()),
			}
			if a := c.GetAuthor(); a != nil {
				info.Author = string(a.GetName())
				info.Email = string(a.GetEmail())
				if ts := a.GetDate(); ts != nil {
					info.Timestamp = ts.AsTime()
				}
			}
			out = append(out, info)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}
