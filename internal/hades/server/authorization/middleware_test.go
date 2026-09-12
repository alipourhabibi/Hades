package authorization

import (
	"testing"

	"github.com/stretchr/testify/assert"

	registryv1alpha1connect "buf.build/gen/go/bufbuild/buf/connectrpc/go/buf/alpha/registry/v1alpha1/registryv1alpha1connect"
	"buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/auth/v1/authv1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/authorization/v1/authorizationv1connect"
	identityv1connect "github.com/alipourhabibi/Hades/api/gen/api/identity/v1/identityv1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
)

// knownProcedures lists every procedure constant generated for a service this
// server mounts.
//
// Every procedure of a mounted service belongs here, including the ones with no
// implementation behind them. Mounting a service exposes its whole method set:
// an unlisted procedure is still routable, it just has nobody deciding what
// credential it needs.
//
// The interceptor's policy maps are string literals with no compile-time link
// to these constants, so a renamed or newly added procedure silently falls
// through to the default branch. This list is what makes that a test failure
// rather than a production surprise.
func knownProcedures() map[string]bool {
	procedures := []string{
		// Auth domain.
		authv1connect.AuthenticationServiceLoginProcedure,
		authv1connect.AuthenticationServiceRegisterProcedure,
		authv1connect.AuthenticationServiceSigninProcedure,
		authv1connect.AuthenticationServiceLogoutProcedure,
		authv1connect.AuthenticationServiceVerifyEmailProcedure,
		authv1connect.AuthenticationServiceResendVerificationEmailProcedure,
		authv1connect.AuthenticationServiceRequestPasswordResetProcedure,
		authv1connect.AuthenticationServiceResetPasswordProcedure,
		authv1connect.AuthenticationServiceChangePasswordProcedure,

		authv1connect.SessionServiceListSessionsProcedure,
		authv1connect.SessionServiceRevokeSessionProcedure,
		authv1connect.SessionServiceRevokeAllOtherSessionsProcedure,

		authv1connect.OAuthServiceGetOAuthURLProcedure,
		authv1connect.OAuthServiceOAuthCallbackProcedure,
		authv1connect.OAuthServiceListLinkedProvidersProcedure,
		authv1connect.OAuthServiceUnlinkProviderProcedure,

		authv1connect.APITokenServiceCreateAPITokenProcedure,
		authv1connect.APITokenServiceListAPITokensProcedure,
		authv1connect.APITokenServiceRevokeAPITokenProcedure,

		authv1connect.DeviceServiceRequestDeviceCodeProcedure,
		authv1connect.DeviceServicePollDeviceTokenProcedure,
		authv1connect.DeviceServiceApproveDeviceGrantProcedure,

		authv1connect.TOTPServiceBeginEnrollTOTPProcedure,
		authv1connect.TOTPServiceConfirmEnrollTOTPProcedure,
		authv1connect.TOTPServiceVerifyTOTPProcedure,
		authv1connect.TOTPServiceDisableTOTPProcedure,
		authv1connect.TOTPServiceListBackupCodesProcedure,
		authv1connect.TOTPServiceRegenerateBackupCodesProcedure,

		authv1connect.AuditServiceListAuditLogProcedure,

		authorizationv1connect.AuthorizationUserBySessionProcedure,

		// Registry domain.
		registryv1connect.ModuleServiceListModulesProcedure,
		registryv1connect.ModuleServiceGetModuleProcedure,
		registryv1connect.ModuleServiceCreateModuleByNameProcedure,
		registryv1connect.ModuleServiceUpdateModuleProcedure,
		registryv1connect.CommitServiceListCommitsProcedure,
		registryv1connect.CommitServiceGetCommitProcedure,
		registryv1connect.CommitServiceGetCommitDiffProcedure,
		registryv1connect.CommitServiceListModuleFilesProcedure,
		registryv1connect.CommitServiceGetFileContentProcedure,
		registryv1connect.CIServiceGetCIRunProcedure,
		registryv1connect.SDKServiceListSDKsProcedure,

		// Identity domain.
		identityv1connect.UserServiceCreateUserProcedure,
		identityv1connect.UserServiceGetUserProcedure,
		identityv1connect.UserServiceListUsersProcedure,
		identityv1connect.UserServiceUpdateUserProcedure,
		identityv1connect.OrgServiceGetOrgProcedure,
		identityv1connect.OrgServiceListOrgMembersProcedure,
		identityv1connect.OrgServiceListOrganizationsProcedure,
		identityv1connect.OrgServiceGetUserOrgsProcedure,
		identityv1connect.OrgServiceCreateOrgProcedure,
		identityv1connect.OrgServiceUpdateOrgProcedure,
		identityv1connect.OrgServiceAddOrgMemberProcedure,
		identityv1connect.OrgServiceRemoveOrgMemberProcedure,
		identityv1connect.NotificationServiceListNotificationsProcedure,
		identityv1connect.NotificationServiceMarkNotificationReadProcedure,

		// buf.build protocol adapters.
		modulev1connect.ModuleServiceGetModulesProcedure,
		modulev1connect.ModuleServiceListModulesProcedure,
		modulev1connect.ModuleServiceCreateModulesProcedure,
		modulev1connect.ModuleServiceUpdateModulesProcedure,
		modulev1connect.ModuleServiceDeleteModulesProcedure,
		modulev1connect.CommitServiceGetCommitsProcedure,
		modulev1connect.CommitServiceListCommitsProcedure,
		modulev1connect.GraphServiceGetGraphProcedure,
		modulev1connect.DownloadServiceDownloadProcedure,
		modulev1connect.UploadServiceUploadProcedure,
		registryv1alpha1connect.AuthnServiceGetCurrentUserProcedure,
		registryv1alpha1connect.AuthnServiceGetCurrentUserSubjectProcedure,
	}

	out := make(map[string]bool, len(procedures))
	for _, p := range procedures {
		out[p] = true
	}
	return out
}

// policyMaps returns every procedure map the interceptor consults, by name.
func policyMaps() map[string]map[string]bool {
	return map[string]map[string]bool{
		"noAuthProcedures":       noAuthProcedures,
		"optionalAuthProcedures": optionalAuthProcedures,
		"totpPendingAllowed":     totpPendingAllowed,
		"emailUnverifiedAllowed": emailUnverifiedAllowed,
		"sessionOnlyProcedures":  sessionOnlyProcedures,
		"patOnlyProcedures":      patOnlyProcedures,
	}
}

func TestPolicyMapsOnlyReferenceRealProcedures(t *testing.T) {
	known := knownProcedures()
	for mapName, m := range policyMaps() {
		for procedure := range m {
			assert.True(t, known[procedure],
				"%s references %q, which is not a procedure of any mounted service; "+
					"a stale entry silently stops applying", mapName, procedure)
		}
	}
}

func TestNoAuthAndOptionalAuthAreDisjoint(t *testing.T) {
	// An entry in both is ambiguous: noAuthProcedures wins because it is checked
	// first, which would silently drop the optional-auth handling.
	for procedure := range noAuthProcedures {
		assert.False(t, optionalAuthProcedures[procedure],
			"%q is in both noAuthProcedures and optionalAuthProcedures", procedure)
	}
}

func TestSessionOnlyAndPATOnlyAreDisjoint(t *testing.T) {
	// A procedure in both is unreachable with any credential at all.
	for procedure := range sessionOnlyProcedures {
		assert.False(t, patOnlyProcedures[procedure],
			"%q is both session-only and PAT-only, so no credential can call it", procedure)
	}
}

func TestUnauthenticatedProceduresAreNotAlsoRestricted(t *testing.T) {
	// A procedure that needs no credential cannot meaningfully require a
	// particular credential type.
	for procedure := range noAuthProcedures {
		assert.False(t, sessionOnlyProcedures[procedure], "%q is no-auth but also session-only", procedure)
		assert.False(t, patOnlyProcedures[procedure], "%q is no-auth but also PAT-only", procedure)
	}
}

func TestCredentialMintingProceduresRequireASession(t *testing.T) {
	// These are the procedures that produce a new long-lived credential.
	// A PAT must not be able to drive any of them, or a leaked PAT can mint a
	// replacement that survives revocation of the original.
	minting := []string{
		authv1connect.APITokenServiceCreateAPITokenProcedure,
		authv1connect.DeviceServiceApproveDeviceGrantProcedure,
	}
	for _, procedure := range minting {
		assert.True(t, sessionOnlyProcedures[procedure],
			"%q issues a credential and must be session-only", procedure)
	}
}

func TestLogoutIsReachableWhileBlocked(t *testing.T) {
	// A session stuck at the TOTP prompt or with an unverified email must still
	// be able to end itself rather than lingering until it expires.
	logout := authv1connect.AuthenticationServiceLogoutProcedure
	assert.True(t, totpPendingAllowed[logout], "Logout must be callable while TOTP is pending")
	assert.True(t, emailUnverifiedAllowed[logout], "Logout must be callable before email verification")
}
