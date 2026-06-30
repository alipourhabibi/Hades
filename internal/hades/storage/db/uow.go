package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TransactionFN is a callback executed within a database transaction.
// The active transaction is injected into ctx via txkeys; storage structs
// can retrieve it using txkeys.PgxTxFromContext or txkeys.SQLTxFromContext.
type TransactionFN func(ctx context.Context) (interface{}, error)

// UnitOfWork abstracts a transactional boundary.
type UnitOfWork interface {
	Do(ctx context.Context, fn TransactionFN, timeout time.Duration) (interface{}, error)
}

// PgxTxFromContext is a convenience re-export for callers that import only package db.
func PgxTxFromContext(ctx context.Context) (pgx.Tx, bool) {
	return txkeys.PgxTxFromContext(ctx)
}

// SQLTxFromContext is a convenience re-export for callers that import only package db.
func SQLTxFromContext(ctx context.Context) (*sql.Tx, bool) {
	return txkeys.SQLTxFromContext(ctx)
}

// PGUnitOfWork implements UnitOfWork on top of a pgx connection pool.
type PGUnitOfWork struct {
	Pool *pgxpool.Pool
}

func NewUnitOfWork(pool *pgxpool.Pool) *PGUnitOfWork {
	return &PGUnitOfWork{Pool: pool}
}

func (uow *PGUnitOfWork) Do(ctx context.Context, fn TransactionFN, timeout time.Duration) (interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := uow.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}

	txCtx := context.WithValue(ctx, txkeys.PgxTxKey{}, tx)

	result, err := fn(txCtx)
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return nil, fmt.Errorf("transaction rollback failed: %v for error: %v", rollbackErr, err)
		}
		return nil, err
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		return nil, fmt.Errorf("transaction commit failed: %v", commitErr)
	}

	return result, nil
}

// SQLiteUnitOfWork implements UnitOfWork on top of a *sql.DB (SQLite).
type SQLiteUnitOfWork struct {
	DB *sql.DB
}

func NewSQLiteUnitOfWork(db *sql.DB) *SQLiteUnitOfWork {
	return &SQLiteUnitOfWork{DB: db}
}

func (uow *SQLiteUnitOfWork) Do(ctx context.Context, fn TransactionFN, timeout time.Duration) (interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tx, err := uow.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	txCtx := context.WithValue(ctx, txkeys.SQLTxKey{}, tx)

	result, err := fn(txCtx)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("transaction rollback failed: %v for error: %v", rollbackErr, err)
		}
		return nil, err
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return nil, fmt.Errorf("transaction commit failed: %v", commitErr)
	}

	return result, nil
}
