// Package orgsvc implements the OrgService ConnectRPC handler.
// It exposes information about organizations (users whose type is
// USER_TYPE_ORGANIZATION) and their members
package orgsvc

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

// Handler implements the OrgService ConnectRPC handler.
type Handler struct {
	registryv1connect.OrgServiceHandler

	logger     *log.LoggerWrapper
	orgStorage *orgdb.OrgStorage
	userDB     *userdb.UserStorage
	moduleDB   *moduledb.ModuleStorage
}

// NewHandler constructs a Handler wired to the dependency bag.
func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:     deps.Logger,
		orgStorage: deps.OrgDB,
		userDB:     deps.UserDB,
		moduleDB:   deps.ModuleDB,
	}
}

// GetOrg returns the organization whose username matches name, including
// module and member counts.
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

// ListOrgMembers returns all members of the organization identified by
// OrgName, together with each member's role.
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

// ListOrganizations returns organizations whose username contains query.
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

// GetUserOrgs returns all organizations the given user belongs to.
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

// CreateOrg creates a new organization and makes the caller its first admin.
// Requires authentication.
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

	return &connect.Response[registrypbv1.CreateOrgResponse]{
		Msg: &registrypbv1.CreateOrgResponse{Org: org},
	}, nil
}

// UpdateOrg updates the description and url of an organization.
// Requires the caller to be an admin of the org.
func (h *Handler) UpdateOrg(ctx context.Context, in *connect.Request[registrypbv1.UpdateOrgRequest]) (*connect.Response[registrypbv1.UpdateOrgResponse], error) {
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

// AddOrgMember adds a user to the organization.
// Requires the caller to be an admin of the org.
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

	return &connect.Response[registrypbv1.AddOrgMemberResponse]{
		Msg: &registrypbv1.AddOrgMemberResponse{},
	}, nil
}

// RemoveOrgMember removes a user from the organization.
// Requires the caller to be an admin or the member themselves.
func (h *Handler) RemoveOrgMember(ctx context.Context, in *connect.Request[registrypbv1.RemoveOrgMemberRequest]) (*connect.Response[registrypbv1.RemoveOrgMemberResponse], error) {
	caller, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok || caller == nil {
		return nil, connErr.Unauthenticated("authentication required")
	}

	org, err := h.orgStorage.GetByName(ctx, in.Msg.OrgName)
	if err != nil {
		return nil, connErr.NotFound("organization not found")
	}

	// Allow if caller is an admin OR if they're removing themselves.
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

	return &connect.Response[registrypbv1.RemoveOrgMemberResponse]{
		Msg: &registrypbv1.RemoveOrgMemberResponse{},
	}, nil
}
