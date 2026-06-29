// Package db provides the metadata storage layer. Each domain entity
// has its own storage struct. The factory functions construct the appropriate
// backend (PostgreSQL or SQLite) based on config.
package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for database/sql
	_ "modernc.org/sqlite"              // SQLite driver for database/sql

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
	pg "github.com/alipourhabibi/Hades/internal/hades/storage/db/postgres"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	sq "github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlite"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBs aggregates all metadata storage backends, each expressed as the
// domain interface rather than the concrete pgx struct.
type DBs struct {
	UserStorage              user.Storage
	SessionStorage           session.Storage
	ModuleStorage            module.Storage
	OPABindingStorage        opabinding.Storage
	CommitStorage            commit.Storage
	SDKJobStorage            sdkjob.Storage
	OrgStorage               org.Storage
	CIRunStorage             cirun.Storage
	NotificationStorage      notification.Storage
	GitalyOpLogStorage       *gitalyoplog.GitalyOpLogStorage
	EmailVerificationStorage emailverification.Storage
	PasswordResetStorage     passwordreset.Storage
	OAuthIdentityStorage     oauthidentity.Storage
	APITokenStorage          apitoken.Storage
	DeviceGrantStorage       devicegrant.Storage
	TOTPSecretStorage        totpsecret.Storage
	BackupCodeStorage        backupcode.Storage
	AuditLogStorage          auditlog.Storage
	UOW                      UnitOfWork
}

// New opens a pgx connection pool and initialises all PostgreSQL storage backends.
func New(c config.DB, logger *log.LoggerWrapper) (*DBs, error) {
	ctx := context.Background()

	pgxCfg, err := pgxpool.ParseConfig(c.ConnectionString)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, err
	}

	return &DBs{
		UserStorage:              pg.NewUser(pool),
		SessionStorage:           pg.NewSession(pool),
		ModuleStorage:            pg.NewModule(pool),
		OPABindingStorage:        pg.NewOPABinding(pool),
		CommitStorage:            pg.NewCommit(pool),
		SDKJobStorage:            pg.NewSDKJob(pool),
		OrgStorage:               pg.NewOrg(pool),
		CIRunStorage:             pg.NewCIRun(pool),
		NotificationStorage:      pg.NewNotification(pool),
		GitalyOpLogStorage:       gitalyoplog.New(pool),
		EmailVerificationStorage: pg.NewEmailVerification(pool),
		PasswordResetStorage:     pg.NewPasswordReset(pool),
		OAuthIdentityStorage:     pg.NewOAuthIdentity(pool),
		APITokenStorage:          pg.NewAPIToken(pool),
		DeviceGrantStorage:       pg.NewDeviceGrant(pool),
		TOTPSecretStorage:        pg.NewTOTPSecret(pool),
		BackupCodeStorage:        pg.NewBackupCode(pool),
		AuditLogStorage:          pg.NewAuditLog(pool),
		UOW:                      NewUnitOfWork(pool),
	}, nil
}

//go:embed sqlite_schema.sql
var sqliteMigration string

// NewSQLite opens a SQLite database and initialises all SQLite storage backends.
// The database file is created at cfg.SQLite.Path; use ":memory:" for tests.
func NewSQLite(cfg config.Config, logger *log.LoggerWrapper) (*DBs, error) {
	path := cfg.SQLite.Path
	if path == "" {
		path = ":memory:"
	}
	// _time_format=sqlite tells modernc.org/sqlite to parse/encode time.Time
	// values using SQLite's native datetime format ("2006-01-02 15:04:05").
	// Without this, datetime columns are returned as raw strings and
	// database/sql cannot scan them into time.Time.
	dsn := path + "?_time_format=sqlite"
	if path == ":memory:" {
		dsn = "file::memory:?mode=memory&cache=shared&_time_format=sqlite"
	}
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: sqlite: open: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("db: sqlite: ping: %w", err)
	}

	// Enable WAL mode for better concurrency.
	if _, err := sqlDB.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		return nil, fmt.Errorf("db: sqlite: pragma: %w", err)
	}

	// Apply embedded schema migration.
	if _, err := sqlDB.Exec(sqliteMigration); err != nil {
		return nil, fmt.Errorf("db: sqlite: migrate: %w", err)
	}

	return &DBs{
		UserStorage:              sq.NewUser(sqlDB),
		SessionStorage:           sq.NewSession(sqlDB),
		ModuleStorage:            sq.NewModule(sqlDB),
		OPABindingStorage:        sq.NewOPABinding(sqlDB),
		CommitStorage:            sq.NewCommit(sqlDB),
		SDKJobStorage:            sq.NewSDKJob(sqlDB),
		OrgStorage:               sq.NewOrg(sqlDB),
		CIRunStorage:             sq.NewCIRun(sqlDB),
		NotificationStorage:      sq.NewNotification(sqlDB),
		GitalyOpLogStorage:       nil, // GitalyOpLog is pgx-only; omit for SQLite
		EmailVerificationStorage: sq.NewEmailVerification(sqlDB),
		PasswordResetStorage:     sq.NewPasswordReset(sqlDB),
		OAuthIdentityStorage:     sq.NewOAuthIdentity(sqlDB),
		APITokenStorage:          sq.NewAPIToken(sqlDB),
		DeviceGrantStorage:       sq.NewDeviceGrant(sqlDB),
		TOTPSecretStorage:        sq.NewTOTPSecret(sqlDB),
		BackupCodeStorage:        sq.NewBackupCode(sqlDB),
		AuditLogStorage:          sq.NewAuditLog(sqlDB),
		UOW:                      NewSQLiteUnitOfWork(sqlDB),
	}, nil
}
