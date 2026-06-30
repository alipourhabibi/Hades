package postgres

import (
	"context"
	"encoding/json"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditLogStorage struct {
	pool *pgxpool.Pool
}

func NewAuditLog(pool *pgxpool.Pool) *AuditLogStorage {
	return &AuditLogStorage{pool: pool}
}

func (s *AuditLogStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *AuditLogStorage) Create(ctx context.Context, userID *string, event, ipAddress, userAgent string, metadata map[string]any) error {
	var metaJSON []byte
	if metadata != nil {
		var err error
		metaJSON, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	}
	_, err := s.q(ctx).Exec(ctx,
		`INSERT INTO audit_log (user_id, event, ip_address, user_agent, metadata)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, event, ipAddress, userAgent, metaJSON,
	)
	return err
}

func (s *AuditLogStorage) List(ctx context.Context, userID string, pageSize, offset int) ([]*auditlog.Row, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	rows, err := s.q(ctx).Query(ctx,
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

	var result []*auditlog.Row
	for rows.Next() {
		row := &auditlog.Row{}
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

func (s *AuditLogStorage) RecentIPsForUser(ctx context.Context, userID string, n int) ([]string, error) {
	rows, err := s.q(ctx).Query(ctx,
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

var _ auditlog.Storage = (*AuditLogStorage)(nil)
