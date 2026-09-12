// Package identity consolidates UserService and OrgService handlers.
package identity

import (
	"context"
	"strings"
	"time"

	"connectrpc.com/connect"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/identity/v1/identityv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	orgdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/org"
	userdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/alipourhabibi/Hades/utils/connerr"
	"github.com/alipourhabibi/Hades/utils/log"
)

// orgAuthz is the subset of authorization.Server used by identity.Handler.
type orgAuthz interface {
	AddOrgOwner(ctx context.Context, subject, orgName string) error
	AddOrgMemberBinding(ctx context.Context, subject, role, orgName string) error
	DeleteOrgBinding(ctx context.Context, subject, orgName string) error
	Can(ctx context.Context, in *constants.Policy) (*constants.CanResponse, error)
}

// requireOrgPermission asks the authorization engine whether caller may perform
// action on the organisation.
//
// Organisation mutations used to authorise themselves by reading org_memberships
// directly, which left two sources of truth for the same question: the
// membership row and the OPA binding written alongside it. They are written in
// one transaction, so the binding is authoritative and is what every other
// mutation in the server is checked against.
//
// The engine error is returned, not discarded. RemoveOrgMember did
// `callerRole, _ :=`, which made a database failure indistinguishable from
// "the caller is not an admin": it happened to fail closed there, but that is
// a coin flip in general.
func (h *Handler) requireOrgPermission(ctx context.Context, caller *registrypbv1.User, orgName string, action constants.Action, procedure string) error {
	resp, err := h.authz.Can(ctx, &constants.Policy{
		Subject:      caller.Username,
		Domain:       orgName + "/*",
		ResourceType: string(constants.ResourceOrg),
		Action:       string(action),
	})
	if err != nil {
		h.logger.Error("authorization check failed", "error", err,
			"procedure", procedure, "org", orgName, "subject", caller.Username)
		return connerr.Unavailable("authorization service unavailable")
	}
	if !resp.Allowed {
		return connerr.PermissionDenied("permission denied on organization " + orgName)
	}
	return nil
}

// Handler implements both UserService and OrgService handlers.
//
// The Unimplemented embeds, rather than the bare handler interfaces, supply the
// one procedure this type does not define: UserService.CreateUser. A nil
// interface embed would panic on call instead of returning Unimplemented.
type Handler struct {
	registryv1connect.UnimplementedUserServiceHandler
	registryv1connect.UnimplementedOrgServiceHandler

	logger     *log.LoggerWrapper
	userDB     userdb.Storage
	orgStorage orgdb.Storage
	moduleDB   moduledb.Storage
	authz      orgAuthz
	uow        db.UnitOfWork
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:     deps.Logger,
		userDB:     deps.UserDB,
		orgStorage: deps.OrgDB,
		moduleDB:   deps.ModuleDB,
		authz:      deps.Authorization,
		uow:        deps.UoW,
	}
}

// redactEmails clears the email address on every record that is not the
// caller's own.
//
// An address is personal data that nothing in the registry needs in order to
// display a profile or a member list. Returning it made every account's address
// readable by any authenticated caller, and ListUsers handed over 50 at a time.
// The caller's own address stays, since the account settings page reads it.
//
// The records come fresh from the database on each request, so clearing the
// field in place affects only this response.
func redactEmails(caller *registrypbv1.User, users ...*registrypbv1.User) {
	callerID := ""
	if caller != nil {
		callerID = caller.Id
	}
	for _, u := range users {
		if u == nil {
			continue
		}
		if callerID != "" && u.Id == callerID {
			continue
		}
		u.Email = ""
	}
}

// callerFrom returns the authenticated user, or nil for an anonymous request.
func callerFrom(ctx context.Context) *registrypbv1.User {
	caller, _ := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	return caller
}

func (h *Handler) GetUser(ctx context.Context, in *connect.Request[registrypbv1.GetUserRequest]) (*connect.Response[registrypbv1.GetUserResponse], error) {
	user, err := h.userDB.GetByUsername(ctx, in.Msg.Username)
	if err != nil {
		h.logger.Warn("user not found", "procedure", "GetUser", "username", in.Msg.Username)
		return nil, connerr.NotFound("user not found")
	}

	if user.Type == registrypbv1.UserType_USER_TYPE_ORGANIZATION {
		return nil, connerr.NotFound("user not found")
	}

	moduleCount, err := h.moduleDB.CountByOwner(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to count modules", "error", err, "user_id", user.Id)
		return nil, connerr.Internal("failed to count modules")
	}

	orgs, err := h.orgStorage.GetUserOrgs(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to get user orgs", "error", err, "user_id", user.Id)
		return nil, connerr.Internal("failed to get user orgs")
	}
	if orgs == nil {
		orgs = []*registrypbv1.User{}
	}

	caller := callerFrom(ctx)
	redactEmails(caller, user)
	redactEmails(caller, orgs...)

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
		return nil, connerr.Unauthenticated("authentication required")
	}

	limit, offset := server.Page(in.Msg.PageSize, in.Msg.PageToken)
	users, err := h.userDB.List(ctx, in.Msg.Query, limit, offset)
	if err != nil {
		h.logger.Error("failed to list users", "error", err, "query", in.Msg.Query)
		return nil, connerr.InternalCause("failed to list users", err)
	}
	if users == nil {
		users = []*registrypbv1.User{}
	}
	redactEmails(callerFrom(ctx), users...)
	return &connect.Response[registrypbv1.ListUsersResponse]{
		Msg: &registrypbv1.ListUsersResponse{
			Users:         users,
			NextPageToken: server.NextPageToken(len(users), limit, offset),
		},
	}, nil
}

func (h *Handler) UpdateUser(ctx context.Context, in *connect.Request[registrypbv1.UpdateUserRequest]) (*connect.Response[registrypbv1.UpdateUserResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok || user == nil {
		return nil, connerr.Unauthenticated("authentication required")
	}

	updated, err := h.userDB.Update(ctx, user.Id, in.Msg.Description, in.Msg.Url)
	if err != nil {
		h.logger.Error("failed to update user", "error", err, "user_id", user.Id)
		return nil, connerr.Internal("failed to update user")
	}

	return &connect.Response[registrypbv1.UpdateUserResponse]{
		Msg: &registrypbv1.UpdateUserResponse{User: updated},
	}, nil
}

func (h *Handler) GetOrg(ctx context.Context, in *connect.Request[registrypbv1.GetOrgRequest]) (*connect.Response[registrypbv1.GetOrgResponse], error) {
	org, err := h.orgStorage.GetByName(ctx, in.Msg.Name)
	if err != nil {
		h.logger.Warn("organization not found", "error", err, "procedure", "GetOrg", "name", in.Msg.Name)
		return nil, connerr.NotFound("organization not found")
	}

	moduleCount, err := h.moduleDB.CountByOwner(ctx, org.Id)
	if err != nil {
		h.logger.Error("failed to count org modules", "error", err, "org_id", org.Id)
		return nil, connerr.Internal("failed to count modules")
	}

	memberCount, err := h.orgStorage.CountMembers(ctx, org.Id)
	if err != nil {
		h.logger.Error("failed to count org members", "error", err, "org_id", org.Id)
		return nil, connerr.Internal("failed to count members")
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
	// A credential is required, and the interceptor already enforces it: this
	// procedure is absent from both noAuthProcedures and optionalAuthProcedures.
	// The check is repeated here because the consequence of the interceptor
	// table and this handler drifting apart is anonymous enumeration of every
	// organisation's roster, and that is worth two lines.
	if callerFrom(ctx) == nil {
		return nil, connerr.Unauthenticated("authentication required")
	}

	org, err := h.orgStorage.GetByName(ctx, in.Msg.OrgName)
	if err != nil {
		h.logger.Warn("organization not found", "error", err, "procedure", "ListOrgMembers", "org_name", in.Msg.OrgName)
		return nil, connerr.NotFound("organization not found")
	}

	members, err := h.orgStorage.ListMembers(ctx, org.Id)
	if err != nil {
		h.logger.Error("failed to list org members", "error", err, "procedure", "ListOrgMembers", "org_id", org.Id)
		return nil, connerr.FromDB(err)
	}

	caller := callerFrom(ctx)
	pbMembers := make([]*registrypbv1.OrgMember, 0, len(members))
	for _, m := range members {
		redactEmails(caller, m.User)
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
	limit, offset := server.Page(in.Msg.PageSize, in.Msg.PageToken)
	orgs, err := h.orgStorage.List(ctx, in.Msg.Query, limit, offset)
	if err != nil {
		h.logger.Error("failed to list organizations", "error", err, "query", in.Msg.Query)
		return nil, connerr.Internal("failed to list organizations")
	}
	if orgs == nil {
		orgs = []*registrypbv1.User{}
	}
	redactEmails(callerFrom(ctx), orgs...)
	return &connect.Response[registrypbv1.ListOrganizationsResponse]{
		Msg: &registrypbv1.ListOrganizationsResponse{
			Organizations: orgs,
			NextPageToken: server.NextPageToken(len(orgs), limit, offset),
		},
	}, nil
}

func (h *Handler) GetUserOrgs(ctx context.Context, in *connect.Request[registrypbv1.GetUserOrgsRequest]) (*connect.Response[registrypbv1.GetUserOrgsResponse], error) {
	user, err := h.userDB.GetByUsername(ctx, in.Msg.Username)
	if err != nil {
		h.logger.Warn("user not found for GetUserOrgs", "error", err, "username", in.Msg.Username)
		return nil, connerr.NotFound("user not found")
	}

	orgs, err := h.orgStorage.GetUserOrgs(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to get user orgs", "error", err, "user_id", user.Id)
		return nil, connerr.Internal("failed to get user orgs")
	}
	if orgs == nil {
		orgs = []*registrypbv1.User{}
	}
	redactEmails(callerFrom(ctx), orgs...)
	return &connect.Response[registrypbv1.GetUserOrgsResponse]{
		Msg: &registrypbv1.GetUserOrgsResponse{Organizations: orgs},
	}, nil
}

func (h *Handler) CreateOrg(ctx context.Context, in *connect.Request[registrypbv1.CreateOrgRequest]) (*connect.Response[registrypbv1.CreateOrgResponse], error) {
	caller, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok || caller == nil {
		return nil, connerr.Unauthenticated("authentication required")
	}

	// Normalised and validated exactly as a username is.
	//
	// Organisations share the users table and one namespace with users, and an
	// org name becomes the first path segment of every module it owns. Register
	// lowercases and trims; this did neither, so "Acme" and "acme" were two
	// rows in a namespace whose own proto documentation calls unique, the
	// UNIQUE constraint on users.username is case-sensitive on both backends
	// and did not stop it, and an org's URLs were case-sensitive while a user's
	// were not.
	//
	// CreateOrgRequest.name bounds nothing but min_len 1, so the character set
	// was unchecked too: see the note in Register for what an unchecked first
	// path segment costs. ValidateName covers the reserved list, which is what
	// used to be tested here on its own.
	in.Msg.Name = strings.ToLower(strings.TrimSpace(in.Msg.Name))
	if err := constants.ValidateName(in.Msg.Name); err != nil {
		return nil, connerr.InvalidArgument("organization " + err.Error())
	}

	// The org row, the creator's admin membership (written by Create) and the
	// OPA owner binding are one unit. Written separately, a failure between them
	// leaves an org whose creator cannot administer it, or an OPA binding
	// pointing at an org that does not exist.
	result, err := h.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
		org, err := h.orgStorage.Create(txCtx, in.Msg.Name, in.Msg.Description, in.Msg.Url, caller.Id)
		if err != nil {
			h.logger.Error("failed to create org", "error", err, "name", in.Msg.Name)
			return nil, connerr.FromDB(err)
		}
		if err := h.authz.AddOrgOwner(txCtx, caller.Username, org.Username); err != nil {
			h.logger.Error("failed to add org owner binding", "error", err, "org", org.Username, "caller", caller.Username)
			return nil, connerr.Internal("failed to set org owner permissions")
		}
		return org, nil
	}, 15*time.Second)
	if err != nil {
		return nil, err
	}

	org, ok := result.(*registrypbv1.User)
	if !ok {
		h.logger.Error("unexpected result type from the create-org transaction", "procedure", "CreateOrg")
		return nil, connerr.Internal("failed to create organization")
	}
	return &connect.Response[registrypbv1.CreateOrgResponse]{
		Msg: &registrypbv1.CreateOrgResponse{Org: org},
	}, nil
}

func (h *Handler) UpdateOrg(ctx context.Context, in *connect.Request[registrypbv1.UpdateOrgRequest]) (*connect.Response[registrypbv1.UpdateOrgResponse], error) {
	caller, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok || caller == nil {
		return nil, connerr.Unauthenticated("authentication required")
	}

	org, err := h.orgStorage.GetByName(ctx, in.Msg.OrgName)
	if err != nil {
		return nil, connerr.NotFound("organization not found")
	}

	if err := h.requireOrgPermission(ctx, caller, org.Username, constants.ActionUpdate, "UpdateOrg"); err != nil {
		return nil, err
	}

	updated, err := h.orgStorage.Update(ctx, org.Id, in.Msg.Description, in.Msg.Url)
	if err != nil {
		h.logger.Error("failed to update org", "error", err, "org_id", org.Id)
		return nil, connerr.InternalCause("failed to update organization", err)
	}

	return &connect.Response[registrypbv1.UpdateOrgResponse]{
		Msg: &registrypbv1.UpdateOrgResponse{Org: updated},
	}, nil
}

func (h *Handler) AddOrgMember(ctx context.Context, in *connect.Request[registrypbv1.AddOrgMemberRequest]) (*connect.Response[registrypbv1.AddOrgMemberResponse], error) {
	caller, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok || caller == nil {
		return nil, connerr.Unauthenticated("authentication required")
	}

	org, err := h.orgStorage.GetByName(ctx, in.Msg.OrgName)
	if err != nil {
		return nil, connerr.NotFound("organization not found")
	}

	if err := h.requireOrgPermission(ctx, caller, org.Username, constants.ActionAdmin, "AddOrgMember"); err != nil {
		return nil, err
	}

	target, err := h.userDB.GetByUsername(ctx, in.Msg.Username)
	if err != nil {
		return nil, connerr.NotFound("user not found")
	}

	memberRole := in.Msg.Role
	if memberRole == "" {
		memberRole = "member"
	}
	// Map org "member" → OPA "contributor"; "admin" stays "admin".
	opaRole := memberRole
	if opaRole == "member" {
		opaRole = constants.RoleContributor
	}

	// Membership row and OPA binding are written together: the two stores
	// together are what grants access, and half of the pair is either a
	// member who cannot act or permissions with no membership record.
	if _, err := h.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
		if err := h.orgStorage.AddMember(txCtx, org.Id, target.Id, memberRole); err != nil {
			h.logger.Error("failed to add org member", "error", err, "org_id", org.Id, "member_id", target.Id)
			return nil, connerr.Internal("failed to add member")
		}
		if err := h.authz.AddOrgMemberBinding(txCtx, target.Username, opaRole, org.Username); err != nil {
			h.logger.Error("failed to add org member binding", "error", err, "org", org.Username, "member", target.Username)
			return nil, connerr.Internal("failed to set member permissions")
		}
		return nil, nil
	}, 15*time.Second); err != nil {
		return nil, err
	}

	return &connect.Response[registrypbv1.AddOrgMemberResponse]{
		Msg: &registrypbv1.AddOrgMemberResponse{},
	}, nil
}

func (h *Handler) RemoveOrgMember(ctx context.Context, in *connect.Request[registrypbv1.RemoveOrgMemberRequest]) (*connect.Response[registrypbv1.RemoveOrgMemberResponse], error) {
	caller, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok || caller == nil {
		return nil, connerr.Unauthenticated("authentication required")
	}

	org, err := h.orgStorage.GetByName(ctx, in.Msg.OrgName)
	if err != nil {
		return nil, connerr.NotFound("organization not found")
	}

	// Leaving an organisation needs no permission; removing somebody else does.
	if caller.Username != in.Msg.Username {
		if err := h.requireOrgPermission(ctx, caller, org.Username, constants.ActionAdmin, "RemoveOrgMember"); err != nil {
			return nil, err
		}
	}

	target, err := h.userDB.GetByUsername(ctx, in.Msg.Username)
	if err != nil {
		return nil, connerr.NotFound("user not found")
	}

	// Removing the last admin would leave the org permanently unmanageable:
	// UpdateOrg and AddOrgMember both require an existing admin, so nobody could
	// ever appoint a replacement. This also covers an admin removing themselves.
	targetRole, err := h.orgStorage.GetMemberRole(ctx, org.Id, target.Id)
	if err != nil {
		return nil, connerr.NotFound("user is not a member of this organization")
	}
	if targetRole == "admin" {
		members, err := h.orgStorage.ListMembers(ctx, org.Id)
		if err != nil {
			h.logger.Error("failed to list org members", "error", err, "procedure", "RemoveOrgMember", "org_id", org.Id)
			return nil, connerr.FromDB(err)
		}
		admins := 0
		for _, m := range members {
			if m.Role == "admin" {
				admins++
			}
		}
		if admins <= 1 {
			return nil, connerr.FailedPrecondition("cannot remove the last admin of an organization")
		}
	}

	// Removing the membership row without removing the binding would leave the
	// user with live permissions over the org, so the two go together.
	if _, err := h.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
		if err := h.orgStorage.RemoveMember(txCtx, org.Id, target.Id); err != nil {
			h.logger.Error("failed to remove org member", "error", err, "org_id", org.Id, "member_id", target.Id)
			return nil, connerr.Internal("failed to remove member")
		}
		if err := h.authz.DeleteOrgBinding(txCtx, target.Username, org.Username); err != nil {
			h.logger.Error("failed to delete org member binding", "error", err, "org", org.Username, "member", target.Username)
			return nil, connerr.Internal("failed to remove member permissions")
		}
		return nil, nil
	}, 15*time.Second); err != nil {
		return nil, err
	}

	return &connect.Response[registrypbv1.RemoveOrgMemberResponse]{
		Msg: &registrypbv1.RemoveOrgMemberResponse{},
	}, nil
}
