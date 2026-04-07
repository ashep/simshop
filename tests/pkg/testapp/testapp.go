package testapp

import (
	"testing"
	"time"

	"github.com/ashep/go-app/runner"
	"github.com/ashep/go-app/testpostgres"
	"github.com/ashep/go-app/testrunner"
	"github.com/ashep/simshop/internal/app"
)

func New(t *testing.T) *testrunner.Runner[func(*runner.Runtime[app.Config]) error, app.Config] {
	cfg := app.Config{
		Debug: false,
		Database: app.Database{
			DSN: testpostgres.New(t).DSN(),
		},
	}

	return testrunner.New(t, app.Run, cfg).SetHTTPReadyStartWaiter("http://localhost:9000", time.Second)
}
