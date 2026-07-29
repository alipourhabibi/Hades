// Package db defines the Store interface that aggregates all domain storage interfaces.
package db

import (
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
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
)

// Store aggregates all domain storage interfaces and the unit-of-work.
// Implementations are returned by New in client.go.
type Store interface {
	UnitOfWork

	User() user.Storage
	Session() session.Storage
	Module() module.Storage
	Commit() commit.Storage
	OPABinding() opabinding.Storage
	SDKJob() sdkjob.Storage
	Org() org.Storage
	CIRun() cirun.Storage
	Notification() notification.Storage
	EmailVerification() emailverification.Storage
	PasswordReset() passwordreset.Storage
	Resource() resource.Storage
	OAuthIdentity() oauthidentity.Storage
	APIToken() apitoken.Storage
	DeviceGrant() devicegrant.Storage
	TOTPSecret() totpsecret.Storage
	BackupCode() backupcode.Storage
	AuditLog() auditlog.Storage

	// GitalyOpLog returns the concrete Gitaly operation log storage.
	// It is PostgreSQL-only and has no SQLite equivalent or interface.
	GitalyOpLog() *gitalyoplog.GitalyOpLogStorage
}
