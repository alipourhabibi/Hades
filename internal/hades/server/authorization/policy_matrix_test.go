package authorization

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	registryv1alpha1connect "buf.build/gen/go/bufbuild/buf/connectrpc/go/buf/alpha/registry/v1alpha1/registryv1alpha1connect"
	"buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/auth/v1/authv1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/authorization/v1/authorizationv1connect"
	identityv1connect "github.com/alipourhabibi/Hades/api/gen/api/identity/v1/identityv1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
)

// This file is the authorization matrix: one row per mounted procedure, saying
// what credential it needs and what decides whether the caller may proceed.
//
// It exists because the two halves of authorization live apart. The
// interceptor decides *whether a credential is required and of what kind*, from
// the string maps in middleware.go. The handler decides *whether this caller
// may touch this resource*, by calling Can, BatchCan, or CheckReadAccess, or by
// checking ownership itself. Neither half can see the other, and nothing in the
// type system connects a procedure to either. The table below is the only place
// both halves are written down together, and the tests make a procedure that
// appears in neither a build failure rather than a hole.

// credential is what the interceptor requires before a handler runs.
type credential int

const (
	// credNone: reachable with no Authorization header, and no user is ever put
	// in the context.
	credNone credential = iota
	// credOptional: reachable anonymously, but a header that IS present must be
	// valid. The handler sees a possibly-nil user and must degrade to public
	// data.
	credOptional
	// credAny: a valid session token or personal API token.
	credAny
	// credSession: an interactive session token only. PATs are refused.
	credSession
	// credPAT: a personal API token only. Session tokens are refused.
	credPAT
)

// guard is what decides access once the caller is known.
type guard int

const (
	// guardOPA: the handler asks the policy engine, via Can or BatchCan for a
	// write, or CheckReadAccess for a read.
	guardOPA guard = iota
	// guardSelf: the procedure only ever touches rows belonging to the
	// authenticated caller, and scopes its query by their user id. No policy
	// question arises.
	guardSelf
	// guardOrgRole: the handler reads the caller's role from the org membership
	// table rather than from OPA. Org membership is not in the policy store, so
	// the table is the authoritative source for these.
	guardOrgRole
	// guardPublic: the data is public by definition and no check is made.
	guardPublic
	// guardNone: the procedure is open to any caller that got past the
	// interceptor, by deliberate decision.
	guardNone
	// guardUnimplemented: mounted but not implemented. Returns Unimplemented.
	// Listed so the row is a decision rather than an oversight, and so that
	// implementing it forces a matrix change.
	guardUnimplemented
)

type policyRow struct {
	procedure string
	cred      credential
	guard     guard
	// note explains anything a reader would otherwise have to go and check.
	note string
}

// matrix is the authorization decision for every procedure this server mounts.
func matrix() []policyRow {
	return []policyRow{
		// -- Authentication: the credential lifecycle itself. ----------------
		{authv1connect.AuthenticationServiceLoginProcedure, credNone, guardNone, "issues the credential; rate limited per IP"},
		{authv1connect.AuthenticationServiceRegisterProcedure, credNone, guardNone, "rate limited per IP; reserved names refused"},
		{authv1connect.AuthenticationServiceSigninProcedure, credNone, guardNone, "legacy alias for Register"},
		{authv1connect.AuthenticationServiceVerifyEmailProcedure, credNone, guardNone, "the single-use token in the request is the credential"},
		{authv1connect.AuthenticationServiceRequestPasswordResetProcedure, credNone, guardNone, "answers alike for known and unknown addresses"},
		{authv1connect.AuthenticationServiceResetPasswordProcedure, credNone, guardNone, "the single-use token in the request is the credential"},
		{authv1connect.AuthenticationServiceLogoutProcedure, credSession, guardSelf, "revokes only the caller's own session"},
		{authv1connect.AuthenticationServiceResendVerificationEmailProcedure, credAny, guardSelf, "sends to the address on the caller's own account"},
		{authv1connect.AuthenticationServiceChangePasswordProcedure, credSession, guardSelf, "re-checks the old password"},

		// -- Session, token, and second-factor management. -------------------
		// All session-only: a leaked PAT must not be able to mint a
		// replacement, evict the owner, or strip their second factor.
		{authv1connect.SessionServiceListSessionsProcedure, credSession, guardSelf, ""},
		{authv1connect.SessionServiceRevokeSessionProcedure, credSession, guardSelf, "ownership checked before revoke; NotFound otherwise"},
		{authv1connect.SessionServiceRevokeAllOtherSessionsProcedure, credSession, guardSelf, ""},
		{authv1connect.APITokenServiceCreateAPITokenProcedure, credSession, guardSelf, "scopes validated; capped per account"},
		{authv1connect.APITokenServiceListAPITokensProcedure, credSession, guardSelf, ""},
		{authv1connect.APITokenServiceRevokeAPITokenProcedure, credSession, guardSelf, "revoke is scoped by owner in SQL"},
		{authv1connect.TOTPServiceBeginEnrollTOTPProcedure, credSession, guardSelf, "re-checks the password"},
		{authv1connect.TOTPServiceConfirmEnrollTOTPProcedure, credSession, guardSelf, "attempt limited"},
		{authv1connect.TOTPServiceVerifyTOTPProcedure, credSession, guardSelf, "callable while the session is still TOTP-pending"},
		{authv1connect.TOTPServiceDisableTOTPProcedure, credSession, guardSelf, "re-checks the password"},
		{authv1connect.TOTPServiceListBackupCodesProcedure, credSession, guardSelf, "returns ids and usage only, never code values"},
		{authv1connect.TOTPServiceRegenerateBackupCodesProcedure, credSession, guardSelf, "re-checks the password"},
		{authv1connect.AuditServiceListAuditLogProcedure, credSession, guardSelf, "query scoped to the caller's user id"},

		// -- OAuth and device flows. -----------------------------------------
		{authv1connect.OAuthServiceGetOAuthURLProcedure, credNone, guardNone, "no session exists yet; state is recorded server-side"},
		{authv1connect.OAuthServiceOAuthCallbackProcedure, credNone, guardNone, "the provider code plus the single-use state are the credential"},
		{authv1connect.OAuthServiceListLinkedProvidersProcedure, credSession, guardSelf, ""},
		{authv1connect.OAuthServiceUnlinkProviderProcedure, credSession, guardSelf, "refuses to remove the last login method"},
		{authv1connect.DeviceServiceRequestDeviceCodeProcedure, credNone, guardNone, "no session exists yet"},
		{authv1connect.DeviceServicePollDeviceTokenProcedure, credNone, guardNone, "the device code is the credential; rate limited per IP"},
		{authv1connect.DeviceServiceApproveDeviceGrantProcedure, credSession, guardSelf, "issues a PAT, so a PAT must not drive it"},

		{authorizationv1connect.AuthorizationUserBySessionProcedure, credAny, guardSelf, "returns the caller's own record"},

		// -- Registry reads. All anonymous-capable, all per-module checked. ---
		{registryv1connect.ModuleServiceListModulesProcedure, credOptional, guardOPA, "SQL pre-filter narrows, OPA decides per row"},
		{registryv1connect.ModuleServiceGetModuleProcedure, credOptional, guardOPA, "denied reads answer NotFound"},
		{registryv1connect.CommitServiceListCommitsProcedure, credOptional, guardOPA, ""},
		{registryv1connect.CommitServiceGetCommitProcedure, credOptional, guardOPA, ""},
		{registryv1connect.CommitServiceGetCommitDiffProcedure, credOptional, guardOPA, ""},
		{registryv1connect.CommitServiceListModuleFilesProcedure, credOptional, guardOPA, "commit must belong to the named module"},
		{registryv1connect.CommitServiceGetFileContentProcedure, credOptional, guardOPA, "commit must belong to the named module"},
		{registryv1connect.CIServiceGetCIRunProcedure, credOptional, guardOPA, ""},
		{registryv1connect.SDKServiceListSDKsProcedure, credOptional, guardOPA, ""},

		// -- Registry writes. -------------------------------------------------
		{registryv1connect.ModuleServiceCreateModuleByNameProcedure, credAny, guardOPA, "module:create on owner/name"},
		{registryv1connect.ModuleServiceUpdateModuleProcedure, credAny, guardOPA, "module:update on owner/name"},

		// -- Identity. --------------------------------------------------------
		{identityv1connect.UserServiceGetUserProcedure, credOptional, guardPublic, "profile is public; email redacted unless it is the caller's own"},
		{identityv1connect.UserServiceListUsersProcedure, credOptional, guardNone, "handler rejects anonymous callers itself, to stop unauthenticated scraping"},
		{identityv1connect.UserServiceUpdateUserProcedure, credAny, guardSelf, "writes the caller's own row only"},
		{identityv1connect.UserServiceCreateUserProcedure, credAny, guardUnimplemented, "use AuthenticationService.Register"},
		{identityv1connect.OrgServiceGetOrgProcedure, credNone, guardPublic, ""},
		{identityv1connect.OrgServiceListOrgMembersProcedure, credNone, guardPublic, "emails redacted for an anonymous caller"},
		{identityv1connect.OrgServiceListOrganizationsProcedure, credOptional, guardPublic, ""},
		{identityv1connect.OrgServiceGetUserOrgsProcedure, credOptional, guardPublic, ""},
		{identityv1connect.OrgServiceCreateOrgProcedure, credAny, guardNone, "any authenticated user may create an org; reserved names refused"},
		{identityv1connect.OrgServiceUpdateOrgProcedure, credAny, guardOrgRole, "org admin only"},
		{identityv1connect.OrgServiceAddOrgMemberProcedure, credAny, guardOrgRole, "org admin only"},
		{identityv1connect.OrgServiceRemoveOrgMemberProcedure, credAny, guardOrgRole, "org admin, or the member removing themselves; last admin refused"},
		{identityv1connect.NotificationServiceListNotificationsProcedure, credAny, guardSelf, ""},
		{identityv1connect.NotificationServiceMarkNotificationReadProcedure, credAny, guardSelf, "update scoped by user id in SQL"},

		// -- buf.build protocol adapters. ------------------------------------
		// The reads stay anonymous-capable so `buf dep update` against a public
		// module works without credentials; each resolves modules through the
		// same handlers as the native reads, so CheckReadAccess still applies.
		{modulev1connect.ModuleServiceGetModulesProcedure, credOptional, guardOPA, ""},
		{modulev1connect.CommitServiceGetCommitsProcedure, credOptional, guardOPA, ""},
		{modulev1connect.GraphServiceGetGraphProcedure, credOptional, guardOPA, ""},
		{modulev1connect.DownloadServiceDownloadProcedure, credOptional, guardOPA, ""},
		{modulev1connect.UploadServiceUploadProcedure, credPAT, guardOPA, "module:push per module, batched; PAT-only so a stolen browser session cannot push"},
		{modulev1connect.ModuleServiceListModulesProcedure, credOptional, guardUnimplemented, ""},
		{modulev1connect.ModuleServiceCreateModulesProcedure, credAny, guardUnimplemented, "native CreateModuleByName is the supported route"},
		{modulev1connect.ModuleServiceUpdateModulesProcedure, credAny, guardUnimplemented, "native UpdateModule is the supported route"},
		{modulev1connect.ModuleServiceDeleteModulesProcedure, credAny, guardUnimplemented, "module deletion is not implemented anywhere"},
		{modulev1connect.CommitServiceListCommitsProcedure, credOptional, guardUnimplemented, ""},
		{registryv1alpha1connect.AuthnServiceGetCurrentUserProcedure, credPAT, guardSelf, "the buf CLI login handshake"},
		{registryv1alpha1connect.AuthnServiceGetCurrentUserSubjectProcedure, credAny, guardUnimplemented, ""},
	}
}

// TestMatrixCoversEveryMountedProcedure is the point of the file: a procedure
// that nobody classified is a procedure whose authorization nobody decided.
func TestMatrixCoversEveryMountedProcedure(t *testing.T) {
	classified := map[string]bool{}
	for _, row := range matrix() {
		assert.False(t, classified[row.procedure], "%q appears twice in the matrix", row.procedure)
		classified[row.procedure] = true
	}

	for procedure := range knownProcedures() {
		assert.True(t, classified[procedure],
			"%q is mounted but has no row in the authorization matrix", procedure)
	}
	for procedure := range classified {
		assert.True(t, knownProcedures()[procedure],
			"%q has a matrix row but is not a procedure of any mounted service", procedure)
	}
}

// TestMatrixAgreesWithTheInterceptorMaps checks the declared credential against
// what middleware.go will actually enforce, in both directions. A row saying
// "session only" that is missing from sessionOnlyProcedures is a documented
// intent the server does not implement.
func TestMatrixAgreesWithTheInterceptorMaps(t *testing.T) {
	for _, row := range matrix() {
		p := row.procedure

		assert.Equal(t, row.cred == credNone, noAuthProcedures[p],
			"noAuthProcedures disagrees with the matrix for %q", p)
		assert.Equal(t, row.cred == credOptional, optionalAuthProcedures[p],
			"optionalAuthProcedures disagrees with the matrix for %q", p)
		assert.Equal(t, row.cred == credSession, sessionOnlyProcedures[p],
			"sessionOnlyProcedures disagrees with the matrix for %q", p)
		assert.Equal(t, row.cred == credPAT, patOnlyProcedures[p],
			"patOnlyProcedures disagrees with the matrix for %q", p)
	}
}

// TestEveryWriteToARegistryResourceAsksThePolicyEngine pins the rule that
// separates this service from a plain CRUD API: anything that reads or writes a
// module must go through OPA, never through an ad-hoc ownership check.
func TestEveryWriteToARegistryResourceAsksThePolicyEngine(t *testing.T) {
	registryProcedures := []string{
		registryv1connect.ModuleServiceCreateModuleByNameProcedure,
		registryv1connect.ModuleServiceUpdateModuleProcedure,
		registryv1connect.ModuleServiceListModulesProcedure,
		registryv1connect.ModuleServiceGetModuleProcedure,
		registryv1connect.CommitServiceListCommitsProcedure,
		registryv1connect.CommitServiceGetCommitProcedure,
		registryv1connect.CommitServiceGetCommitDiffProcedure,
		registryv1connect.CommitServiceListModuleFilesProcedure,
		registryv1connect.CommitServiceGetFileContentProcedure,
		registryv1connect.CIServiceGetCIRunProcedure,
		registryv1connect.SDKServiceListSDKsProcedure,
		modulev1connect.ModuleServiceGetModulesProcedure,
		modulev1connect.CommitServiceGetCommitsProcedure,
		modulev1connect.GraphServiceGetGraphProcedure,
		modulev1connect.DownloadServiceDownloadProcedure,
		modulev1connect.UploadServiceUploadProcedure,
	}

	byProcedure := map[string]policyRow{}
	for _, row := range matrix() {
		byProcedure[row.procedure] = row
	}

	for _, p := range registryProcedures {
		row, ok := byProcedure[p]
		require.True(t, ok, "%q missing from the matrix", p)
		assert.Equal(t, guardOPA, row.guard,
			"%q touches a module and must be guarded by the policy engine", p)
	}
}

// TestNoProcedureIsBothAnonymousAndPolicyFree guards the combination that would
// expose data outright: reachable without a credential and with nothing
// deciding what the caller may see.
func TestNoProcedureIsBothAnonymousAndPolicyFree(t *testing.T) {
	for _, row := range matrix() {
		if row.cred != credNone && row.cred != credOptional {
			continue
		}
		switch row.guard {
		case guardOPA, guardPublic, guardUnimplemented:
			// Checked per resource, public by definition, or not reachable.
		case guardNone:
			// Permitted only where the handler is itself the gate, or the
			// procedure hands out no data at all. Each is named here so adding
			// another requires saying why.
			allowed := map[string]bool{
				authv1connect.AuthenticationServiceLoginProcedure:                true,
				authv1connect.AuthenticationServiceRegisterProcedure:             true,
				authv1connect.AuthenticationServiceSigninProcedure:               true,
				authv1connect.AuthenticationServiceVerifyEmailProcedure:          true,
				authv1connect.AuthenticationServiceRequestPasswordResetProcedure: true,
				authv1connect.AuthenticationServiceResetPasswordProcedure:        true,
				authv1connect.OAuthServiceGetOAuthURLProcedure:                   true,
				authv1connect.OAuthServiceOAuthCallbackProcedure:                 true,
				authv1connect.DeviceServiceRequestDeviceCodeProcedure:            true,
				authv1connect.DeviceServicePollDeviceTokenProcedure:              true,
				identityv1connect.UserServiceListUsersProcedure:                  true,
			}
			assert.True(t, allowed[row.procedure],
				"%q is reachable without a credential and has no guard", row.procedure)
		default:
			t.Errorf("%q is reachable without a credential but relies on %v, which assumes a known caller",
				row.procedure, row.guard)
		}
	}
}
