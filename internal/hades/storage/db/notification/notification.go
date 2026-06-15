// Package notification provides storage operations for in-app notification
// records.  Notifications are user-scoped events (e.g. a new commit on a
// watched module, an SDK job finishing) stored in the notifications table
// (migration 021).
package notification

import (
	"context"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// NotificationStorage handles CRUD operations for the notifications table.
type NotificationStorage struct {
	db querier
}

// New creates a NotificationStorage backed by a connection pool.
func New(pool *pgxpool.Pool) *NotificationStorage {
	return &NotificationStorage{db: pool}
}

// WithTx returns a shallow copy of NotificationStorage that executes queries
// within the given transaction instead of the pool.
func (s *NotificationStorage) WithTx(tx pgx.Tx) *NotificationStorage {
	return &NotificationStorage{db: tx}
}

// ListForUser returns all notifications for userID, ordered newest first.
// Already-read notifications are included; callers may filter on the Read field
// if they only want unread ones.
func (s *NotificationStorage) ListForUser(ctx context.Context, userID string) ([]*registryv1.Notification, error) {
	query := `
SELECT id, type, title, COALESCE(body,''), COALESCE(resource_id,''), read_at, created_at
FROM notifications
WHERE user_id = $1
ORDER BY created_at DESC`

	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []*registryv1.Notification
	for rows.Next() {
		n := &registryv1.Notification{}
		var createdAt time.Time
		var readAt *time.Time
		if err := rows.Scan(
			&n.Id,
			&n.Type,
			&n.Title,
			&n.Body,
			&n.ResourceId,
			&readAt,
			&createdAt,
		); err != nil {
			return nil, err
		}
		n.Read = readAt != nil
		n.CreateTime = timestamppb.New(createdAt)
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

// MarkRead sets read_at = NOW() for the notification identified by (id, userID).
// The userID check prevents users from marking each other's notifications.
// Silently succeeds if the notification is already marked read.
func (s *NotificationStorage) MarkRead(ctx context.Context, id, userID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE notifications SET read_at = NOW() WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		id, userID)
	return err
}
