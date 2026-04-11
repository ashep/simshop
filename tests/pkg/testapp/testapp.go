//go:build functest

package testapp

import (
	"fmt"
	"net"
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
	pg   *testpostgres.Postgres
	addr string
}

func New(t *testing.T) *App {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("listen: %s", err))
	}

	addr := lis.Addr().String()
	pg := testpostgres.New(t)
	cfg := app.Config{
		Debug: false,
		Database: app.Database{
			DSN: pg.DSN(),
		},
		Server: app.Server{
			Addr: addr,
		},
	}

	r := testrunner.New(t, app.Run, cfg).SetHTTPReadyStartWaiter("http://"+addr, time.Second)

	return &App{Runner: r, pg: pg, addr: addr}
}

func (a *App) URL(path string) string {
	return "http://" + a.addr + path
}

func (a *App) DB() *pgxpool.Pool {
	return a.pg.DB()
}
