package postgres

import (
	"context"
	"fmt"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// opaQuerier is satisfied by both *pgxpool.Pool and pgx.Tx, and also supports SendBatch.
type opaQuerier interface {
	txkeys.PgxQuerier
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// OPABindingStorage handles persistence for OPA role bindings.
type OPABindingStorage struct {
	pool *pgxpool.Pool
}

func NewOPABinding(pool *pgxpool.Pool) *OPABindingStorage {
	return &OPABindingStorage{pool: pool}
}

func (s *OPABindingStorage) q(ctx context.Context) opaQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *OPABindingStorage) Create(ctx context.Context, subject, role, domain string) error {
	_, err := s.q(ctx).Exec(ctx,
		`INSERT INTO opa_role_bindings (subject, role, domain)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (subject, role, domain) DO NOTHING`,
		subject, role, domain,
	)
	if err != nil {
		return fmt.Errorf("opabinding: create: %w", err)
	}
	return nil
}

func (s *OPABindingStorage) CreateBatch(ctx context.Context, bindings []opabinding.RoleBinding) error {
	if len(bindings) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, b := range bindings {
		batch.Queue(
			`INSERT INTO opa_role_bindings (subject, role, domain)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (subject, role, domain) DO NOTHING`,
			b.Subject, b.Role, b.Domain,
		)
	}
	results := s.q(ctx).SendBatch(ctx, batch)
	defer results.Close()
	for i := range bindings {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("opabinding: create batch [%d]: %w", i, err)
		}
	}
	return nil
}

func (s *OPABindingStorage) ListAll(ctx context.Context) ([]opabinding.RoleBinding, error) {
	rows, err := s.q(ctx).Query(ctx,
		`SELECT id, subject, role, domain, created_at FROM opa_role_bindings ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("opabinding: list all: %w", err)
	}
	defer rows.Close()

	var out []opabinding.RoleBinding
	for rows.Next() {
		var b opabinding.RoleBinding
		if err := rows.Scan(&b.ID, &b.Subject, &b.Role, &b.Domain, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("opabinding: list all scan: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *OPABindingStorage) Delete(ctx context.Context, id string) error {
	_, err := s.q(ctx).Exec(ctx,
		`DELETE FROM opa_role_bindings WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("opabinding: delete: %w", err)
	}
	return nil
}

var _ opabinding.Storage = (*OPABindingStorage)(nil)
