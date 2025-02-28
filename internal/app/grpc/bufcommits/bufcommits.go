package bufcommits

import (
	"context"
	"encoding/hex"

	"buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	"github.com/alipourhabibi/Hades/models"
	"github.com/alipourhabibi/Hades/internal/pkg/services/bufcommits"
	"github.com/alipourhabibi/Hades/utils/log"
)

type Server struct {
	modulev1connect.CommitServiceHandler

	service *bufcommits.Service
	logger  *log.LoggerWrapper
}

func NewServer(l *log.LoggerWrapper, service *bufcommits.Service) *Server {
	return &Server{
		logger:  l,
		service: service,
	}
}

func (s *Server) GetCommits(ctx context.Context, req *connect.Request[modulev1.GetCommitsRequest]) (*connect.Response[modulev1.GetCommitsResponse], error) {
	// TODO we may need to use resourceRefs model instead of moduleRef because of label and ref
	refs := make([]*models.ModuleRef, 0, len(req.Msg.ResourceRefs))
	for _, r := range req.Msg.ResourceRefs {
		ref := &models.ModuleRef{
			Id:     r.GetId(),
			Owner:  r.GetName().GetOwner(),
			Module: r.GetName().GetModule(),
		}
		refs = append(refs, ref)
	}

	commits, err := s.service.GetLastCommitForRefs(ctx, refs)
	if err != nil {
		return nil, err
	}

	moduleV1Commits := make([]*modulev1.Commit, 0, len(commits))
	for _, c := range commits {
		mv1commit := models.ToCommitPB(c)
		// TODO better way?
		mv1commit.Digest.Value, _ = hex.DecodeString(string(mv1commit.Digest.Value))
		// dig, err := shake256.NewDigestForContent(bytes.NewReader(mv1commit.Digest.Value))
		// if err != nil {
		// 	return nil, err
		// }
		// mv1commit.Digest.Value = dig.Value()
		moduleV1Commits = append(moduleV1Commits, mv1commit)
	}

	return &connect.Response[modulev1.GetCommitsResponse]{
		Msg: &modulev1.GetCommitsResponse{
			Commits: moduleV1Commits,
		},
	}, nil
}
