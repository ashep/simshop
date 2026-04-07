//go:build functest

package auth_test

import (
	"testing"

	"github.com/ashep/go-app/dbmigrator"
	"github.com/ashep/go-app/testpostgres"
	"github.com/ashep/simshop/internal/auth"
	appsql "github.com/ashep/simshop/internal/sql"
	"github.com/ashep/simshop/tests/pkg/seeder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserByApiKey(main *testing.T) {
	main.Parallel()

	db := testpostgres.New(main, testpostgres.WithMigrations(
		dbmigrator.Source{FS: appsql.FS, Path: "."},
	)).DB()

	sd := seeder.New(main, db)
	user1 := sd.CreateUser(main)

	main.Run("Success", func(t *testing.T) {
		t.Parallel()
		svc := auth.NewService(db)

		res, err := svc.GetUserByAPIKey(t.Context(), user1.APIKey)
		require.NoError(t, err)
		assert.NotEmpty(t, res.ID)
		assert.Equal(t, user1.ID, res.ID)
		assert.Equal(t, user1.APIKey, res.APIKey)
	})

	main.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		svc := auth.NewService(db)

		_, err := svc.GetUserByAPIKey(t.Context(), "NonExistentAPIKey")
		assert.ErrorIs(t, err, auth.ErrUserNotFound)
	})
}
