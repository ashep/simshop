package file

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type Service struct {
	db            *pgxpool.Pool
	publicDir     string
	maxNumPerUser int
	l             zerolog.Logger
}

func NewService(db *pgxpool.Pool, publicDir string, maxNumPerUser int, l zerolog.Logger) *Service {
	return &Service{db: db, publicDir: publicDir, maxNumPerUser: maxNumPerUser, l: l}
}
