// Package txkeys provides the shared context key types and accessor functions
// for database transactions injected by the UnitOfWork implementations.
// It is a separate leaf package to avoid import cycles between the db parent
// package (which imports all sub-packages) and the sub-packages themselves.
package txkeys

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PgxTxKey is the context key type used by PGUnitOfWork.
type PgxTxKey struct{}

// SQLTxKey is the context key type used by SQLiteUnitOfWork.
type SQLTxKey struct{}

// PgxQuerier is satisfied by both *pgxpool.Pool and pgx.Tx.
type PgxQuerier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// SQLQuerier is satisfied by both *sql.DB and *sql.Tx.
type SQLQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// PgxTxFromContext returns the pgx.Tx stored in ctx by PGUnitOfWork, if any.
func PgxTxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(PgxTxKey{}).(pgx.Tx)
	return tx, ok
}

// SQLTxFromContext returns the *sql.Tx stored in ctx by SQLiteUnitOfWork, if any.
func SQLTxFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(SQLTxKey{}).(*sql.Tx)
	return tx, ok
}
