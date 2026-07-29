package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
)

// SQLiteAuditLogStorage implements auditlog.Storage using database/sql with SQLite.
type SQLiteAuditLogStorage struct {
	db *sql.DB
}

func NewAuditLog(db *sql.DB) *SQLiteAuditLogStorage {
	return &SQLiteAuditLogStorage{db: db}
}

func (s *SQLiteAuditLogStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func (s *SQLiteAuditLogStorage) Create(ctx context.Context, userID *string, event, ipAddress, userAgent string, metadata map[string]any) error {
	var metaJSON []byte
	if metadata != nil {
		var err error
		metaJSON, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	}
	_, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO audit_log (user_id, event, ip_address, user_agent, metadata) VALUES (?, ?, ?, ?, ?)`,
		userID, event, ipAddress, userAgent, metaJSON)
	return err
}

func (s *SQLiteAuditLogStorage) List(ctx context.Context, userID string, pageSize, offset int) ([]*auditlog.Row, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT id, user_id, event, COALESCE(ip_address,''), COALESCE(user_agent,''), metadata, create_time
		 FROM audit_log WHERE user_id = ?
		 ORDER BY create_time DESC LIMIT ? OFFSET ?`,
		userID, pageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*auditlog.Row
	for rows.Next() {
		row := &auditlog.Row{}
		var metaJSON []byte
		var createdAt sqltypes.Time
		if err := rows.Scan(&row.ID, &row.UserID, &row.Event, &row.IPAddress, &row.UserAgent, &metaJSON, &createdAt); err != nil {
			return nil, err
		}
		row.CreatedAt = createdAt.V
		if metaJSON != nil {
			_ = json.Unmarshal(metaJSON, &row.Metadata)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *SQLiteAuditLogStorage) RecentIPsForUser(ctx context.Context, userID string, n int) ([]string, error) {
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT ip_address FROM audit_log
		 WHERE user_id = ? AND ip_address IS NOT NULL AND ip_address != ''
		 GROUP BY ip_address ORDER BY MAX(create_time) DESC LIMIT ?`, userID, n)
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

var _ auditlog.Storage = (*SQLiteAuditLogStorage)(nil)
