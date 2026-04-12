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

		sd.CreateShop(t, "updshop1", admin.ID, map[string]string{"EN": "Original"}, nil)

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop1", shop.UpdateRequest{
			Titles: map[string]string{"EN": "Updated", "UK": "Оновлено"},
		})

		require.NoError(t, err)

		got := sd.GetShop(t, "updshop1")
		assert.Equal(t, "Updated", got.Titles["EN"])
		assert.Equal(t, "Оновлено", got.Titles["UK"])
	})

	main.Run("PartialUpsert", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "updshop2", admin.ID, map[string]string{"EN": "Original EN", "UK": "Original UK"}, nil)

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop2", shop.UpdateRequest{
			Titles: map[string]string{"EN": "Updated EN"},
		})

		require.NoError(t, err)

		got := sd.GetShop(t, "updshop2")
		assert.Equal(t, "Updated EN", got.Titles["EN"])
		assert.Equal(t, "Original UK", got.Titles["UK"])
	})

	main.Run("WithDescriptions", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "updshop4", admin.ID, map[string]string{"EN": "Desc Shop"}, nil)

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop4", shop.UpdateRequest{
			Titles:        map[string]string{"EN": "Desc Shop"},
			Descriptions: map[string]string{"EN": "A description"},
		})

		require.NoError(t, err)

		got := sd.GetShop(t, "updshop4")
		assert.Equal(t, "A description", got.Descriptions["EN"])
	})

	main.Run("DescriptionOnlyPreservesName", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "updshop5", admin.ID, map[string]string{"EN": "Keep Name"}, nil)

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop5", shop.UpdateRequest{
			Descriptions: map[string]string{"EN": "New desc"},
		})

		require.NoError(t, err)

		got := sd.GetShop(t, "updshop5")
		assert.Equal(t, "Keep Name", got.Titles["EN"])
		assert.Equal(t, "New desc", got.Descriptions["EN"])
	})

	main.Run("DescriptionOnlyForUnknownLangFails", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "updshop6", admin.ID, map[string]string{"EN": "Only EN"}, nil)

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop6", shop.UpdateRequest{
			Descriptions: map[string]string{"UK": "No name for uk"},
		})

		require.ErrorAs(t, err, new(*shop.InvalidLanguageError))
	})

	main.Run("ShopNotFound", func(t *testing.T) {
		t.Parallel()

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "nosuchshop", shop.UpdateRequest{
			Titles: map[string]string{"EN": "Test"},
		})

		require.ErrorIs(t, err, shop.ErrShopNotFound)
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		t.Parallel()

		sd.CreateShop(t, "updshop3", admin.ID, map[string]string{"EN": "Lang Test"}, nil)

		svc := shop.NewService(db, zerolog.Nop())
		err := svc.Update(t.Context(), "updshop3", shop.UpdateRequest{
			Titles: map[string]string{"xx": "Unknown"},
		})

		require.ErrorAs(t, err, new(*shop.InvalidLanguageError))
	})
}
