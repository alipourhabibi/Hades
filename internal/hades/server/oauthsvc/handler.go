// Package oauthsvc implements the OAuthService ConnectRPC handler. It
// supports GitHub and Google as identity providers. On first login an
// account is created automatically; on subsequent logins the existing
// account is used. A session token is issued after a successful OAuth
// callback.
package oauthsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1"
	v1connect "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/oauthidentity"
	dbsession "github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	dbuser "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

type Handler struct {
	v1connect.OAuthServiceHandler

	logger           *log.LoggerWrapper
	userStorage      *dbuser.UserStorage
	sessionStorage   *dbsession.SessionStorage
	oauthIdentityDB  *oauthidentity.OAuthIdentityStorage
	auditLogDB       *auditlog.AuditLogStorage
	authorizationSvc *authorization.Server
	oauthCfg         config.OAuthConfig
	authCfg          config.AuthConfig
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:           deps.Logger,
		userStorage:      deps.UserDB,
		sessionStorage:   deps.SessionDB,
		oauthIdentityDB:  deps.OAuthIdentityDB,
		auditLogDB:       deps.AuditLogDB,
		authorizationSvc: deps.Authorization,
		oauthCfg:         deps.OAuthConfig,
		authCfg:          deps.AuthConfig,
	}
}

func (h *Handler) oauth2Config(provider v1.OAuthProvider) (*oauth2.Config, error) {
	switch provider {
	case v1.OAuthProvider_OAUTH_PROVIDER_GITHUB:
		return &oauth2.Config{
			ClientID:     h.oauthCfg.GitHub.ClientID,
			ClientSecret: h.oauthCfg.GitHub.ClientSecret,
			RedirectURL:  h.oauthCfg.GitHub.RedirectURL,
			Endpoint:     github.Endpoint,
			Scopes:       []string{"user:email"},
		}, nil
	case v1.OAuthProvider_OAUTH_PROVIDER_GOOGLE:
		return &oauth2.Config{
			ClientID:     h.oauthCfg.Google.ClientID,
			ClientSecret: h.oauthCfg.Google.ClientSecret,
			RedirectURL:  h.oauthCfg.Google.RedirectURL,
			Endpoint:     google.Endpoint,
			Scopes:       []string{"openid", "email", "profile"},
		}, nil
	default:
		return nil, connErr.InvalidArgument("unsupported OAuth provider")
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

func (h *Handler) GetOAuthURL(ctx context.Context, in *connect.Request[v1.GetOAuthURLRequest]) (*connect.Response[v1.GetOAuthURLResponse], error) {
	cfg, err := h.oauth2Config(in.Msg.Provider)
	if err != nil {
		return nil, err
	}
	url := cfg.AuthCodeURL(in.Msg.State, oauth2.AccessTypeOnline)
	return &connect.Response[v1.GetOAuthURLResponse]{
		Msg: &v1.GetOAuthURLResponse{Url: url},
	}, nil
}

func (h *Handler) OAuthCallback(ctx context.Context, in *connect.Request[v1.OAuthCallbackRequest]) (*connect.Response[v1.OAuthCallbackResponse], error) {
	cfg, err := h.oauth2Config(in.Msg.Provider)
	if err != nil {
		return nil, err
	}

	oauthToken, err := cfg.Exchange(ctx, in.Msg.Code)
	if err != nil {
		h.logger.Warn("OAuth token exchange failed", "error", err, "procedure", "OAuthCallback")
		return nil, connErr.Unauthenticated("OAuth token exchange failed")
	}

	providerUID, emailAddr, err := h.fetchProviderProfile(ctx, in.Msg.Provider, oauthToken)
	if err != nil {
		return nil, err
	}

	pName := providerName(in.Msg.Provider)

	// Look up existing OAuth identity.
	identity, err := h.oauthIdentityDB.GetByProviderUID(ctx, pName, providerUID)
	var userID string
	if err != nil {
		// No existing identity - try to find user by email or create new user.
		user, userErr := h.userStorage.GetByEmail(ctx, emailAddr)
		if userErr != nil {
			// Create a new user.
			username := fmt.Sprintf("%s_%s", pName, providerUID)
			if createErr := h.userStorage.Create(ctx, username, emailAddr, "", registryv1.UserType_USER_TYPE_USER, registryv1.UserState_USER_STATE_ACTIVE, "", ""); createErr != nil {
				h.logger.Error("failed to create OAuth user", "error", createErr, "procedure", "OAuthCallback")
				return nil, connErr.FromPgx(createErr)
			}
			// Mark email verified since it came from OAuth.
			newUser, _ := h.userStorage.GetByUsername(ctx, username)
			userID = newUser.Id
			_ = h.userStorage.SetEmailVerified(ctx, userID)
			_ = h.authorizationSvc.AddBasicRoles(ctx, username)
		} else {
			userID = user.Id
		}
		// Link the OAuth identity.
		_ = h.oauthIdentityDB.Create(ctx, userID, pName, providerUID, emailAddr)
		if h.auditLogDB != nil {
			_ = h.auditLogDB.Create(ctx, &userID, "oauth_linked", "", "", map[string]any{"provider": pName})
		}
	} else {
		userID = identity.UserID
	}

	// Issue session token.
	raw, hash, err := utilscrypto.GenerateToken()
	if err != nil {
		h.logger.Error("failed to generate session token", "error", err, "procedure", "OAuthCallback")
		return nil, connErr.Internal("failed to generate session token")
	}
	idleDays := h.authCfg.Session.IdleTimeoutDays
	if idleDays == 0 {
		idleDays = 7
	}
	absDays := h.authCfg.Session.AbsoluteTimeoutDays
	if absDays == 0 {
		absDays = 14
	}
	_, err = h.sessionStorage.CreateWithToken(ctx, userID, "oauth:"+pName, hash, "", "", time.Now().Add(time.Duration(idleDays)*24*time.Hour), time.Now().Add(time.Duration(absDays)*24*time.Hour))
	if err != nil {
		h.logger.Error("failed to create session", "error", err, "procedure", "OAuthCallback", "user_id", userID)
		return nil, connErr.FromPgx(err)
	}

	return &connect.Response[v1.OAuthCallbackResponse]{
		Msg: &v1.OAuthCallbackResponse{
			Login: &v1.LoginResponse{Token: raw},
		},
	}, nil
}

func (h *Handler) fetchProviderProfile(ctx context.Context, provider v1.OAuthProvider, token *oauth2.Token) (uid, email string, err error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	switch provider {
	case v1.OAuthProvider_OAUTH_PROVIDER_GITHUB:
		return fetchGitHubProfile(client)
	case v1.OAuthProvider_OAUTH_PROVIDER_GOOGLE:
		return fetchGoogleProfile(client)
	}
	return "", "", connErr.InvalidArgument("unsupported provider")
}

type gitHubUser struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
	Login string `json:"login"`
}

func fetchGitHubProfile(client *http.Client) (uid, email string, err error) {
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return "", "", connErr.Internal("GitHub profile fetch failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var u gitHubUser
	if jsonErr := json.Unmarshal(body, &u); jsonErr != nil {
		return "", "", connErr.Internal("GitHub profile parse failed")
	}
	return fmt.Sprintf("%d", u.ID), u.Email, nil
}

type googleUser struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
}

func fetchGoogleProfile(client *http.Client) (uid, email string, err error) {
	resp, err := client.Get("https://openidconnect.googleapis.com/v1/userinfo")
	if err != nil {
		return "", "", connErr.Internal("Google profile fetch failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var u googleUser
	if jsonErr := json.Unmarshal(body, &u); jsonErr != nil {
		return "", "", connErr.Internal("Google profile parse failed")
	}
	return u.Sub, u.Email, nil
}

func (h *Handler) ListLinkedProviders(ctx context.Context, in *connect.Request[v1.ListLinkedProvidersRequest]) (*connect.Response[v1.ListLinkedProvidersResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "ListLinkedProviders")
		return nil, connErr.Internal("missing user in context")
	}

	rows, err := h.oauthIdentityDB.GetByUserID(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to get linked providers", "error", err, "procedure", "ListLinkedProviders", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
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

func (h *Handler) UnlinkProvider(ctx context.Context, in *connect.Request[v1.UnlinkProviderRequest]) (*connect.Response[v1.UnlinkProviderResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "UnlinkProvider")
		return nil, connErr.Internal("missing user in context")
	}

	pName := providerName(in.Msg.Provider)
	if err := h.oauthIdentityDB.DeleteByUserAndProvider(ctx, user.Id, pName); err != nil {
		h.logger.Error("failed to unlink provider", "error", err, "procedure", "UnlinkProvider", "user_id", user.Id, "provider", pName)
		return nil, connErr.FromPgx(err)
	}
	if h.auditLogDB != nil {
		_ = h.auditLogDB.Create(ctx, &user.Id, "oauth_unlinked", "", "", map[string]any{"provider": pName})
	}
	return &connect.Response[v1.UnlinkProviderResponse]{Msg: &v1.UnlinkProviderResponse{}}, nil
}
