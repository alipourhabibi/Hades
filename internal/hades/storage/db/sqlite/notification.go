package sqlite

import (
	"context"
	"database/sql"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SQLiteNotificationStorage implements notification.Storage using database/sql with SQLite.
type SQLiteNotificationStorage struct {
	db *sql.DB
}

func NewNotification(db *sql.DB) *SQLiteNotificationStorage {
	return &SQLiteNotificationStorage{db: db}
}

func (s *SQLiteNotificationStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func (s *SQLiteNotificationStorage) ListForUser(ctx context.Context, userID string) ([]*registryv1.Notification, error) {
	rows, err := s.q(ctx).QueryContext(ctx, `
SELECT id, type, title, COALESCE(body,''), COALESCE(resource_id,''), read_at, created_at
FROM notifications WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notifications []*registryv1.Notification
	for rows.Next() {
		n := &registryv1.Notification{}
		var createdAt sqltypes.Time
		var readAt sqltypes.NullTime
		if err := rows.Scan(&n.Id, &n.Type, &n.Title, &n.Body, &n.ResourceId, &readAt, &createdAt); err != nil {
			return nil, err
		}
		n.Read = readAt.Valid
		n.CreateTime = timestamppb.New(createdAt.V)
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

func (s *SQLiteNotificationStorage) MarkRead(ctx context.Context, id, userID string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE notifications SET read_at = datetime('now') WHERE id = ? AND user_id = ? AND read_at IS NULL`,
		id, userID)
	return err
}

var _ notification.Storage = (*SQLiteNotificationStorage)(nil)
