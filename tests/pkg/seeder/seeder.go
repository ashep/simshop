//go:build functest

package seeder

import (
	"testing"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/product"
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

func (s *Seeder) CreateShop(t *testing.T, id string, ownerID string, names map[string]string, descriptions map[string]string) *shop.Shop {
	t.Helper()
	_, err := s.db.Exec(t.Context(), "INSERT INTO shops (id, owner_id) VALUES ($1, $2)", id, ownerID)
	require.NoError(t, err)
	for lang, name := range names {
		var desc *string
		if d, ok := descriptions[lang]; ok {
			desc = &d
		}
		_, err = s.db.Exec(t.Context(),
			"INSERT INTO shop_data (shop_id, lang_id, name, description) VALUES ($1, $2, $3, $4)",
			id, lang, name, desc,
		)
		require.NoError(t, err)
	}
	return s.GetShop(t, id)
}

func (s *Seeder) GetShop(t *testing.T, id string) *shop.Shop {
	t.Helper()
	sh := &shop.Shop{ID: id, Names: map[string]string{}, Descriptions: map[string]string{}}

	var count int
	err := s.db.QueryRow(t.Context(), "SELECT COUNT(*) FROM shops WHERE id = $1", id).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "shop %q not found in db", id)

	rows, err := s.db.Query(t.Context(), "SELECT lang_id, name, description FROM shop_data WHERE shop_id = $1", id)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var lang, name string
		var desc *string
		require.NoError(t, rows.Scan(&lang, &name, &desc))
		sh.Names[lang] = name
		if desc != nil {
			sh.Descriptions[lang] = *desc
		}
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

func (s *Seeder) CreateProduct(t *testing.T, shopID string, prices map[string]int, content map[string]product.ContentItem) *product.Product {
	t.Helper()

	var productID string
	row := s.db.QueryRow(t.Context(), "INSERT INTO products (shop_id) VALUES ($1) RETURNING id", shopID)
	require.NoError(t, row.Scan(&productID))

	for countryID, value := range prices {
		_, err := s.db.Exec(t.Context(),
			"INSERT INTO product_prices (product_id, country_id, value) VALUES ($1, $2, $3)",
			productID, countryID, value,
		)
		require.NoError(t, err)
	}

	for lang, c := range content {
		_, err := s.db.Exec(t.Context(),
			"INSERT INTO product_content (product_id, lang_id, title, description) VALUES ($1, $2, $3, $4)",
			productID, lang, c.Title, c.Description,
		)
		require.NoError(t, err)
	}

	return &product.Product{ID: productID}
}

func (s *Seeder) GetProduct(t *testing.T, id string) *product.Product {
	t.Helper()

	var count int
	err := s.db.QueryRow(t.Context(), "SELECT COUNT(*) FROM products WHERE id = $1", id).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "product %q not found in db", id)

	return &product.Product{ID: id}
}
