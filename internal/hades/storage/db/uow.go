package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5"
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
		// Roll back on a fresh context. Reusing ctx means that when the callback
		// failed because the timeout fired, the rollback is issued on an expired
		// context and fails too. The original error is always what is returned:
		// a rollback failure is an operational detail, not the reason the caller's
		// request failed.
		rollbackCtx, cancelRollback := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancelRollback()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return nil, fmt.Errorf("%w (rollback also failed: %v)", err, rollbackErr)
		}
		return nil, err
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		return nil, fmt.Errorf("transaction commit failed: %w", commitErr)
	}

	return result, nil
}

// rollbackTimeout bounds a rollback issued after the caller's context is
// already done.
const rollbackTimeout = 5 * time.Second

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
		// The original error is what the caller needs; a rollback failure is
		// reported alongside it rather than replacing it. sql.ErrTxDone means the
		// transaction already ended, which is not a failure worth surfacing.
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return nil, fmt.Errorf("%w (rollback also failed: %v)", err, rollbackErr)
		}
		return nil, err
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return nil, fmt.Errorf("transaction commit failed: %w", commitErr)
	}

	return result, nil
}
