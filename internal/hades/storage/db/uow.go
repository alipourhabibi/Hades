package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TransactionFN func(ctx context.Context, tx pgx.Tx) (interface{}, error)

type UnitOfWork interface {
	Do(ctx context.Context, fn TransactionFN, timeout time.Duration) (interface{}, error)
}

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

	var result interface{}
	result, err = fn(ctx, tx)
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
