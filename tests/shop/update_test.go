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

func TestUpdate(main *testing.T) {
	main.Parallel()

	db := testpostgres.New(main, testpostgres.WithMigrations(
		dbmigrator.Source{FS: appsql.FS, Path: "."},
	)).DB()

	sd := seeder.New(main, db)
	admin := sd.GetAdminUser(main)

	main.Run("Success", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "updshop1", admin.ID, map[string]string{"en": "Original"})

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop1", shop.UpdateRequest{
			Names: map[string]string{"en": "Updated", "uk": "Оновлено"},
		})

		require.NoError(t, err)

		got := sd.GetShop(t, "updshop1")
		assert.Equal(t, "Updated", got.Names["en"])
		assert.Equal(t, "Оновлено", got.Names["uk"])
	})

	main.Run("PartialUpsert", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "updshop2", admin.ID, map[string]string{"en": "Original EN", "uk": "Original UK"})

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop2", shop.UpdateRequest{
			Names: map[string]string{"en": "Updated EN"},
		})

		require.NoError(t, err)

		got := sd.GetShop(t, "updshop2")
		assert.Equal(t, "Updated EN", got.Names["en"])
		assert.Equal(t, "Original UK", got.Names["uk"])
	})

	main.Run("ShopNotFound", func(t *testing.T) {
		t.Parallel()

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "nosuchshop", shop.UpdateRequest{
			Names: map[string]string{"en": "Test"},
		})

		require.ErrorIs(t, err, shop.ErrShopNotFound)
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "updshop3", admin.ID, map[string]string{"en": "Lang Test"})

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop3", shop.UpdateRequest{
			Names: map[string]string{"xx": "Unknown"},
		})

		require.ErrorIs(t, err, shop.ErrInvalidLanguage)
	})
}
