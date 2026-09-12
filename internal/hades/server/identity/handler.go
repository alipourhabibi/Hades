// Package identity consolidates UserService and OrgService handlers.
package identity

import (
	"context"

	"connectrpc.com/connect"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/identity/v1/identityv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	orgdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/org"
	userdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// orgAuthz is the subset of authorization.Server used by identity.Handler.
type orgAuthz interface {
	AddOrgOwner(ctx context.Context, subject, orgName string) error
	AddOrgMemberBinding(ctx context.Context, subject, role, orgName string) error
	DeleteOrgBinding(ctx context.Context, subject, orgName string) error
}

// Handler implements both UserService and OrgService handlers.
type Handler struct {
	registryv1connect.UserServiceHandler
	registryv1connect.OrgServiceHandler

	logger     *log.LoggerWrapper
	userDB     userdb.Storage
	orgStorage orgdb.Storage
	moduleDB   moduledb.Storage
	authz      orgAuthz
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:     deps.Logger,
		userDB:     deps.UserDB,
		orgStorage: deps.OrgDB,
		moduleDB:   deps.ModuleDB,
		authz:      deps.Authorization,
	}
}

func (h *Handler) GetUser(ctx context.Context, in *connect.Request[registrypbv1.GetUserRequest]) (*connect.Response[registrypbv1.GetUserResponse], error) {
	user, err := h.userDB.GetByUsername(ctx, in.Msg.Username)
	if err != nil {
		h.logger.Warn("user not found", "procedure", "GetUser", "username", in.Msg.Username)
		return nil, connErr.NotFound("user not found")
	}

	if user.Type == registrypbv1.UserType_USER_TYPE_ORGANIZATION {
		return nil, connErr.NotFound("user not found")
	}

	moduleCount, err := h.moduleDB.CountByOwner(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to count modules", "error", err, "user_id", user.Id)
		return nil, connErr.Internal("failed to count modules")
	}

	orgs, err := h.orgStorage.GetUserOrgs(ctx, user.Id)
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

func (h *Handler) ListUsers(ctx context.Context, in *connect.Request[registrypbv1.ListUsersRequest]) (*connect.Response[registrypbv1.ListUsersResponse], error) {
	// Any authenticated caller may enumerate users. This is intentional for an
	// internal registry where member discovery is expected. Anonymous callers
	// are rejected to prevent unauthenticated scraping.
	if _, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User); !ok {
		return nil, connErr.Unauthenticated("authentication required")
	}

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

func (h *Handler) GetOrg(ctx context.Context, in *connect.Request[registrypbv1.GetOrgRequest]) (*connect.Response[registrypbv1.GetOrgResponse], error) {
	org, err := h.orgStorage.GetByName(ctx, in.Msg.Name)
	if err != nil {
		h.logger.Warn("organization not found", "error", err, "procedure", "GetOrg", "name", in.Msg.Name)
		return nil, connErr.NotFound("organization not found")
	}

	moduleCount, err := h.moduleDB.CountByOwner(ctx, org.Id)
	if err != nil {
		h.logger.Error("failed to count org modules", "error", err, "org_id", org.Id)
		return nil, connErr.Internal("failed to count modules")
	}

	memberCount, err := h.orgStorage.CountMembers(ctx, org.Id)
	if err != nil {
		h.logger.Error("failed to count org members", "error", err, "org_id", org.Id)
		return nil, connErr.Internal("failed to count members")
	}

	return &connect.Response[registrypbv1.GetOrgResponse]{
		Msg: &registrypbv1.GetOrgResponse{
			Org:         org,
			ModuleCount: moduleCount,
			MemberCount: memberCount,
		},
	}, nil
}

func (h *Handler) ListOrgMembers(ctx context.Context, in *connect.Request[registrypbv1.ListOrgMembersRequest]) (*connect.Response[registrypbv1.ListOrgMembersResponse], error) {
	org, err := h.orgStorage.GetByName(ctx, in.Msg.OrgName)
	if err != nil {
		h.logger.Warn("organization not found", "error", err, "procedure", "ListOrgMembers", "org_name", in.Msg.OrgName)
		return nil, connErr.NotFound("organization not found")
	}

	members, err := h.orgStorage.ListMembers(ctx, org.Id)
	if err != nil {
		h.logger.Error("failed to list org members", "error", err, "procedure", "ListOrgMembers", "org_id", org.Id)
		return nil, connErr.FromPgx(err)
	}

	pbMembers := make([]*registrypbv1.OrgMember, 0, len(members))
	for _, m := range members {
		pbMembers = append(pbMembers, &registrypbv1.OrgMember{
			User: m.User,
			Role: m.Role,
		})
	}

	return &connect.Response[registrypbv1.ListOrgMembersResponse]{
		Msg: &registrypbv1.ListOrgMembersResponse{Members: pbMembers},
	}, nil
}

func (h *Handler) ListOrganizations(ctx context.Context, in *connect.Request[registrypbv1.ListOrganizationsRequest]) (*connect.Response[registrypbv1.ListOrganizationsResponse], error) {
	orgs, err := h.orgStorage.List(ctx, in.Msg.Query)
	if err != nil {
		h.logger.Error("failed to list organizations", "error", err, "query", in.Msg.Query)
		return nil, connErr.Internal("failed to list organizations")
	}
	if orgs == nil {
		orgs = []*registrypbv1.User{}
	}
	return &connect.Response[registrypbv1.ListOrganizationsResponse]{
		Msg: &registrypbv1.ListOrganizationsResponse{Organizations: orgs},
	}, nil
}

func (h *Handler) GetUserOrgs(ctx context.Context, in *connect.Request[registrypbv1.GetUserOrgsRequest]) (*connect.Response[registrypbv1.GetUserOrgsResponse], error) {
	user, err := h.userDB.GetByUsername(ctx, in.Msg.Username)
	if err != nil {
		h.logger.Warn("user not found for GetUserOrgs", "error", err, "username", in.Msg.Username)
		return nil, connErr.NotFound("user not found")
	}

	orgs, err := h.orgStorage.GetUserOrgs(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to get user orgs", "error", err, "user_id", user.Id)
		return nil, connErr.Internal("failed to get user orgs")
	}
	if orgs == nil {
		orgs = []*registrypbv1.User{}
	}
	return &connect.Response[registrypbv1.GetUserOrgsResponse]{
		Msg: &registrypbv1.GetUserOrgsResponse{Organizations: orgs},
	}, nil
}

func (h *Handler) CreateOrg(ctx context.Context, in *connect.Request[registrypbv1.CreateOrgRequest]) (*connect.Response[registrypbv1.CreateOrgResponse], error) {
	caller, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok || caller == nil {
		return nil, connErr.Unauthenticated("authentication required")
	}

	org, err := h.orgStorage.Create(ctx, in.Msg.Name, in.Msg.Description, in.Msg.Url, caller.Id)
	if err != nil {
		h.logger.Error("failed to create org", "error", err, "name", in.Msg.Name)
		return nil, connErr.FromPgx(err)
	}

	if err := h.authz.AddOrgOwner(ctx, caller.Username, org.Username); err != nil {
		h.logger.Error("failed to add org owner binding", "error", err, "org", org.Username, "caller", caller.Username)
		return nil, connErr.Internal("failed to set org owner permissions")
	}

	return &connect.Response[registrypbv1.CreateOrgResponse]{
		Msg: &registrypbv1.CreateOrgResponse{Org: org},
	}, nil
}

func (h *Handler) UpdateOrg(ctx context.Context, in *connect.Request[registrypbv1.UpdateOrgRequest]) (*connect.Response[registrypbv1.UpdateOrgResponse], error) {
	// Org mutation handlers (UpdateOrg, AddOrgMember, RemoveOrgMember) use a direct
	// DB role lookup instead of OPA. Org membership is stored in the DB, not in the
	// OPA policy store, so the DB check is the authoritative source for these operations.
	caller, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok || caller == nil {
		return nil, connErr.Unauthenticated("authentication required")
	}

	org, err := h.orgStorage.GetByName(ctx, in.Msg.OrgName)
	if err != nil {
		return nil, connErr.NotFound("organization not found")
	}

	role, err := h.orgStorage.GetMemberRole(ctx, org.Id, caller.Id)
	if err != nil || role != "admin" {
		return nil, connErr.PermissionDenied("only org admins can update the organization")
	}

	updated, err := h.orgStorage.Update(ctx, org.Id, in.Msg.Description, in.Msg.Url)
	if err != nil {
		h.logger.Error("failed to update org", "error", err, "org_id", org.Id)
		return nil, connErr.Internal("failed to update organization")
	}

	return &connect.Response[registrypbv1.UpdateOrgResponse]{
		Msg: &registrypbv1.UpdateOrgResponse{Org: updated},
	}, nil
}

func (h *Handler) AddOrgMember(ctx context.Context, in *connect.Request[registrypbv1.AddOrgMemberRequest]) (*connect.Response[registrypbv1.AddOrgMemberResponse], error) {
	caller, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok || caller == nil {
		return nil, connErr.Unauthenticated("authentication required")
	}

	org, err := h.orgStorage.GetByName(ctx, in.Msg.OrgName)
	if err != nil {
		return nil, connErr.NotFound("organization not found")
	}

	role, err := h.orgStorage.GetMemberRole(ctx, org.Id, caller.Id)
	if err != nil || role != "admin" {
		return nil, connErr.PermissionDenied("only org admins can add members")
	}

	target, err := h.userDB.GetByUsername(ctx, in.Msg.Username)
	if err != nil {
		return nil, connErr.NotFound("user not found")
	}

	memberRole := in.Msg.Role
	if memberRole == "" {
		memberRole = "member"
	}
	if err := h.orgStorage.AddMember(ctx, org.Id, target.Id, memberRole); err != nil {
		h.logger.Error("failed to add org member", "error", err, "org_id", org.Id, "member_id", target.Id)
		return nil, connErr.Internal("failed to add member")
	}

	// Map org "member" → OPA "contributor"; "admin" stays "admin".
	opaRole := memberRole
	if opaRole == "member" {
		opaRole = constants.RoleContributor
	}
	if err := h.authz.AddOrgMemberBinding(ctx, target.Username, opaRole, org.Username); err != nil {
		h.logger.Error("failed to add org member binding", "error", err, "org", org.Username, "member", target.Username)
		return nil, connErr.Internal("failed to set member permissions")
	}

	return &connect.Response[registrypbv1.AddOrgMemberResponse]{
		Msg: &registrypbv1.AddOrgMemberResponse{},
	}, nil
}

func (h *Handler) RemoveOrgMember(ctx context.Context, in *connect.Request[registrypbv1.RemoveOrgMemberRequest]) (*connect.Response[registrypbv1.RemoveOrgMemberResponse], error) {
	caller, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok || caller == nil {
		return nil, connErr.Unauthenticated("authentication required")
	}

	org, err := h.orgStorage.GetByName(ctx, in.Msg.OrgName)
	if err != nil {
		return nil, connErr.NotFound("organization not found")
	}

	callerRole, _ := h.orgStorage.GetMemberRole(ctx, org.Id, caller.Id)
	isSelf := caller.Username == in.Msg.Username
	if callerRole != "admin" && !isSelf {
		return nil, connErr.PermissionDenied("only org admins can remove other members")
	}

	target, err := h.userDB.GetByUsername(ctx, in.Msg.Username)
	if err != nil {
		return nil, connErr.NotFound("user not found")
	}

	if err := h.orgStorage.RemoveMember(ctx, org.Id, target.Id); err != nil {
		h.logger.Error("failed to remove org member", "error", err, "org_id", org.Id, "member_id", target.Id)
		return nil, connErr.Internal("failed to remove member")
	}

	if err := h.authz.DeleteOrgBinding(ctx, target.Username, org.Username); err != nil {
		h.logger.Error("failed to delete org member binding", "error", err, "org", org.Username, "member", target.Username)
		return nil, connErr.Internal("failed to remove member permissions")
	}

	return &connect.Response[registrypbv1.RemoveOrgMemberResponse]{
		Msg: &registrypbv1.RemoveOrgMemberResponse{},
	}, nil
}
