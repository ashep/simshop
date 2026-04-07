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

	main.Run("Success", func(t *testing.T) {
		t.Parallel()

		svc := shop.NewService(db, zerolog.Nop())
		got, err := svc.Create(t.Context(), shop.CreateRequest{
			ID:    "myshop",
			Names: map[string]string{"en": "My Shop"},
		})

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "myshop", got.ID)
		assert.Equal(t, map[string]string{"en": "My Shop"}, got.Names)

		dbShop := sd.GetShop(t, "myshop")
		assert.Equal(t, map[string]string{"en": "My Shop"}, dbShop.Names)
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		t.Parallel()

		svc := shop.NewService(db, zerolog.Nop())
		_, err := svc.Create(t.Context(), shop.CreateRequest{
			ID:    "langshop",
			Names: map[string]string{"xx": "Lang Shop"},
		})

		require.ErrorIs(t, err, shop.ErrInvalidLanguage)
	})

	main.Run("DuplicateID", func(t *testing.T) {
		t.Parallel()

		svc := shop.NewService(db, zerolog.Nop())
		_, err := svc.Create(t.Context(), shop.CreateRequest{
			ID:    "dupshop",
			Names: map[string]string{"en": "Dup Shop"},
		})
		require.NoError(t, err)

		_, err = svc.Create(t.Context(), shop.CreateRequest{
			ID:    "dupshop",
			Names: map[string]string{"en": "Dup Shop"},
		})
		require.Error(t, err)
	})
}
