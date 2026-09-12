package gitaly

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// OperationService wraps the Gitaly OperationService gRPC client.
type OperationService struct {
	client             pb.OperationServiceClient
	defaultStorageName string
}

func newOperationService(conn *grpc.ClientConn, c config.Gitaly) *OperationService {
	return &OperationService{
		client:             pb.NewOperationServiceClient(conn),
		defaultStorageName: c.DefaultStorageName,
	}
}

// UserCommitFiles writes one commit to a module's Gitaly repository and
// returns the resulting commit hash.
//
// Deletions are expressed. The previous version emitted only CREATE and UPDATE
// actions, so a path removed from the module stayed in the repository forever.
//
// The ref update is a compare-and-swap on req.ExpectedHead rather than
// Force: true. Forcing meant two overlapping pushes each computed a tree from
// the same parent and the second discarded the first, with both returning
// success.
func (o *OperationService) UserCommitFiles(ctx context.Context, user *identityv1.User, req git.PutFilesRequest) (string, error) {
	stream, err := o.client.UserCommitFiles(ctx)
	if err != nil {
		return "", err
	}

	userPb := &pb.User{
		// The git author is identified by the user's id, not their email. GlId
		// is an identity, and an email address is a mutable attribute of one.
		GlId:  user.Id,
		Name:  []byte(user.Username),
		Email: []byte(user.Email),
	}

	repo := &pb.Repository{
		StorageName:  o.defaultStorageName,
		RelativePath: req.RepoPath,
		GlRepository: req.RepoPath,
	}

	// The digest is written from the parameter, not recovered by scanning the
	// message for a marker string.
	commitMessage := fmt.Sprintf("%s\n\nupdate_by_user_id:%s\nat:%d\ndigest_value:%s",
		req.Message, user.Id, time.Now().Unix(), req.Digest)

	err = stream.Send(&pb.UserCommitFilesRequest{
		UserCommitFilesRequestPayload: &pb.UserCommitFilesRequest_Header{
			Header: &pb.UserCommitFilesRequestHeader{
				Repository:    repo,
				User:          userPb,
				CommitMessage: []byte(commitMessage),
				BranchName:    []byte(req.Branch),
				Force:         false,
				// Gitaly rejects the update when the branch does not point
				// here, which is the compare-and-swap.
				ExpectedOldOid:  req.ExpectedHead,
				StartRepository: repo,
			},
		},
	})
	if err != nil {
		return "", err
	}

	existing := make(map[string]struct{}, len(req.ExistingPaths))
	for _, p := range req.ExistingPaths {
		existing[p] = struct{}{}
	}
	incoming := make(map[string]struct{}, len(req.Files))
	for _, f := range req.Files {
		incoming[f.Path] = struct{}{}
	}

	// Deletions first, so a path in both sets is written rather than removed.
	for _, p := range req.ExistingPaths {
		if _, kept := incoming[p]; kept {
			continue
		}
		if err := stream.Send(deleteAction(p)); err != nil {
			return "", err
		}
	}

	for _, file := range req.Files {
		op := pb.UserCommitFilesActionHeader_CREATE
		if _, ok := existing[file.Path]; ok {
			op = pb.UserCommitFilesActionHeader_UPDATE
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
		if isRefMoved(err) {
			return "", fmt.Errorf("%w: %w", git.ErrRefMoved, err)
		}
		return "", err
	}

	return r.BranchUpdate.GetCommitId(), nil
}

// deleteAction builds the stream message that removes one path.
func deleteAction(path string) *pb.UserCommitFilesRequest {
	return &pb.UserCommitFilesRequest{
		UserCommitFilesRequestPayload: &pb.UserCommitFilesRequest_Action{
			Action: &pb.UserCommitFilesAction{
				UserCommitFilesActionPayload: &pb.UserCommitFilesAction_Header{
					Header: &pb.UserCommitFilesActionHeader{
						Action:   pb.UserCommitFilesActionHeader_DELETE,
						FilePath: []byte(path),
					},
				},
			},
		},
	}
}

// isRefMoved reports whether err is Gitaly refusing the update because the
// branch does not point where the caller expected.
func isRefMoved(err error) bool {
	// grpcCode, not status.Code. status.Code is a bare type assertion, and this
	// package wraps everything it returns with fmt.Errorf("%w"), so the
	// assertion failed and this branch never fired: the classification fell
	// through to matching Gitaly's error prose, which is not part of any
	// contract and changes between releases. Losing it does not fail loudly,
	// it silently turns a compare-and-swap rejection into a generic error, and
	// the caller then treats a lost update as a server fault.
	if grpcCode(err) == codes.FailedPrecondition {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "expected_old_oid") ||
		strings.Contains(msg, "reference update") ||
		strings.Contains(msg, "not point to expected")
}

// RollbackCommit resets module's default branch back to previousHead.
// Called as a compensating action when a DB insert fails after a Gitaly write.
func (o *OperationService) RollbackCommit(ctx context.Context, module *registryv1.Module, currentHead, previousHead string) error {
	if previousHead == "" {
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
