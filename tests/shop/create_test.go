//go:build functest

package shop_test

import (
	"testing"

	"github.com/ashep/go-app/dbmigrator"
	"github.com/ashep/go-app/testpostgres"
	"github.com/ashep/simshop/internal/shop"
	appsql "github.com/ashep/simshop/internal/sql"
	"github.com/ashep/simshop/tests/pkg/seeder"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreate(main *testing.T) {
	main.Parallel()

	db := testpostgres.New(main, testpostgres.WithMigrations(
		dbmigrator.Source{FS: appsql.FS, Path: "."},
	)).DB()

	sd := seeder.New(main, db)
	admin := sd.GetAdminUser(main)

	main.Run("Success", func(t *testing.T) {
		t.Parallel()

		svc := shop.NewService(db, zerolog.Nop())
		got, err := svc.Create(t.Context(), shop.CreateRequest{
			ID:      "myshop",
			Titles:   map[string]string{"EN": "My Shop"},
			OwnerID: admin.ID,
		})

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "myshop", got.ID)
		assert.Equal(t, map[string]string{"EN": "My Shop"}, got.Titles)

		dbShop := sd.GetShop(t, "myshop")
		assert.Equal(t, map[string]string{"EN": "My Shop"}, dbShop.Titles)
	})

	main.Run("WithDescriptions", func(t *testing.T) {
		t.Parallel()

		svc := shop.NewService(db, zerolog.Nop())
		got, err := svc.Create(t.Context(), shop.CreateRequest{
			ID:           "descshop",
			Titles:        map[string]string{"EN": "Desc Shop"},
			Descriptions: map[string]string{"EN": "A shop with a description"},
			OwnerID:      admin.ID,
		})

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "A shop with a description", got.Descriptions["EN"])

		dbShop := sd.GetShop(t, "descshop")
		assert.Equal(t, "A shop with a description", dbShop.Descriptions["EN"])
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		t.Parallel()

		svc := shop.NewService(db, zerolog.Nop())
		_, err := svc.Create(t.Context(), shop.CreateRequest{
			ID:      "langshop",
			Titles:   map[string]string{"xx": "Lang Shop"},
			OwnerID: admin.ID,
		})

		require.ErrorIs(t, err, shop.ErrInvalidLanguage)
	})

	main.Run("DuplicateID", func(t *testing.T) {
		t.Parallel()

		svc := shop.NewService(db, zerolog.Nop())
		_, err := svc.Create(t.Context(), shop.CreateRequest{
			ID:      "dupshop",
			Titles:   map[string]string{"EN": "Dup Shop"},
			OwnerID: admin.ID,
		})
		require.NoError(t, err)

		_, err = svc.Create(t.Context(), shop.CreateRequest{
			ID:      "dupshop",
			Titles:   map[string]string{"EN": "Dup Shop"},
			OwnerID: admin.ID,
		})
		require.Error(t, err)
	})
}
