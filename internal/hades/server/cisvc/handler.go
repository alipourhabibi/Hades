// Package cisvc implements the CIService ConnectRPC handler.
// It exposes CI (lint + breaking-change) run results for schema module commits
// via:
//   - GetCIRun - returns the CI result for a given owner/module/commit triple.
//
// The handler enforces OPA read-access checks so that callers cannot retrieve
// CI results for private modules they are not authorised to read.
package cisvc

import (
	"context"

	"connectrpc.com/connect"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// readAccessChecker is the subset of the authorization Server used by Handler.
// Defined as an interface so that tests can substitute a fake.
type readAccessChecker interface {
	CheckReadAccess(ctx context.Context, user *registrypbv1.User, modules []*registrypbv1.Module) error
}

// Handler implements the CIService ConnectRPC handler.
type Handler struct {
	registryv1connect.CIServiceHandler

	logger          *log.LoggerWrapper
	ciRunStorage    cirun.Storage
	moduleDBStorage moduledb.Storage
	authz           readAccessChecker
}

// NewHandler constructs a Handler wired to the storages and authorisation
// service from the shared dependency bag.
func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:          deps.Logger,
		ciRunStorage:    deps.CIRunDB,
		moduleDBStorage: deps.ModuleDB,
		authz:           deps.Authorization,
	}
}

// GetCIRun returns the CI run result for the module commit identified by
// owner, module name, and commit hash.  Returns NOT_FOUND if the module does
// not exist or no CI run has been recorded for that commit.  Returns
// PERMISSION_DENIED (via CheckReadAccess) if the module is private and the
// caller is not authorised to read it.
func (h *Handler) GetCIRun(ctx context.Context, in *connect.Request[registrypbv1.GetCIRunRequest]) (*connect.Response[registrypbv1.GetCIRunResponse], error) {
	// user may be nil for anonymous access; CheckReadAccess handles the nil case.
	user, _ := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)

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
