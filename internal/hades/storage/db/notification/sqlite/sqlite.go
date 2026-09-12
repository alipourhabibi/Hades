package sqlite

import (
	"context"
	"database/sql"
	"strings"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlutil"
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

func (s *SQLiteNotificationStorage) Create(ctx context.Context, userID, notificationType, title, body, resourceID string) error {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	resourceID = sqlutil.ID(resourceID)
	_, err := s.q(ctx).ExecContext(ctx, `
INSERT INTO notifications (user_id, type, title, body, resource_id)
VALUES (?, ?, ?, ?, ?)`, userID, notificationType, title, body, resourceID)
	return err
}

// CreateBatch inserts one row per user in a single statement.
func (s *SQLiteNotificationStorage) CreateBatch(ctx context.Context, userIDs []string, notificationType, title, body, resourceID string) error {
	if len(userIDs) == 0 {
		return nil
	}
	resourceID = sqlutil.ID(resourceID)

	var b strings.Builder
	b.WriteString(`INSERT INTO notifications (user_id, type, title, body, resource_id) VALUES `)
	args := make([]any, 0, len(userIDs)*5)
	for i, u := range userIDs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(?, ?, ?, ?, ?)")
		args = append(args, sqlutil.ID(u), notificationType, title, body, resourceID)
	}
	_, err := s.q(ctx).ExecContext(ctx, b.String(), args...)
	return err
}

func (s *SQLiteNotificationStorage) ListForUser(ctx context.Context, userID string, limit, offset int) ([]*identityv1.Notification, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	rows, err := s.q(ctx).QueryContext(ctx, `
SELECT id, type, title, COALESCE(body,''), COALESCE(resource_id,''), read_at, created_at
FROM notifications WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notifications []*identityv1.Notification
	for rows.Next() {
		n := &identityv1.Notification{}
		var createdAt sqltypes.Time
		var readAt sqltypes.NullTime
		if err := rows.Scan(&n.Id, &n.Type, &n.Title, &n.Body, &n.ResourceId, &readAt, &createdAt); err != nil {
			return nil, err
		}
		n.Read = readAt.Valid
		n.CreateTime = timestamppb.New(createdAt.V)
		n.Id = sqlutil.Canonical(n.Id)
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

func (s *SQLiteNotificationStorage) MarkRead(ctx context.Context, id, userID string) error {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	id = sqlutil.ID(id)
	userID = sqlutil.ID(userID)
	res, err := s.q(ctx).ExecContext(ctx,
		`UPDATE notifications SET read_at = datetime('now') WHERE id = ? AND user_id = ? AND read_at IS NULL`,
		id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// See the PostgreSQL implementation: zero rows is a refusal, not a
		// success.
		return notification.ErrNotFound
	}
	return nil
}

var _ notification.Storage = (*SQLiteNotificationStorage)(nil)
