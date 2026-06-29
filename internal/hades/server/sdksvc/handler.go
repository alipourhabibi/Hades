// Package sdksvc implements the SDKService ConnectRPC handler.
// It exposes SDK code-generation job status for schema modules via:
//   - ListSDKs - returns all SDK jobs for a given owner/module, newest first.
//
// The handler enforces OPA read-access checks so that callers cannot list
// SDK jobs for private modules they are not authorised to read.
package sdksvc

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// readAccessChecker is the subset of the authorization Server used by Handler.
// Defined as an interface so that tests can substitute a fake.
type readAccessChecker interface {
	CheckReadAccess(ctx context.Context, user *registrypbv1.User, modules []*registrypbv1.Module) error
}

// Handler implements the SDKService ConnectRPC handler.
type Handler struct {
	registryv1connect.SDKServiceHandler

	logger          *log.LoggerWrapper
	sdkJobStorage   sdkjob.Storage
	moduleDBStorage moduledb.Storage
	authz           readAccessChecker
}

// NewHandler constructs a Handler wired to the storages and authorisation
// service from the shared dependency bag.
func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:          deps.Logger,
		sdkJobStorage:   deps.SDKJobDB,
		moduleDBStorage: deps.ModuleDB,
		authz:           deps.Authorization,
	}
}

// ListSDKs returns all SDK generation jobs for the module identified by
// owner and module name, ordered newest first.  Returns NOT_FOUND if the
// module does not exist.  Returns PERMISSION_DENIED (via CheckReadAccess)
// if the module is private and the caller is not authorised to read it.
func (h *Handler) ListSDKs(ctx context.Context, in *connect.Request[registrypbv1.ListSDKsRequest]) (*connect.Response[registrypbv1.ListSDKsResponse], error) {
	// user may be nil for anonymous access; CheckReadAccess handles the nil case.
	user, _ := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)

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
		return nil, connErr.FromPgx(err)
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
