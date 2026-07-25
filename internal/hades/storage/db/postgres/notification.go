package postgres

import (
	"context"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// NotificationStorage handles CRUD operations for the notifications table.
type NotificationStorage struct {
	pool *pgxpool.Pool
}

func NewNotification(pool *pgxpool.Pool) *NotificationStorage {
	return &NotificationStorage{pool: pool}
}

func (s *NotificationStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *NotificationStorage) ListForUser(ctx context.Context, userID string) ([]*registryv1.Notification, error) {
	rows, err := s.q(ctx).Query(ctx, `
SELECT id, type, title, COALESCE(body,''), COALESCE(resource_id,''), read_at, created_at
FROM notifications
WHERE user_id = $1
ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []*registryv1.Notification
	for rows.Next() {
		n := &registryv1.Notification{}
		var createdAt time.Time
		var readAt *time.Time
		if err := rows.Scan(&n.Id, &n.Type, &n.Title, &n.Body, &n.ResourceId, &readAt, &createdAt); err != nil {
			return nil, err
		}
		n.Read = readAt != nil
		n.CreateTime = timestamppb.New(createdAt)
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

func (s *NotificationStorage) MarkRead(ctx context.Context, id, userID string) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE notifications SET read_at = NOW() WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		id, userID)
	return err
}

var _ notification.Storage = (*NotificationStorage)(nil)
