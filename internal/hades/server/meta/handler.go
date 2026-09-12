// Package meta consolidates CIService and SDKService handlers.
package meta

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

type readAccessChecker interface {
	CheckReadAccess(ctx context.Context, user *identityv1.User, modules []*registrypbv1.Module) error
}

// Handler implements both CIService and SDKService handlers.
type Handler struct {
	registryv1connect.UnimplementedCIServiceHandler
	registryv1connect.UnimplementedSDKServiceHandler

	logger          *log.LoggerWrapper
	ciRunStorage    cirun.Storage
	sdkJobStorage   sdkjob.Storage
	moduleDBStorage moduledb.Storage
	authz           readAccessChecker
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:          deps.Logger,
		ciRunStorage:    deps.CIRunDB,
		sdkJobStorage:   deps.SDKJobDB,
		moduleDBStorage: deps.ModuleDB,
		authz:           deps.Authorization,
	}
}

func (h *Handler) GetCIRun(ctx context.Context, in *connect.Request[registrypbv1.GetCIRunRequest]) (*connect.Response[registrypbv1.GetCIRunResponse], error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

	userID := "anonymous"
	if user != nil {
		userID = user.Id
	}

	modules, err := h.moduleDBStorage.GetModulesByRefs(ctx, &registrypbv1.ModuleRef{
		Owner:  in.Msg.Owner,
		Module: in.Msg.ModuleName,
	})
	if err != nil || len(modules) == 0 {
		h.logger.Warn("module not found", "procedure", "GetCIRun", "user_id", userID, "owner", in.Msg.Owner, "module", in.Msg.ModuleName)
		return nil, connErr.NotFound("module not found")
	}

	if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
		return nil, err
	}

	run, err := h.ciRunStorage.GetByModuleAndCommit(ctx, modules[0].Id, in.Msg.CommitHash)
	if err != nil {
		h.logger.Warn("CI run not found", "error", err, "procedure", "GetCIRun", "user_id", userID, "module_id", modules[0].Id, "commit_hash", in.Msg.CommitHash)
		return nil, connErr.NotFound("CI run not found")
	}

	return &connect.Response[registrypbv1.GetCIRunResponse]{
		Msg: &registrypbv1.GetCIRunResponse{CiRun: run},
	}, nil
}

func (h *Handler) ListSDKs(ctx context.Context, in *connect.Request[registrypbv1.ListSDKsRequest]) (*connect.Response[registrypbv1.ListSDKsResponse], error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

	userID := "anonymous"
	if user != nil {
		userID = user.Id
	}

	modules, err := h.moduleDBStorage.GetModulesByRefs(ctx, &registrypbv1.ModuleRef{
		Owner:  in.Msg.Owner,
		Module: in.Msg.Module,
	})
	if err != nil || len(modules) == 0 {
		h.logger.Warn("module not found", "procedure", "ListSDKs", "user_id", userID, "owner", in.Msg.Owner, "module", in.Msg.Module)
		return nil, connErr.NotFound("module not found")
	}

	if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
		return nil, err
	}

	jobs, err := h.sdkJobStorage.ListByModule(ctx, modules[0].Id)
	if err != nil {
		h.logger.Error("failed to list SDK jobs", "error", err, "procedure", "ListSDKs", "user_id", userID, "module_id", modules[0].Id)
		return nil, connErr.FromDB(err)
	}

	sdkJobs := make([]*registrypbv1.SDKJob, 0, len(jobs))
	for _, j := range jobs {
		sj := &registrypbv1.SDKJob{
			Id:             j.ID,
			ModuleId:       j.ModuleID,
			CommitId:       j.CommitID,
			Language:       j.Language,
			Plugin:         j.Plugin,
			Status:         j.Status,
			OutputLocation: j.OutputLocation,
			ErrorMessage:   j.ErrorMessage,
			CreateTime:     timestamppb.New(j.CreatedAt),
		}
		if j.FinishedAt != nil {
			sj.UpdateTime = timestamppb.New(*j.FinishedAt)
		}
		sdkJobs = append(sdkJobs, sj)
	}

	return &connect.Response[registrypbv1.ListSDKsResponse]{
		Msg: &registrypbv1.ListSDKsResponse{SdkJobs: sdkJobs},
	}, nil
}
