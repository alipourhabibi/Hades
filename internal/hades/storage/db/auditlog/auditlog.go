// Package auditlog provides an append-only audit log for security-relevant
// user actions (logins, token operations, password changes, etc.).
// Rows are never updated or deleted; callers only insert and query.
package auditlog

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type AuditLogStorage struct {
	db querier
}

func New(pool *pgxpool.Pool) *AuditLogStorage {
	return &AuditLogStorage{db: pool}
}

func (s *AuditLogStorage) WithTx(tx pgx.Tx) *AuditLogStorage {
	return &AuditLogStorage{db: tx}
}

type Row struct {
	ID        uuid.UUID
	UserID    *string
	Event     string
	IPAddress string
	UserAgent string
	Metadata  map[string]any
	CreatedAt time.Time
}

// Create inserts an audit log entry. metadata may be nil.
func (s *AuditLogStorage) Create(ctx context.Context, userID *string, event, ipAddress, userAgent string, metadata map[string]any) error {
	var metaJSON []byte
	if metadata != nil {
		var err error
		metaJSON, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	}
	_, err := s.db.Exec(ctx,
		`INSERT INTO audit_log (user_id, event, ip_address, user_agent, metadata)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, event, ipAddress, userAgent, metaJSON,
	)
	return err
}

// List returns audit events for a user, newest first, with pagination.
func (s *AuditLogStorage) List(ctx context.Context, userID string, pageSize, offset int) ([]*Row, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, event, COALESCE(ip_address,''), COALESCE(user_agent,''), metadata, create_time
		 FROM audit_log WHERE user_id = $1
		 ORDER BY create_time DESC
		 LIMIT $2 OFFSET $3`,
		userID, pageSize, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Row
	for rows.Next() {
		row := &Row{}
		var metaJSON []byte
		if err := rows.Scan(&row.ID, &row.UserID, &row.Event, &row.IPAddress, &row.UserAgent, &metaJSON, &row.CreatedAt); err != nil {
			return nil, err
		}
		if metaJSON != nil {
			_ = json.Unmarshal(metaJSON, &row.Metadata)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// RecentIPsForUser returns the last N distinct IP addresses seen for a user.
func (s *AuditLogStorage) RecentIPsForUser(ctx context.Context, userID string, n int) ([]string, error) {
	rows, err := s.db.Query(ctx,
		`SELECT ip_address FROM audit_log
		 WHERE user_id = $1 AND ip_address IS NOT NULL AND ip_address != ''
		 GROUP BY ip_address
		 ORDER BY MAX(create_time) DESC
		 LIMIT $2`,
		userID, n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ips []string
	for rows.Next() {
		var ip string
		if e := rows.Scan(&ip); e != nil {
			return nil, e
		}
		ips = append(ips, ip)
	}
	return ips, rows.Err()
}
