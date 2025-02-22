package uow

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type uowStore struct {
}

type unitOfWork struct {
	conn *pgx.Conn
}

type UnitOfWork interface {
	Do(context.Context, TransactionFN) (interface{}, error)
}

func New(db *pgx.Conn) UnitOfWork {
	return &unitOfWork{conn: db}
}

type TransactionFN func(ctx context.Context, tx pgx.Tx) (interface{}, error)

// Do executes the given TransactionFN atomically (inside a DB transaction).
func (s *unitOfWork) Do(ctx context.Context, fn TransactionFN) (interface{}, error) {
	// Start a transaction
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}

	// Ensure the transaction is rolled back in case of any error
	defer func() {
		if err != nil {
			tx.Rollback(ctx)
		}
	}()

	// Execute the function block with the store
	result, err := fn(ctx, tx)
	if err != nil {
		return nil, err
	}

	// Commit the transaction if everything is successful
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return result, nil
}
