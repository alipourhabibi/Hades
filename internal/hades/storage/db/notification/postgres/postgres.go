// Package postgres provides the PostgreSQL implementation of notification.Storage.
package postgres

import (
	"context"
	"time"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// NotificationStorage implements notification.Storage against PostgreSQL.
type NotificationStorage struct {
	pool *pgxpool.Pool
}

// New creates a NotificationStorage backed by a connection pool.
func New(pool *pgxpool.Pool) *NotificationStorage {
	return &NotificationStorage{pool: pool}
}

var _ notification.Storage = (*NotificationStorage)(nil)

func (s *NotificationStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *NotificationStorage) Create(ctx context.Context, userID, notificationType, title, body, resourceID string) error {
	_, err := s.q(ctx).Exec(ctx, `
INSERT INTO notifications (user_id, type, title, body, resource_id)
VALUES ($1, $2, $3, $4, $5)`, userID, notificationType, title, body, resourceID)
	return err
}

func (s *NotificationStorage) ListForUser(ctx context.Context, userID string) ([]*identityv1.Notification, error) {
	query := `
SELECT id, type, title, COALESCE(body,''), COALESCE(resource_id,''), read_at, created_at
FROM notifications
WHERE user_id = $1
ORDER BY created_at DESC`

	rows, err := s.q(ctx).Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []*identityv1.Notification
	for rows.Next() {
		n := &identityv1.Notification{}
		var createdAt time.Time
		var readAt *time.Time
		if err := rows.Scan(
			&n.Id, &n.Type, &n.Title, &n.Body, &n.ResourceId,
			&readAt, &createdAt,
		); err != nil {
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
