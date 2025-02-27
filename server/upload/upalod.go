package upload

import (
	"context"

	modulev1connect "buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	"github.com/alipourhabibi/Hades/models"
	pkgerr "github.com/alipourhabibi/Hades/pkg/errors"
	"github.com/alipourhabibi/Hades/pkg/services/authorization"
	"github.com/alipourhabibi/Hades/pkg/services/upload"
	"github.com/alipourhabibi/Hades/utils/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	modulev1connect.UploadServiceHandler

	service *upload.Service

	logger               *log.LoggerWrapper
	authorizationService *authorization.Service
}

func NewServer(l *log.LoggerWrapper, service *upload.Service, authorization *authorization.Service) *Server {
	return &Server{
		authorizationService: authorization,
		logger:               l,
		service:              service,
	}
}

func (s *Server) Upload(ctx context.Context, req *connect.Request[modulev1.UploadRequest]) (*connect.Response[modulev1.UploadResponse], error) {

	user, ok := ctx.Value("user").(*models.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	for _, content := range req.Msg.Contents {
		moduleFullName := content.ModuleRef.GetName().GetOwner() + "/" + content.ModuleRef.GetName().GetModule()
		pol := &models.Policy{
			Subject: user.Username,
			Object:  string(models.REPOSITORY),
			Action:  string(models.PUSH),
			Domain:  moduleFullName,
		}
		can, err := s.authorizationService.Can(ctx, pol)
		if err != nil {
			return nil, pkgerr.FromCasbin(err)
		}
		if !can.Allowed {
			return nil, status.Errorf(codes.PermissionDenied, "Permission Denied for %s", moduleFullName)
		}
	}

	uploadRequest := &models.UploadRequest{}
	for _, r := range req.Msg.Contents {
		content := &models.UploadRequest_Content{
			ModuleRef: &models.ModuleRef{
				Id:     r.ModuleRef.GetId(),
				Owner:  r.ModuleRef.GetName().GetOwner(),
				Module: r.ModuleRef.GetName().GetModule(),
			},
			Files: make([]*models.File, 0, len(r.Files)),
		}

		for _, f := range r.Files {
			content.Files = append(content.Files, &models.File{
				Path:    f.Path,
				Content: f.Content,
			})
		}

		uploadRequest.Contents = append(uploadRequest.Contents, content)
	}

	commits, err := s.service.Upload(ctx, uploadRequest)
	if err != nil {
		return nil, err
	}

	responseCommits := []*modulev1.Commit{}
	for _, v := range commits {
		responseCommits = append(responseCommits, models.ToCommitPB(v))
	}

	return &connect.Response[modulev1.UploadResponse]{
		Msg: &modulev1.UploadResponse{
			Commits: responseCommits,
		},
	}, nil

}
