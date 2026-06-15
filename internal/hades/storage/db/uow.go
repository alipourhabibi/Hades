package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TransactionFN is a callback executed within a database transaction.
// Returning a non-nil error triggers an automatic rollback.
type TransactionFN func(ctx context.Context, tx pgx.Tx) (interface{}, error)

// UnitOfWork abstracts a transactional boundary so that callers do not
// need to manage Begin/Commit/Rollback directly.
type UnitOfWork interface {
	Do(ctx context.Context, fn TransactionFN, timeout time.Duration) (interface{}, error)
}

// PGUnitOfWork implements UnitOfWork on top of a pgx connection pool.
type PGUnitOfWork struct {
	Pool *pgxpool.Pool
}

// NewUnitOfWork returns a UnitOfWork backed by the given pool.
func NewUnitOfWork(pool *pgxpool.Pool) *PGUnitOfWork {
	return &PGUnitOfWork{Pool: pool}
}

// Do acquires a connection, begins a transaction, executes fn, and either
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

	result, err := fn(ctx, tx)
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
