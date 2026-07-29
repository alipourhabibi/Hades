package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TransactionFN is the function signature for UoW callbacks.
// The transaction is injected into ctx via txkeys.PgxTxKey{} so that all
// storage implementations can pick it up with txkeys.PgxTxFromContext.
type TransactionFN func(ctx context.Context) (interface{}, error)

// UnitOfWork manages database transactions.
type UnitOfWork interface {
	Do(ctx context.Context, fn TransactionFN, timeout time.Duration) (interface{}, error)
}

// PGUnitOfWork implements UnitOfWork for PostgreSQL.
type PGUnitOfWork struct {
	Pool *pgxpool.Pool
}

// NewUnitOfWork creates a PGUnitOfWork backed by the given pool.
func NewUnitOfWork(pool *pgxpool.Pool) *PGUnitOfWork {
	return &PGUnitOfWork{Pool: pool}
}

// Do begins a transaction, injects it into ctx via txkeys, calls fn, then
// commits or rolls back depending on whether fn returned an error.
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

	// Inject the transaction into the context so storage implementations can
	// use it transparently via txkeys.PgxTxFromContext.
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
