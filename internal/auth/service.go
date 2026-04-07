package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID     string
	APIKey string
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{
		db: db,
	}
}

func (s *Service) GetByAPIKey(ctx context.Context, key string) (*User, error) {
	var u User

	err := s.db.QueryRow(ctx, "SELECT id, api_key FROM users WHERE api_key = $1", key).Scan(&u.ID, &u.APIKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	} else if err != nil {
		return nil, err
	}

	return &u, nil
}
