package operation

import (
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/models"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
)

type OperationService struct {
	client             pb.OperationServiceClient
	defaultStorageName string
}

func NewDefault(c config.Gitaly) (*OperationService, error) {
	conn, err := grpc.NewClient(fmt.Sprintf(":%d", c.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	client := pb.NewOperationServiceClient(conn)
	return &OperationService{
		client:             client,
		defaultStorageName: c.DefaultStorageName,
	}, nil
}

func (o *OperationService) UserCommitFiles(ctx context.Context, module *models.Module, files []*models.File, user *models.User, paths []string, digestValue string) (string, error) {
	stream, err := o.client.UserCommitFiles(ctx)
	if err != nil {
		return "", err
	}

	userPb := &pb.User{
		GlId:  user.ID.String(),
		Name:  []byte(user.Username),
		Email: []byte(user.Email),
	}

	repo := &pb.Repository{
		StorageName:  o.defaultStorageName,
		RelativePath: module.Name,
		GlRepository: module.Name, // TODO check
	}

	// TODO think about it
	commitMessage := fmt.Sprintf("module:%s\n\nupdate_by_user_id:%s\nat:%d\ndigest_value:%s", module.Name, user.ID, time.Now().Unix(), digestValue)
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
