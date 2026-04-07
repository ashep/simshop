//go:build functest

package testapp

import (
	"testing"
	"time"

	"github.com/ashep/go-app/runner"
	"github.com/ashep/go-app/testpostgres"
	"github.com/ashep/go-app/testrunner"
	"github.com/ashep/simshop/internal/app"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	*testrunner.Runner[func(*runner.Runtime[app.Config]) error, app.Config]
	pg *testpostgres.Postgres
}

func New(t *testing.T) *App {
	pg := testpostgres.New(t)
	cfg := app.Config{
		Debug: false,
		Database: app.Database{
			DSN: pg.DSN(),
		},
	}

	r := testrunner.New(t, app.Run, cfg).SetHTTPReadyStartWaiter("http://localhost:9000", time.Second)

	return &App{Runner: r, pg: pg}
}

func (a *App) DB() *pgxpool.Pool {
	return a.pg.DB()
}
