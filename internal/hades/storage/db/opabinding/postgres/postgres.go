// Package postgres provides the PostgreSQL implementation of opabinding.Storage.
package postgres

import (
	"context"
	"fmt"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OPABindingStorage implements opabinding.Storage against PostgreSQL.
type OPABindingStorage struct {
	pool *pgxpool.Pool
}

// New creates an OPABindingStorage backed by a connection pool.
func New(pool *pgxpool.Pool) *OPABindingStorage {
	return &OPABindingStorage{pool: pool}
}

var _ opabinding.Storage = (*OPABindingStorage)(nil)

func (s *OPABindingStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

// Create inserts a single role binding. Silently ignores conflicts.
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

// CreateBatch inserts multiple role bindings. Conflicts are silently ignored.
func (s *OPABindingStorage) CreateBatch(ctx context.Context, bindings []opabinding.RoleBinding) error {
	if len(bindings) == 0 {
		return nil
	}
	for i, b := range bindings {
		_, err := s.q(ctx).Exec(ctx,
			`INSERT INTO opa_role_bindings (subject, role, domain)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (subject, role, domain) DO NOTHING`,
			b.Subject, b.Role, b.Domain,
		)
		if err != nil {
			return fmt.Errorf("opabinding: create batch [%d]: %w", i, err)
		}
	}
	return nil
}

// ListAll returns every row in opa_role_bindings.
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

// Delete removes a role binding by its UUID primary key.
func (s *OPABindingStorage) Delete(ctx context.Context, id string) error {
	_, err := s.q(ctx).Exec(ctx,
		`DELETE FROM opa_role_bindings WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("opabinding: delete: %w", err)
	}
	return nil
}

// ListBySubject returns all role bindings for the given subject.
func (s *OPABindingStorage) ListBySubject(ctx context.Context, subject string) ([]opabinding.RoleBinding, error) {
	rows, err := s.q(ctx).Query(ctx,
		`SELECT id, subject, role, domain, created_at FROM opa_role_bindings WHERE subject = $1 ORDER BY created_at`,
		subject,
	)
	if err != nil {
		return nil, fmt.Errorf("opabinding: list by subject: %w", err)
	}
	defer rows.Close()

	var out []opabinding.RoleBinding
	for rows.Next() {
		var b opabinding.RoleBinding
		if err := rows.Scan(&b.ID, &b.Subject, &b.Role, &b.Domain, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("opabinding: list by subject scan: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeleteBySubjectDomain removes all role bindings for a given subject and domain.
func (s *OPABindingStorage) DeleteBySubjectDomain(ctx context.Context, subject, domain string) error {
	_, err := s.q(ctx).Exec(ctx,
		`DELETE FROM opa_role_bindings WHERE subject = $1 AND domain = $2`,
		subject, domain,
	)
	if err != nil {
		return fmt.Errorf("opabinding: delete by subject domain: %w", err)
	}
	return nil
}
