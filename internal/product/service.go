package product

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type Service struct {
	db *pgxpool.Pool
	l  zerolog.Logger
}

func NewService(db *pgxpool.Pool, l zerolog.Logger) *Service {
	return &Service{
		db: db,
		l:  l,
	}
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*Product, error) {
	return nil, nil
}
