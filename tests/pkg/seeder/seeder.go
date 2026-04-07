package seeder

import (
	"testing"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/tests/pkg/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type Seeder struct {
	t  *testing.T
	db *pgxpool.Pool
}

func New(t *testing.T, db *pgxpool.Pool) *Seeder {
	return &Seeder{
		t:  t,
		db: db,
	}
}

func (s *Seeder) CreateUser(t *testing.T) *auth.User {
	key := testutil.RandStr(64)
	u := &auth.User{
		APIKey: key,
	}

	row := s.db.QueryRow(t.Context(), "INSERT INTO users VALUES (uuidv7(), $1) RETURNING id", key)
	require.NoError(t, row.Scan(&u.ID))

	return u
}
