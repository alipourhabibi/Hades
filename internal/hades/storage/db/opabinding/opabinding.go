// Package opabinding provides PostgreSQL persistence for OPA role bindings.
// Bindings are synced to the in-memory OPA store on demand by the
// authorization engine.
package opabinding

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RoleBinding is a single row from the opa_role_bindings table.
type RoleBinding struct {
	ID        string
	Subject   string
	Role      string
	Domain    string
	CreatedAt time.Time
}

type querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (interface{ RowsAffected() int64 }, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// OPABindingStorage handles persistence for OPA role bindings.
type OPABindingStorage struct {
	db querier
}

// poolQuerier adapts pgxpool.Pool to the querier interface.
type poolQuerier struct{ p *pgxpool.Pool }

func (q *poolQuerier) Exec(ctx context.Context, sql string, arguments ...any) (interface{ RowsAffected() int64 }, error) {
	return q.p.Exec(ctx, sql, arguments...)
}
func (q *poolQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return q.p.QueryRow(ctx, sql, args...)
}
func (q *poolQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return q.p.Query(ctx, sql, args...)
}
func (q *poolQuerier) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return q.p.SendBatch(ctx, b)
}

// txQuerier adapts pgx.Tx to the querier interface.
type txQuerier struct{ tx pgx.Tx }

func (q *txQuerier) Exec(ctx context.Context, sql string, arguments ...any) (interface{ RowsAffected() int64 }, error) {
	return q.tx.Exec(ctx, sql, arguments...)
}
func (q *txQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return q.tx.QueryRow(ctx, sql, args...)
}
func (q *txQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return q.tx.Query(ctx, sql, args...)
}
func (q *txQuerier) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return q.tx.SendBatch(ctx, b)
}

// New creates a new OPABindingStorage backed by the given pool.
func New(pool *pgxpool.Pool) *OPABindingStorage {
	return &OPABindingStorage{db: &poolQuerier{pool}}
}

// WithTx returns a copy of OPABindingStorage scoped to the given transaction.
func (s *OPABindingStorage) WithTx(tx pgx.Tx) *OPABindingStorage {
	return &OPABindingStorage{db: &txQuerier{tx}}
}

// Create inserts a single role binding. Silently ignores conflicts (ON CONFLICT DO NOTHING).
func (s *OPABindingStorage) Create(ctx context.Context, subject, role, domain string) error {
	_, err := s.db.Exec(ctx,
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

// CreateBatch inserts multiple role bindings atomically using a pgx.Batch.
// Conflicts are silently ignored.
func (s *OPABindingStorage) CreateBatch(ctx context.Context, bindings []RoleBinding) error {
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
	results := s.db.SendBatch(ctx, batch)
	defer results.Close()
	for i := range bindings {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("opabinding: create batch [%d]: %w", i, err)
		}
	}
	return nil
}

// ListAll returns every row in opa_role_bindings (used to seed the OPA in-memory store).
func (s *OPABindingStorage) ListAll(ctx context.Context) ([]RoleBinding, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, subject, role, domain, created_at FROM opa_role_bindings ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("opabinding: list all: %w", err)
	}
	defer rows.Close()

	var out []RoleBinding
	for rows.Next() {
		var b RoleBinding
		if err := rows.Scan(&b.ID, &b.Subject, &b.Role, &b.Domain, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("opabinding: list all scan: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Delete removes a role binding by its UUID primary key.
func (s *OPABindingStorage) Delete(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM opa_role_bindings WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("opabinding: delete: %w", err)
	}
	return nil
}
