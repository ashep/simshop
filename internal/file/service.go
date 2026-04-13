package file

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type Service struct {
	db             *pgxpool.Pool
	maxNumPerUser  int
	l              zerolog.Logger
}

func NewService(db *pgxpool.Pool, maxNumPerUser int, l zerolog.Logger) *Service {
	return &Service{db: db, maxNumPerUser: maxNumPerUser, l: l}
}
