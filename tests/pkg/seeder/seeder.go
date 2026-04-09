//go:build functest

package seeder

import (
	"testing"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/shop"
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

func (s *Seeder) AddUserScope(t *testing.T, u *auth.User, scope auth.Scope) {
	_, err := s.db.Exec(t.Context(), "INSERT INTO user_permissions (user_id, scope) VALUES ($1, $2)", u.ID, scope)
	require.NoError(t, err)
	u.Scopes = append(u.Scopes, scope)
}

func (s *Seeder) CreateShop(t *testing.T, id string, names map[string]string) *shop.Shop {
	t.Helper()
	_, err := s.db.Exec(t.Context(), "INSERT INTO shops (id) VALUES ($1)", id)
	require.NoError(t, err)
	for lang, name := range names {
		_, err = s.db.Exec(t.Context(),
			"INSERT INTO shop_names (shop_id, lang_id, name) VALUES ($1, $2, $3)",
			id, lang, name,
		)
		require.NoError(t, err)
	}
	return s.GetShop(t, id)
}

func (s *Seeder) GetShop(t *testing.T, id string) *shop.Shop {
	t.Helper()
	sh := &shop.Shop{ID: id, Names: map[string]string{}}

	var count int
	err := s.db.QueryRow(t.Context(), "SELECT COUNT(*) FROM shops WHERE id = $1", id).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "shop %q not found in db", id)

	rows, err := s.db.Query(t.Context(), "SELECT lang_id, name FROM shop_names WHERE shop_id = $1", id)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var lang, name string
		require.NoError(t, rows.Scan(&lang, &name))
		sh.Names[lang] = name
	}
	require.NoError(t, rows.Err())

	return sh
}

func (s *Seeder) GetAdminUser(t *testing.T) *auth.User {
	t.Helper()
	u := &auth.User{}
	err := s.db.QueryRow(t.Context(),
		`SELECT u.id, u.api_key FROM users u
		JOIN user_permissions up ON up.user_id = u.id
		WHERE up.scope = 'admin' LIMIT 1`,
	).Scan(&u.ID, &u.APIKey)
	require.NoError(t, err)
	u.Scopes = []auth.Scope{auth.ScopeAdmin}
	return u
}
