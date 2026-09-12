// Package db provides the metadata storage layer. Use NewFromConfig to select
// the backend (PostgreSQL or SQLite) based on config.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	apitokenpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken/postgres"
	apitokensq "github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	auditlogpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog/postgres"
	auditlogsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/backupcode"
	backupcodepg "github.com/alipourhabibi/Hades/internal/hades/storage/db/backupcode/postgres"
	backupcodesq "github.com/alipourhabibi/Hades/internal/hades/storage/db/backupcode/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun"
	cirunpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun/postgres"
	cirunsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	commitpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit/postgres"
	commitsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/devicegrant"
	devicegrantpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/devicegrant/postgres"
	devicegrantsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/devicegrant/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/emailverification"
	emailverificationpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/emailverification/postgres"
	emailverificationsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/emailverification/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	gitalyoplogpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog/postgres"
	gitalyoplogsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	modulepg "github.com/alipourhabibi/Hades/internal/hades/storage/db/module/postgres"
	modulesq "github.com/alipourhabibi/Hades/internal/hades/storage/db/module/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	notificationpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/notification/postgres"
	notificationsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/notification/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/oauthidentity"
	oauthidentitypg "github.com/alipourhabibi/Hades/internal/hades/storage/db/oauthidentity/postgres"
	oauthidentitysq "github.com/alipourhabibi/Hades/internal/hades/storage/db/oauthidentity/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	opabindingpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding/postgres"
	opabindingsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/org"
	orgpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/org/postgres"
	orgsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/org/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/passwordreset"
	passwordresetpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/passwordreset/postgres"
	passwordresetsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/passwordreset/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	resourcepg "github.com/alipourhabibi/Hades/internal/hades/storage/db/resource/postgres"
	resourcesq "github.com/alipourhabibi/Hades/internal/hades/storage/db/resource/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	sdkjobpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob/postgres"
	sdkjobsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	sessionpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/session/postgres"
	sessionsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/session/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	totpsecretpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret/postgres"
	totpsecretsq "github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	userpg "github.com/alipourhabibi/Hades/internal/hades/storage/db/user/postgres"
	usersq "github.com/alipourhabibi/Hades/internal/hades/storage/db/user/sqlite"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/jackc/pgx/v5/pgxpool"
)

// concreteStore is the private implementation of Store.
type concreteStore struct {
	uow               UnitOfWork
	userStorage       user.Storage
	sessionStorage    session.Storage
	moduleStorage     module.Storage
	commitStorage     commit.Storage
	opaBindingStorage opabinding.Storage
	sdkJobStorage     sdkjob.Storage
	orgStorage        org.Storage
	ciRunStorage      cirun.Storage
	notifStorage      notification.Storage
	emailVerStorage   emailverification.Storage
	pwdResetStorage   passwordreset.Storage
	resourceStorage   resource.Storage
	oauthStorage      oauthidentity.Storage
	apiTokenStorage   apitoken.Storage
	deviceStorage     devicegrant.Storage
	totpStorage       totpsecret.Storage
	backupStorage     backupcode.Storage
	auditStorage      auditlog.Storage
	gitalyOpLog       gitalyoplog.Storage
}

var _ Store = (*concreteStore)(nil)

func (s *concreteStore) Do(ctx context.Context, fn TransactionFN, timeout time.Duration) (interface{}, error) {
	return s.uow.Do(ctx, fn, timeout)
}

// Accessor methods.
func (s *concreteStore) User() user.Storage                           { return s.userStorage }
func (s *concreteStore) Session() session.Storage                     { return s.sessionStorage }
func (s *concreteStore) Module() module.Storage                       { return s.moduleStorage }
func (s *concreteStore) Commit() commit.Storage                       { return s.commitStorage }
func (s *concreteStore) OPABinding() opabinding.Storage               { return s.opaBindingStorage }
func (s *concreteStore) SDKJob() sdkjob.Storage                       { return s.sdkJobStorage }
func (s *concreteStore) Org() org.Storage                             { return s.orgStorage }
func (s *concreteStore) CIRun() cirun.Storage                         { return s.ciRunStorage }
func (s *concreteStore) Notification() notification.Storage           { return s.notifStorage }
func (s *concreteStore) EmailVerification() emailverification.Storage { return s.emailVerStorage }
func (s *concreteStore) PasswordReset() passwordreset.Storage         { return s.pwdResetStorage }
func (s *concreteStore) Resource() resource.Storage                   { return s.resourceStorage }
func (s *concreteStore) OAuthIdentity() oauthidentity.Storage         { return s.oauthStorage }
func (s *concreteStore) APIToken() apitoken.Storage                   { return s.apiTokenStorage }
func (s *concreteStore) DeviceGrant() devicegrant.Storage             { return s.deviceStorage }
func (s *concreteStore) TOTPSecret() totpsecret.Storage               { return s.totpStorage }
func (s *concreteStore) BackupCode() backupcode.Storage               { return s.backupStorage }
func (s *concreteStore) AuditLog() auditlog.Storage                   { return s.auditStorage }
func (s *concreteStore) GitalyOpLog() gitalyoplog.Storage             { return s.gitalyOpLog }

// NewFromConfig selects the database backend from cfg.Backends.Database.
func NewFromConfig(cfg config.Config, logger *log.LoggerWrapper) (Store, error) {
	if cfg.Backends.Database == config.DatabaseSQLite || cfg.Backends.Database == "" {
		return NewSQLite(cfg, logger)
	}
	return New(cfg.DB, logger)
}

// New opens a pgx connection pool and returns a Store backed by PostgreSQL.
func New(c config.DB, logger *log.LoggerWrapper) (Store, error) {
	ctx := context.Background()

	pgxCfg, err := pgxpool.ParseConfig(c.ConnectionString)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, err
	}

	pgRes := resourcepg.NewResource(pool)
	return &concreteStore{
		uow:               NewUnitOfWork(pool),
		userStorage:       userpg.New(pool),
		sessionStorage:    sessionpg.New(pool),
		moduleStorage:     modulepg.New(pool, pgRes),
		commitStorage:     commitpg.New(pool, pgRes),
		resourceStorage:   pgRes,
		opaBindingStorage: opabindingpg.New(pool),
		sdkJobStorage:     sdkjobpg.New(pool),
		orgStorage:        orgpg.New(pool),
		ciRunStorage:      cirunpg.New(pool),
		notifStorage:      notificationpg.New(pool),
		emailVerStorage:   emailverificationpg.New(pool),
		pwdResetStorage:   passwordresetpg.New(pool),
		oauthStorage:      oauthidentitypg.New(pool),
		apiTokenStorage:   apitokenpg.New(pool),
		deviceStorage:     devicegrantpg.New(pool),
		totpStorage:       totpsecretpg.New(pool),
		backupStorage:     backupcodepg.New(pool),
		auditStorage:      auditlogpg.New(pool),
		gitalyOpLog:       gitalyoplogpg.New(pool),
	}, nil
}

// NewSQLite opens a SQLite database and returns a Store backed by SQLite.
func NewSQLite(cfg config.Config, logger *log.LoggerWrapper) (Store, error) {
	path := cfg.SQLite.Path
	if path == "" {
		path = ":memory:"
	}

	// Pragmas belong in the DSN, not in a one-off Exec.
	//
	// foreign_keys and busy_timeout are per-connection state, and database/sql
	// hands out connections from a pool: setting them with sqlDB.Exec applies
	// them to whichever single connection served that call, leaving foreign keys
	// disabled on every other connection in the pool. Driver support for
	// multi-statement Exec also varies, so the second pragma may never have run
	// at all.
	//
	// busy_timeout is what turns a concurrent-write SQLITE_BUSY into a short
	// wait instead of an immediate error.
	pragmas := "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_time_format=sqlite"
	dsn := path + "?" + pragmas
	if path == ":memory:" {
		dsn = "file::memory:?mode=memory&cache=shared&" + pragmas
	}
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: sqlite: open: %w", err)
	}

	// SQLite serialises writes at the file level. Allowing the pool to open many
	// connections converts that into SQLITE_BUSY errors under load rather than
	// throughput, so writes are funnelled through a single connection.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("db: sqlite: ping: %w", err)
	}
	// One versioned migration runner is the only thing that changes the schema.
	// See internal/hades/storage/db/sqlitemigrate.go.
	if err := migrateSQLite(sqlDB); err != nil {
		return nil, err
	}

	sqRes := resourcesq.NewResource(sqlDB)
	return &concreteStore{
		uow:               NewSQLiteUnitOfWork(sqlDB),
		userStorage:       usersq.NewUser(sqlDB),
		sessionStorage:    sessionsq.NewSession(sqlDB),
		moduleStorage:     modulesq.NewModule(sqlDB, sqRes),
		commitStorage:     commitsq.NewCommit(sqlDB, sqRes),
		resourceStorage:   sqRes,
		opaBindingStorage: opabindingsq.NewOPABinding(sqlDB),
		sdkJobStorage:     sdkjobsq.NewSDKJob(sqlDB),
		orgStorage:        orgsq.NewOrg(sqlDB),
		ciRunStorage:      cirunsq.NewCIRun(sqlDB),
		notifStorage:      notificationsq.NewNotification(sqlDB),
		emailVerStorage:   emailverificationsq.NewEmailVerification(sqlDB),
		pwdResetStorage:   passwordresetsq.NewPasswordReset(sqlDB),
		oauthStorage:      oauthidentitysq.NewOAuthIdentity(sqlDB),
		apiTokenStorage:   apitokensq.NewAPIToken(sqlDB),
		deviceStorage:     devicegrantsq.NewDeviceGrant(sqlDB),
		totpStorage:       totpsecretsq.NewTOTPSecret(sqlDB),
		backupStorage:     backupcodesq.NewBackupCode(sqlDB),
		auditStorage:      auditlogsq.NewAuditLog(sqlDB),
		gitalyOpLog:       gitalyoplogsq.NewGitalyOpLog(sqlDB),
	}, nil
}
