package uow

import (
	"context"

	"github.com/jackc/pgx/v5"
)

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

func (s *unitOfWork) Do(ctx context.Context, fn TransactionFN) (interface{}, error) {
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			tx.Rollback(ctx)
		}
	}()

	result, err := fn(ctx, tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return result, nil
}
