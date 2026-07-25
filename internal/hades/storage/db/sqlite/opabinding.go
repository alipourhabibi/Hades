package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
)

// SQLiteOPABindingStorage implements opabinding.Storage using database/sql with SQLite.
type SQLiteOPABindingStorage struct {
	db *sql.DB
}

func NewOPABinding(db *sql.DB) *SQLiteOPABindingStorage {
	return &SQLiteOPABindingStorage{db: db}
}

func (s *SQLiteOPABindingStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func (s *SQLiteOPABindingStorage) Create(ctx context.Context, subject, role, domain string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`INSERT OR IGNORE INTO opa_role_bindings (subject, role, domain) VALUES (?, ?, ?)`,
		subject, role, domain)
	if err != nil {
		return fmt.Errorf("opabinding: create: %w", err)
	}
	return nil
}

func (s *SQLiteOPABindingStorage) CreateBatch(ctx context.Context, bindings []opabinding.RoleBinding) error {
	for i, b := range bindings {
		_, err := s.q(ctx).ExecContext(ctx,
			`INSERT OR IGNORE INTO opa_role_bindings (subject, role, domain) VALUES (?, ?, ?)`,
			b.Subject, b.Role, b.Domain)
		if err != nil {
			return fmt.Errorf("opabinding: create batch [%d]: %w", i, err)
		}
	}
	return nil
}

func (s *SQLiteOPABindingStorage) ListAll(ctx context.Context) ([]opabinding.RoleBinding, error) {
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT id, subject, role, domain, created_at FROM opa_role_bindings ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("opabinding: list all: %w", err)
	}
	defer rows.Close()
	var out []opabinding.RoleBinding
	for rows.Next() {
		var b opabinding.RoleBinding
		var createdAt sqltypes.Time
		if err := rows.Scan(&b.ID, &b.Subject, &b.Role, &b.Domain, &createdAt); err != nil {
			return nil, fmt.Errorf("opabinding: list all scan: %w", err)
		}
		b.CreatedAt = createdAt.V
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *SQLiteOPABindingStorage) Delete(ctx context.Context, id string) error {
	_, err := s.q(ctx).ExecContext(ctx, `DELETE FROM opa_role_bindings WHERE id = ?`, id)
	return err
}

var _ opabinding.Storage = (*SQLiteOPABindingStorage)(nil)
