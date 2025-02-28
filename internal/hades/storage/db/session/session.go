package session

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SessionStorage struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *SessionStorage {
	return &SessionStorage{
		pool: pool,
	}
}

func (s *SessionStorage) Create(
	ctx context.Context,
	userId string,
	authModule string,
	expiresAt time.Time,
) (string, error) {
	query := `
INSERT INTO sessions (
  user_id,
  auth_module,
  expires_at
) VALUES ($1, $2, $3) RETURNING id`

	var id string
	err := s.pool.QueryRow(ctx, query,
		userId,
		authModule,
		expiresAt,
	).Scan(&id)
	if err != nil {
		return "", err
	}

	return id, nil
}
