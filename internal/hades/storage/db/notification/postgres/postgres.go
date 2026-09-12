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

// CreateBatch inserts one row per user in a single statement.
func (s *NotificationStorage) CreateBatch(ctx context.Context, userIDs []string, notificationType, title, body, resourceID string) error {
	if len(userIDs) == 0 {
		return nil
	}
	_, err := s.q(ctx).Exec(ctx, `
INSERT INTO notifications (user_id, type, title, body, resource_id)
SELECT unnest($1::uuid[]), $2, $3, $4, $5`,
		userIDs, notificationType, title, body, resourceID)
	return err
}

func (s *NotificationStorage) ListForUser(ctx context.Context, userID string, limit, offset int) ([]*identityv1.Notification, error) {
	query := `
SELECT id, type, title, COALESCE(body,''), COALESCE(resource_id,''), read_at, created_at
FROM notifications
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`

	rows, err := s.q(ctx).Query(ctx, query, userID, limit, offset)
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
	tag, err := s.q(ctx).Exec(ctx,
		`UPDATE notifications SET read_at = NOW() WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Zero rows means the notification does not exist, belongs to someone
		// else, or was already read. Reporting success for all three, which is
		// what this did, meant one user marking another's notification read
		// looked like it worked.
		return notification.ErrNotFound
	}
	return nil
}
