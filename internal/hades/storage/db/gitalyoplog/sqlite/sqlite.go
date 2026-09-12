// Package sqlite provides the SQLite implementation of gitalyoplog.Storage.
package sqlite

import (
	"context"
	"database/sql"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlutil"
	"github.com/google/uuid"
)

// SQLiteGitalyOpLogStorage persists git operation metadata.
//
// Like the PostgreSQL implementation it writes through the pool directly and
// never through a unit-of-work transaction, so a 'pending' row is durable
// before the git call it describes begins.
type SQLiteGitalyOpLogStorage struct {
	db *sql.DB
}

// NewGitalyOpLog returns a storage backed by the given database handle.
func NewGitalyOpLog(db *sql.DB) *SQLiteGitalyOpLogStorage {
	return &SQLiteGitalyOpLogStorage{db: db}
}

var _ gitalyoplog.Storage = (*SQLiteGitalyOpLogStorage)(nil)

func (s *SQLiteGitalyOpLogStorage) CreatePending(ctx context.Context, opType, moduleName, userID string) (uuid.UUID, error) {
	id := uuid.New()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO gitaly_operation_log (id, operation_type, status, module_name, user_id)
		 VALUES (?, ?, ?, ?, ?)`,
		sqlutil.UUID(id), opType, gitalyoplog.StatusPending, moduleName, sqlutil.ID(userID))
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (s *SQLiteGitalyOpLogStorage) UpdateStatus(ctx context.Context, id uuid.UUID, status, commitHash, errorReason string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE gitaly_operation_log
		 SET status = ?, commit_hash = NULLIF(?, ''), error_reason = NULLIF(?, ''), update_time = datetime('now')
		 WHERE id = ?`,
		status, commitHash, errorReason, sqlutil.UUID(id))
	return err
}
