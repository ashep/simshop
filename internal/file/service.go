package file

import (
	"github.com/ashep/simshop/internal/app"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type Service struct {
	db  *pgxpool.Pool
	cfg app.Files
	l   zerolog.Logger
}

func NewService(db *pgxpool.Pool, cfg app.Files, l zerolog.Logger) *Service {
	return &Service{db: db, cfg: cfg, l: l}
}
