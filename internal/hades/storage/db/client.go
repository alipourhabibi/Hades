// Package db provides the PostgreSQL storage layer. Each domain entity
// has its own storage struct backed by a shared pgx connection pool.
package db

import (
	"context"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/backupcode"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/devicegrant"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/emailverification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/oauthidentity"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/org"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/passwordreset"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBs aggregates all PostgreSQL storage backends. It is constructed once
// at startup and injected into the server's Dependencies struct.
type DBs struct {
	UserStorage              *user.UserStorage
	SessionStorage           *session.SessionStorage
	ModuleStorage            *module.ModuleStorage
	OPABindingStorage        *opabinding.OPABindingStorage
	CommitStorage            *commit.CommitStorage
	SDKJobStorage            *sdkjob.SDKJobStorage
	OrgStorage               *org.OrgStorage
	CIRunStorage             *cirun.CIRunStorage
	NotificationStorage      *notification.NotificationStorage
	GitalyOpLogStorage       *gitalyoplog.GitalyOpLogStorage
	EmailVerificationStorage *emailverification.EmailVerificationStorage
	PasswordResetStorage     *passwordreset.PasswordResetStorage
	OAuthIdentityStorage     *oauthidentity.OAuthIdentityStorage
	APITokenStorage          *apitoken.APITokenStorage
	DeviceGrantStorage       *devicegrant.DeviceGrantStorage
	TOTPSecretStorage        *totpsecret.TOTPSecretStorage
	BackupCodeStorage        *backupcode.BackupCodeStorage
	AuditLogStorage          *auditlog.AuditLogStorage
	UOW                      UnitOfWork
}

// New opens a pgx connection pool and initialises all storage backends.
func New(c config.DB, logger *log.LoggerWrapper) (*DBs, error) {
	ctx := context.Background()

	config, err := pgxpool.ParseConfig(c.ConnectionString)
	if err != nil {
		return nil, err
	}

	pgxDB, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	uow := NewUnitOfWork(pgxDB)
	userStorage := user.New(pgxDB)
	sessionStorage := session.New(pgxDB)
	opaBindingStorage := opabinding.New(pgxDB)
	moduleStorage := module.New(pgxDB)
	commitStorage := commit.New(pgxDB)
	sdkJobStorage := sdkjob.New(pgxDB)
	orgStorage := org.New(pgxDB)
	ciRunStorage := cirun.New(pgxDB)
	notificationStorage := notification.New(pgxDB)
	gitalyOpLogStorage := gitalyoplog.New(pgxDB)
	emailVerificationStorage := emailverification.New(pgxDB)
	passwordResetStorage := passwordreset.New(pgxDB)
	oauthIdentityStorage := oauthidentity.New(pgxDB)
	apiTokenStorage := apitoken.New(pgxDB)
	deviceGrantStorage := devicegrant.New(pgxDB)
	totpSecretStorage := totpsecret.New(pgxDB)
	backupCodeStorage := backupcode.New(pgxDB)
	auditLogStorage := auditlog.New(pgxDB)

	return &DBs{
		UserStorage:              userStorage,
		SessionStorage:           sessionStorage,
		ModuleStorage:            moduleStorage,
		OPABindingStorage:        opaBindingStorage,
		CommitStorage:            commitStorage,
		SDKJobStorage:            sdkJobStorage,
		OrgStorage:               orgStorage,
		CIRunStorage:             ciRunStorage,
		NotificationStorage:      notificationStorage,
		GitalyOpLogStorage:       gitalyOpLogStorage,
		EmailVerificationStorage: emailVerificationStorage,
		PasswordResetStorage:     passwordResetStorage,
		OAuthIdentityStorage:     oauthIdentityStorage,
		APITokenStorage:          apiTokenStorage,
		DeviceGrantStorage:       deviceGrantStorage,
		TOTPSecretStorage:        totpSecretStorage,
		BackupCodeStorage:        backupCodeStorage,
		AuditLogStorage:          auditLogStorage,
		UOW:                      uow,
	}, nil
}
