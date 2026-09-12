package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/utils/connerr"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
)

// oauthStateTTL bounds how long an issued OAuth state value stays valid.
const oauthStateTTL = 10 * time.Minute

// oauthStateKey is the cache key under which an issued state is recorded.
func oauthStateKey(state string) string { return "oauth:state:" + state }

func (s *Server) oauth2Config(provider v1.OAuthProvider) (*oauth2.Config, error) {
	switch provider {
	case v1.OAuthProvider_OAUTH_PROVIDER_GITHUB:
		return &oauth2.Config{
			ClientID:     s.oauthCfg.GitHub.ClientID,
			ClientSecret: s.oauthCfg.GitHub.ClientSecret,
			RedirectURL:  s.oauthCfg.GitHub.RedirectURL,
			Endpoint:     github.Endpoint,
			Scopes:       []string{"user:email"},
		}, nil
	case v1.OAuthProvider_OAUTH_PROVIDER_GOOGLE:
		return &oauth2.Config{
			ClientID:     s.oauthCfg.Google.ClientID,
			ClientSecret: s.oauthCfg.Google.ClientSecret,
			RedirectURL:  s.oauthCfg.Google.RedirectURL,
			Endpoint:     google.Endpoint,
			Scopes:       []string{"openid", "email", "profile"},
		}, nil
	default:
		return nil, connerr.InvalidArgument("unsupported OAuth provider")
	}
}

func providerName(p v1.OAuthProvider) string {
	switch p {
	case v1.OAuthProvider_OAUTH_PROVIDER_GITHUB:
		return "github"
	case v1.OAuthProvider_OAUTH_PROVIDER_GOOGLE:
		return "google"
	}
	return "unknown"
}

// newOAuthState returns a fresh 256-bit state value.
func newOAuthState() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// GetOAuthURL returns the provider authorization URL and records the state
// value so OAuthCallback can verify it.
//
// The state is generated **here**, not by the caller. A client-supplied state
// makes CSRF resistance depend entirely on the client remembering to compare
// its own copy, and a client that skips that step has none; it also lets an
// anonymous caller choose the cache key it writes. The issued value is carried
// in the returned URL's `state` query parameter, so a client that does want to
// pin it can read it from there.
//
// Recording it server-side adds three guarantees: the state must have been
// issued through this endpoint, it is single-use (a replayed callback is
// rejected), and it is bound to one provider and expires after oauthStateTTL.
func (s *Server) GetOAuthURL(ctx context.Context, in *connect.Request[v1.GetOAuthURLRequest]) (*connect.Response[v1.GetOAuthURLResponse], error) {
	cfg, err := s.oauth2Config(in.Msg.Provider)
	if err != nil {
		return nil, err
	}
	if s.cache == nil {
		s.logger.Error("OAuth state cache not configured", "procedure", "GetOAuthURL")
		return nil, connerr.Internal("OAuth is not available")
	}

	// Unauthenticated and it writes a cache entry per call, so it needs a
	// bound. Without one this endpoint is a memory-growth vector.
	if err := s.enforceLimit(ctx, fmt.Sprintf("oauthurl:ip:%s", extractClientIP(in, s.trustedProxies)), 10, time.Minute, "GetOAuthURL"); err != nil {
		return nil, err
	}

	state, err := newOAuthState()
	if err != nil {
		s.logger.Error("failed to generate OAuth state", "error", err, "procedure", "GetOAuthURL")
		return nil, connerr.Internal("failed to start OAuth flow")
	}
	if err := s.cache.Set(ctx, oauthStateKey(state), providerName(in.Msg.Provider), oauthStateTTL); err != nil {
		s.logger.Error("failed to record OAuth state", "error", err, "procedure", "GetOAuthURL")
		return nil, connerr.Internal("failed to start OAuth flow")
	}
	url := cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
	return &connect.Response[v1.GetOAuthURLResponse]{
		Msg: &v1.GetOAuthURLResponse{Url: url},
	}, nil
}

// consumeOAuthState verifies that state was issued by GetOAuthURL for this
// provider and removes it so it cannot be replayed.
func (s *Server) consumeOAuthState(ctx context.Context, state string, provider v1.OAuthProvider) error {
	if s.cache == nil {
		return connerr.Internal("OAuth is not available")
	}
	recorded, ok, err := s.cache.Get(ctx, oauthStateKey(state))
	if err != nil {
		// A cache outage must not be reported as an invalid state: that sends
		// every user round the flow again to fail again. Say the service is
		// unavailable, which is what is true.
		s.logger.Error("OAuth state lookup failed", "error", err, "procedure", "OAuthCallback")
		return connerr.Unavailable("OAuth is temporarily unavailable")
	}
	if !ok {
		return connerr.PermissionDenied("invalid or expired OAuth state")
	}
	// Delete before any further work so a concurrent replay loses the race.
	if err := s.cache.Delete(ctx, oauthStateKey(state)); err != nil {
		s.logger.Error("failed to clear OAuth state", "error", err, "procedure", "OAuthCallback")
	}
	if recorded != providerName(provider) {
		return connerr.PermissionDenied("OAuth state does not match provider")
	}
	return nil
}

func (s *Server) OAuthCallback(ctx context.Context, in *connect.Request[v1.OAuthCallbackRequest]) (*connect.Response[v1.OAuthCallbackResponse], error) {
	cfg, err := s.oauth2Config(in.Msg.Provider)
	if err != nil {
		return nil, err
	}

	if err := s.consumeOAuthState(ctx, in.Msg.State, in.Msg.Provider); err != nil {
		s.logger.Warn("OAuth state rejected", "procedure", "OAuthCallback", "provider", providerName(in.Msg.Provider))
		return nil, err
	}

	oauthToken, err := cfg.Exchange(ctx, in.Msg.Code)
	if err != nil {
		s.logger.Warn("OAuth token exchange failed", "error", err, "procedure", "OAuthCallback")
		return nil, connerr.Unauthenticated("OAuth token exchange failed")
	}

	providerUID, emailAddr, err := s.fetchProviderProfile(ctx, in.Msg.Provider, oauthToken)
	if err != nil {
		return nil, err
	}
	// fetchProviderProfile only returns an email the provider states it has
	// verified. Both values are required: an empty uid or email would otherwise
	// match unrelated rows (organisation rows carry an empty email).
	if providerUID == "" || emailAddr == "" {
		s.logger.Warn("OAuth profile missing verified identity", "procedure", "OAuthCallback", "provider", providerName(in.Msg.Provider))
		return nil, connerr.Unauthenticated("provider did not supply a verified email address")
	}

	pName := providerName(in.Msg.Provider)

	userID, err := s.resolveOAuthUser(ctx, pName, providerUID, emailAddr)
	if err != nil {
		return nil, err
	}

	fullToken, tokenHash, err := utilscrypto.GenerateToken(utilscrypto.SessionTokenPrefix)
	if err != nil {
		s.logger.Error("failed to generate session token", "error", err, "procedure", "OAuthCallback")
		return nil, connerr.Internal("failed to generate session token")
	}
	idleDays := s.authCfg.Session.IdleTimeoutDays
	if idleDays == 0 {
		idleDays = 7
	}
	absDays := s.authCfg.Session.AbsoluteTimeoutDays
	if absDays == 0 {
		absDays = 14
	}
	ip := extractClientIP(in, s.trustedProxies)
	ua := in.Header().Get("User-Agent")
	_, err = s.sessionStorage.CreateWithToken(ctx, userID, "oauth:"+pName, tokenHash, ip, ua, time.Now().Add(time.Duration(idleDays)*24*time.Hour), time.Now().Add(time.Duration(absDays)*24*time.Hour))
	if err != nil {
		s.logger.Error("failed to create session", "error", err, "procedure", "OAuthCallback", "user_id", userID)
		return nil, connerr.FromDB(err)
	}

	return &connect.Response[v1.OAuthCallbackResponse]{
		Msg: &v1.OAuthCallbackResponse{
			Login: &v1.LoginResponse{
				Token:       fullToken,
				PendingTotp: s.totpRequired(ctx, userID),
			},
		},
	}, nil
}

// resolveOAuthUser maps a verified provider identity to a Hades user id,
// creating the account on first login. Account creation, email verification and
// the initial role binding happen in one transaction so a partial account can
// never survive.
func (s *Server) resolveOAuthUser(ctx context.Context, pName, providerUID, emailAddr string) (string, error) {
	if identity, err := s.oauthIdentityDB.GetByProviderUID(ctx, pName, providerUID); err == nil {
		return identity.UserID, nil
	}

	// No identity link yet. Taking over an existing local account on nothing
	// more than a provider-asserted email is a full account takeover if the
	// provider account is compromised or the provider mis-reports
	// verification, so it is off unless an operator has explicitly said the
	// identity provider is in the same trust domain.
	if existing, err := s.userStorage.GetByEmail(ctx, emailAddr); err == nil {
		if !s.oauthCfg.AllowAccountLinkingByEmail {
			s.logger.Warn("refused OAuth claim of an existing account by email",
				"procedure", "OAuthCallback", "provider", pName, "user_id", existing.Id)
			if s.auditLogDB != nil {
				_ = s.auditLogDB.Create(ctx, &existing.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_FAILED, "", "",
					map[string]any{"provider": pName, "reason": "oauth_email_claim_refused"})
			}
			return "", connerr.FailedPrecondition(
				"an account already uses this email address; sign in with your password and link " + pName + " from account settings")
		}
		if err := s.oauthIdentityDB.Create(ctx, existing.Id, pName, providerUID, emailAddr); err != nil {
			s.logger.Error("failed to link OAuth identity", "error", err, "procedure", "OAuthCallback", "user_id", existing.Id)
			return "", connerr.FromDB(err)
		}
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &existing.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_OAUTH_LINKED, "", "",
				map[string]any{"provider": pName, "claimed_by_email": true})
		}
		return existing.Id, nil
	}

	username, err := s.generateOAuthUsername(ctx, pName, providerUID)
	if err != nil {
		return "", err
	}
	result, err := s.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
		if err := s.userStorage.Create(txCtx, username, emailAddr, "", identityv1.UserType_USER_TYPE_USER, identityv1.UserState_USER_STATE_ACTIVE, "", ""); err != nil {
			return nil, connerr.FromDB(err)
		}
		newUser, err := s.userStorage.GetByUsername(txCtx, username)
		if err != nil {
			return nil, connerr.FromDB(err)
		}
		if err := s.userStorage.SetEmailVerified(txCtx, newUser.Id); err != nil {
			return nil, connerr.FromDB(err)
		}
		if err := s.authorizationService.AddBasicRoles(txCtx, username); err != nil {
			return nil, err
		}
		if err := s.oauthIdentityDB.Create(txCtx, newUser.Id, pName, providerUID, emailAddr); err != nil {
			return nil, connerr.FromDB(err)
		}
		return newUser.Id, nil
	}, 15*time.Second)
	if err != nil {
		s.logger.Error("failed to create OAuth user", "error", err, "procedure", "OAuthCallback", "provider", pName)
		return "", err
	}

	userID, ok := result.(string)
	if !ok {
		s.logger.Error("unexpected result type from the create-user transaction",
			"procedure", "OAuthCallback", "provider", pName)
		return "", connerr.Internal("failed to create account")
	}
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &userID, v1.AuditEventType_AUDIT_EVENT_TYPE_OAUTH_LINKED, "", "", map[string]any{"provider": pName})
	}
	return userID, nil
}

// generateOAuthUsername derives a free, non-reserved username for a new OAuth
// account.
//
// The naive "<provider>_<uid>" form skips the reserved-name check that
// Register applies and has no collision recovery, so a clash surfaces as an
// opaque unique-violation from inside a transaction. Both are handled here.
func (s *Server) generateOAuthUsername(ctx context.Context, pName, providerUID string) (string, error) {
	base := strings.ToLower(fmt.Sprintf("%s_%s", pName, providerUID))
	for attempt := 0; attempt < 10; attempt++ {
		candidate := base
		if attempt > 0 {
			candidate = fmt.Sprintf("%s_%d", base, attempt)
		}
		if constants.IsReservedName(candidate) {
			continue
		}
		_, err := s.userStorage.GetByUsername(ctx, candidate)
		switch {
		case err == nil:
			continue // taken
		case errors.Is(err, connerr.ErrNotFound), errors.Is(err, sql.ErrNoRows), errors.Is(err, pgx.ErrNoRows):
			return candidate, nil // free
		default:
			s.logger.Error("username lookup failed while deriving an OAuth username",
				"error", err, "procedure", "OAuthCallback", "candidate", candidate)
			return "", connerr.FromDB(err)
		}
	}
	s.logger.Error("could not derive a free username for an OAuth account",
		"procedure", "OAuthCallback", "provider", pName)
	return "", connerr.Internal("failed to create account")
}

func (s *Server) fetchProviderProfile(ctx context.Context, provider v1.OAuthProvider, token *oauth2.Token) (uid, email string, err error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	client.Timeout = 10 * time.Second
	switch provider {
	case v1.OAuthProvider_OAUTH_PROVIDER_GITHUB:
		return fetchGitHubProfile(client)
	case v1.OAuthProvider_OAUTH_PROVIDER_GOOGLE:
		return fetchGoogleProfile(client)
	}
	return "", "", connerr.InvalidArgument("unsupported provider")
}

// getJSON performs a GET and decodes a 2xx JSON body into out. A non-2xx
// status is an error: without this check an error body decodes into a
// zero-value struct and is indistinguishable from a real profile.
func getJSON(client *http.Client, url string, out any) error {
	resp, err := client.Get(url)
	if err != nil {
		return connerr.Unauthenticated("provider profile fetch failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return connerr.Unauthenticated("provider profile fetch failed")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return connerr.Unauthenticated("provider profile fetch failed")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return connerr.Unauthenticated("provider profile parse failed")
	}
	return nil
}

type gitHubUser struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
	Login string `json:"login"`
}

type gitHubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// fetchGitHubProfile returns the GitHub account id and its primary verified
// email address.
//
// The email is read from /user/emails, never from /user. The email on /user is
// the public profile field, which any account can set to an address it does not
// own; trusting it would let an attacker claim another user's Hades account by
// email match.
func fetchGitHubProfile(client *http.Client) (uid, email string, err error) {
	var u gitHubUser
	if err := getJSON(client, "https://api.github.com/user", &u); err != nil {
		return "", "", err
	}
	if u.ID == 0 {
		return "", "", connerr.Unauthenticated("GitHub profile is missing an account id")
	}

	var emails []gitHubEmail
	if err := getJSON(client, "https://api.github.com/user/emails", &emails); err != nil {
		return "", "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified && e.Email != "" {
			return fmt.Sprintf("%d", u.ID), e.Email, nil
		}
	}
	return "", "", connerr.Unauthenticated("GitHub account has no primary verified email address")
}

type googleUser struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

// fetchGoogleProfile returns the Google subject id and email, requiring that
// Google states the address is verified.
func fetchGoogleProfile(client *http.Client) (uid, email string, err error) {
	var u googleUser
	if err := getJSON(client, "https://openidconnect.googleapis.com/v1/userinfo", &u); err != nil {
		return "", "", err
	}
	if u.Sub == "" {
		return "", "", connerr.Unauthenticated("Google profile is missing a subject id")
	}
	if !u.EmailVerified || u.Email == "" {
		return "", "", connerr.Unauthenticated("Google account has no verified email address")
	}
	return u.Sub, u.Email, nil
}

func (s *Server) ListLinkedProviders(ctx context.Context, in *connect.Request[v1.ListLinkedProvidersRequest]) (*connect.Response[v1.ListLinkedProvidersResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "ListLinkedProviders")
		return nil, connerr.Unauthenticated("not authenticated")
	}

	rows, err := s.oauthIdentityDB.GetByUserID(ctx, user.Id)
	if err != nil {
		s.logger.Error("failed to get linked providers", "error", err, "procedure", "ListLinkedProviders", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}

	providers := make([]*v1.LinkedProvider, 0, len(rows))
	for _, row := range rows {
		p := &v1.LinkedProvider{
			LinkedAt: timestamppb.New(row.CreatedAt),
		}
		switch row.Provider {
		case "github":
			p.Provider = v1.OAuthProvider_OAUTH_PROVIDER_GITHUB
		case "google":
			p.Provider = v1.OAuthProvider_OAUTH_PROVIDER_GOOGLE
		}
		providers = append(providers, p)
	}
	return &connect.Response[v1.ListLinkedProvidersResponse]{
		Msg: &v1.ListLinkedProvidersResponse{Providers: providers},
	}, nil
}

func (s *Server) UnlinkProvider(ctx context.Context, in *connect.Request[v1.UnlinkProviderRequest]) (*connect.Response[v1.UnlinkProviderResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "UnlinkProvider")
		return nil, connerr.Unauthenticated("not authenticated")
	}

	pName := providerName(in.Msg.Provider)

	// Unlinking the last provider on an account with no password would leave it
	// with no way to log in, so it is refused (see the RPC contract).
	af, err := s.userStorage.GetAuthFieldsByID(ctx, user.Id)
	if err != nil {
		s.logger.Error("failed to load auth fields", "error", err, "procedure", "UnlinkProvider", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}
	if af.PasswordHash == "" {
		links, err := s.oauthIdentityDB.GetByUserID(ctx, user.Id)
		if err != nil {
			s.logger.Error("failed to list linked providers", "error", err, "procedure", "UnlinkProvider", "user_id", user.Id)
			return nil, connerr.FromDB(err)
		}
		remaining := 0
		for _, l := range links {
			if l.Provider != pName {
				remaining++
			}
		}
		if remaining == 0 {
			return nil, connerr.FailedPrecondition("cannot unlink the only login method; set a password first")
		}
	}

	if err := s.oauthIdentityDB.DeleteByUserAndProvider(ctx, user.Id, pName); err != nil {
		s.logger.Error("failed to unlink provider", "error", err, "procedure", "UnlinkProvider", "user_id", user.Id, "provider", pName)
		return nil, connerr.FromDB(err)
	}
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_OAUTH_UNLINKED, "", "", map[string]any{"provider": pName})
	}
	return &connect.Response[v1.UnlinkProviderResponse]{Msg: &v1.UnlinkProviderResponse{}}, nil
}
