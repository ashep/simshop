//go:build functest

package testapp

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ashep/go-app/runner"
	"github.com/ashep/go-app/testrunner"
	"github.com/ashep/simshop/internal/app"
)

type App struct {
	*testrunner.Runner[func(*runner.Runtime[app.Config]) error, app.Config]
	addr string
}

// New creates a test app instance.
func New(t *testing.T, dataDir string, opts ...func(*app.Config)) *App {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("listen: %s", err))
	}

	addr := lis.Addr().String()
	if err = lis.Close(); err != nil {
		panic(fmt.Sprintf("close listener: %s", err))
	}

	cfg := app.Config{
		Server: app.Server{
			Addr: addr,
		},
		DataDir: dataDir,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	r := testrunner.New(t, app.Run, cfg).SetHTTPReadyStartWaiter("http://"+addr, time.Second)

	return &App{Runner: r, addr: addr}
}

func (a *App) URL(path string) string {
	return "http://" + a.addr + path
}
