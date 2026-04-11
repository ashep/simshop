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

		sd.CreateShop(t, "updshop1", admin.ID, map[string]string{"en": "Original"}, nil)

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

		sd.CreateShop(t, "updshop2", admin.ID, map[string]string{"en": "Original EN", "uk": "Original UK"}, nil)

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop2", shop.UpdateRequest{
			Names: map[string]string{"en": "Updated EN"},
		})

		require.NoError(t, err)

		got := sd.GetShop(t, "updshop2")
		assert.Equal(t, "Updated EN", got.Names["en"])
		assert.Equal(t, "Original UK", got.Names["uk"])
	})

	main.Run("WithDescriptions", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "updshop4", admin.ID, map[string]string{"en": "Desc Shop"}, nil)

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop4", shop.UpdateRequest{
			Names:        map[string]string{"en": "Desc Shop"},
			Descriptions: map[string]string{"en": "A description"},
		})

		require.NoError(t, err)

		got := sd.GetShop(t, "updshop4")
		assert.Equal(t, "A description", got.Descriptions["en"])
	})

	main.Run("DescriptionOnlyPreservesName", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "updshop5", admin.ID, map[string]string{"en": "Keep Name"}, nil)

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop5", shop.UpdateRequest{
			Descriptions: map[string]string{"en": "New desc"},
		})

		require.NoError(t, err)

		got := sd.GetShop(t, "updshop5")
		assert.Equal(t, "Keep Name", got.Names["en"])
		assert.Equal(t, "New desc", got.Descriptions["en"])
	})

	main.Run("DescriptionOnlyForUnknownLangFails", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "updshop6", admin.ID, map[string]string{"en": "Only EN"}, nil)

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop6", shop.UpdateRequest{
			Descriptions: map[string]string{"uk": "No name for uk"},
		})

		require.ErrorIs(t, err, shop.ErrInvalidLanguage)
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

		sd.CreateShop(t, "updshop3", admin.ID, map[string]string{"en": "Lang Test"}, nil)

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop3", shop.UpdateRequest{
			Names: map[string]string{"xx": "Unknown"},
		})

		require.ErrorIs(t, err, shop.ErrInvalidLanguage)
	})
}
