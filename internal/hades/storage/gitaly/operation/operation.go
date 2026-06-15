// Package operation wraps the Gitaly OperationService gRPC client,
// providing commit and branch mutation operations.
package operation

import (
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
)

// OperationService wraps the Gitaly OperationService gRPC client.
type OperationService struct {
	client             pb.OperationServiceClient
	defaultStorageName string
}

// NewDefault dials the Gitaly server and returns an OperationService.
func NewDefault(c config.Gitaly) (*OperationService, error) {
	conn, err := grpc.NewClient(fmt.Sprintf("%s:%d", func() string { if c.Host != "" { return c.Host }; return "localhost" }(), c.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	client := pb.NewOperationServiceClient(conn)
	return &OperationService{
		client:             client,
		defaultStorageName: c.DefaultStorageName,
	}, nil
}

// UserCommitFiles writes files to a module's Gitaly repository in a single
// commit and returns the resulting commit hash. Existing paths in `paths`
// are updated; all others are created.
func (o *OperationService) UserCommitFiles(ctx context.Context, module *registryv1.Module, files []*registryv1.File, user *registryv1.User, paths []string, digestValue string) (string, error) {
	stream, err := o.client.UserCommitFiles(ctx)
	if err != nil {
		return "", err
	}

	userPb := &pb.User{
		GlId:  user.Id,
		Name:  []byte(user.Username),
		Email: []byte(user.Email),
	}

	repo := &pb.Repository{
		StorageName:  o.defaultStorageName,
		RelativePath: module.Name,
		GlRepository: module.Name,
	}

	commitMessage := fmt.Sprintf("module:%s\n\nupdate_by_user_id:%s\nat:%d\ndigest_value:%s", module.Name, user.Id, time.Now().Unix(), digestValue)
	err = stream.Send(&pb.UserCommitFilesRequest{
		UserCommitFilesRequestPayload: &pb.UserCommitFilesRequest_Header{
			Header: &pb.UserCommitFilesRequestHeader{
				Repository:    repo,
				User:          userPb,
				CommitMessage: []byte(commitMessage),
				BranchName:    []byte(module.DefaultBranch),
				Force:         true,

				StartRepository: repo,
			},
		},
	})
	if err != nil {
		return "", err
	}

	for _, file := range files {
		var op pb.UserCommitFilesActionHeader_ActionType
		if slices.Contains(paths, file.Path) {
			op = pb.UserCommitFilesActionHeader_UPDATE
		} else {
			op = pb.UserCommitFilesActionHeader_CREATE
		}
		err = stream.Send(&pb.UserCommitFilesRequest{
			UserCommitFilesRequestPayload: &pb.UserCommitFilesRequest_Action{
				Action: &pb.UserCommitFilesAction{
					UserCommitFilesActionPayload: &pb.UserCommitFilesAction_Header{
						Header: &pb.UserCommitFilesActionHeader{
							Action:        op,
							Base64Content: true,
							FilePath:      []byte(file.Path),
						},
					},
				},
			},
		})
		if err != nil {
			return "", err
		}

		base64Content := base64.StdEncoding.EncodeToString(file.Content)
		err = stream.Send(&pb.UserCommitFilesRequest{
			UserCommitFilesRequestPayload: &pb.UserCommitFilesRequest_Action{
				Action: &pb.UserCommitFilesAction{
					UserCommitFilesActionPayload: &pb.UserCommitFilesAction_Content{
						Content: []byte(base64Content),
					},
				},
			},
		})
		if err != nil {
			return "", err
		}
	}

	r, err := stream.CloseAndRecv()
	if err != nil {
		return "", err
	}

	return r.BranchUpdate.GetCommitId(), nil
}

// RollbackCommit resets module's default branch back to previousHead,
// undoing the commit written by UserCommitFiles.
// Called as a compensating action when a DB insert fails after a Gitaly write.
// If previousHead is empty (the module had no commits before) the branch is
// left in place - the orphan commit will be cleaned up by the background job
// that reads gitaly_operation_log.
func (o *OperationService) RollbackCommit(ctx context.Context, module *registryv1.Module, currentHead, previousHead string) error {
	if previousHead == "" {
		// No previous HEAD to reset to; the cleanup job handles this case.
		return nil
	}
	userPb := &pb.User{
		GlId:  "system",
		Name:  []byte("system"),
		Email: []byte("system@hades"),
	}
	resp, err := o.client.UserUpdateBranch(ctx, &pb.UserUpdateBranchRequest{
		Repository: &pb.Repository{
			StorageName:  o.defaultStorageName,
			RelativePath: module.Name,
			GlRepository: module.Name,
		},
		BranchName: []byte(module.DefaultBranch),
		User:       userPb,
		Newrev:     []byte(previousHead),
		Oldrev:     []byte(currentHead),
	})
	if err != nil {
		return err
	}
	if resp.GetPreReceiveError() != "" {
		return fmt.Errorf("rollback blocked by pre-receive hook: %s", resp.GetPreReceiveError())
	}
	return nil
}
