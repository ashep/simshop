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
	user2 := sd.CreateUser(main)
	sd.AddUserScope(main, user2, "read")
	sd.AddUserScope(main, user2, "write")

	main.Run("Success", func(t *testing.T) {
		t.Parallel()
		svc := auth.NewService(db)

		res, err := svc.GetUserByAPIKey(t.Context(), user1.APIKey)
		require.NoError(t, err)
		assert.Equal(t, user1.ID, res.ID)
		assert.Equal(t, user1.APIKey, res.APIKey)
		assert.Empty(t, res.Scopes)
	})

	main.Run("SuccessWithScopes", func(t *testing.T) {
		t.Parallel()
		svc := auth.NewService(db)

		res, err := svc.GetUserByAPIKey(t.Context(), user2.APIKey)
		require.NoError(t, err)
		assert.Equal(t, user2.ID, res.ID)
		assert.Equal(t, user2.APIKey, res.APIKey)
		assert.ElementsMatch(t, user2.Scopes, res.Scopes)
	})

	main.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		svc := auth.NewService(db)

		_, err := svc.GetUserByAPIKey(t.Context(), "NonExistentAPIKey")
		assert.ErrorIs(t, err, auth.ErrUserNotFound)
	})
}
