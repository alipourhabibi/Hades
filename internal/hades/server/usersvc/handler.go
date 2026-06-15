// Package usersvc implements the UserService ConnectRPC handler.
// It exposes user profile information and management
package usersvc

import (
	"context"

	"connectrpc.com/connect"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	orgdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/org"
	userdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// Handler implements the UserService ConnectRPC handler.
type Handler struct {
	registryv1connect.UserServiceHandler

	logger   *log.LoggerWrapper
	userDB   *userdb.UserStorage
	orgDB    *orgdb.OrgStorage
	moduleDB *moduledb.ModuleStorage
}

// NewHandler constructs a Handler wired to the dependency bag.
func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:   deps.Logger,
		userDB:   deps.UserDB,
		orgDB:    deps.OrgDB,
		moduleDB: deps.ModuleDB,
	}
}

// GetUser returns an enriched profile for the user with the given username.
// Returns NOT_FOUND if the username belongs to an organization (use GetOrg)
// or if it does not exist at all.
// TODO should not return the org data and say also this is org? or return error it is org? or some similar thing instead of not found
func (h *Handler) GetUser(ctx context.Context, in *connect.Request[registrypbv1.GetUserRequest]) (*connect.Response[registrypbv1.GetUserResponse], error) {
	user, err := h.userDB.GetByUsername(ctx, in.Msg.Username)
	if err != nil {
		h.logger.Warn("user not found", "procedure", "GetUser", "username", in.Msg.Username)
		return nil, connErr.NotFound("user not found")
	}

	// Orgs are looked up via OrgService/GetOrg.
	if user.Type == registrypbv1.UserType_USER_TYPE_ORGANIZATION {
		return nil, connErr.NotFound("user not found")
	}

	moduleCount, err := h.moduleDB.CountByOwner(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to count modules", "error", err, "user_id", user.Id)
		return nil, connErr.Internal("failed to count modules")
	}

	orgs, err := h.orgDB.GetUserOrgs(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to get user orgs", "error", err, "user_id", user.Id)
		return nil, connErr.Internal("failed to get user orgs")
	}
	if orgs == nil {
		orgs = []*registrypbv1.User{}
	}

	return &connect.Response[registrypbv1.GetUserResponse]{
		Msg: &registrypbv1.GetUserResponse{
			User:          user,
			ModuleCount:   moduleCount,
			Organizations: orgs,
		},
	}, nil
}

// ListUsers returns users (type=USER) whose username contains query.
func (h *Handler) ListUsers(ctx context.Context, in *connect.Request[registrypbv1.ListUsersRequest]) (*connect.Response[registrypbv1.ListUsersResponse], error) {
	users, err := h.userDB.List(ctx, in.Msg.Query)
	if err != nil {
		h.logger.Error("failed to list users", "error", err, "query", in.Msg.Query)
		return nil, connErr.Internal("failed to list users")
	}
	if users == nil {
		users = []*registrypbv1.User{}
	}
	return &connect.Response[registrypbv1.ListUsersResponse]{
		Msg: &registrypbv1.ListUsersResponse{Users: users},
	}, nil
}

// UpdateUser updates the description and url of the currently authenticated user.
func (h *Handler) UpdateUser(ctx context.Context, in *connect.Request[registrypbv1.UpdateUserRequest]) (*connect.Response[registrypbv1.UpdateUserResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok || user == nil {
		return nil, connErr.Unauthenticated("authentication required")
	}

	updated, err := h.userDB.Update(ctx, user.Id, in.Msg.Description, in.Msg.Url)
	if err != nil {
		h.logger.Error("failed to update user", "error", err, "user_id", user.Id)
		return nil, connErr.Internal("failed to update user")
	}

	return &connect.Response[registrypbv1.UpdateUserResponse]{
		Msg: &registrypbv1.UpdateUserResponse{User: updated},
	}, nil
}
