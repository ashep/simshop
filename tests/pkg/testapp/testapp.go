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

// New creates a test app with a temporary public dir.
func New(t *testing.T, dataDir string) *App {
	return NewWithPublicDir(t, dataDir, t.TempDir())
}

// NewWithPublicDir creates a test app with explicit public and data dirs.
// Use this when you need to pre-populate the public dir with binary files.
func NewWithPublicDir(t *testing.T, dataDir, publicDir string) *App {
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
			Addr:      addr,
			PublicDir: publicDir,
		},
		DataDir: dataDir,
	}

	r := testrunner.New(t, app.Run, cfg).SetHTTPReadyStartWaiter("http://"+addr, time.Second)

	return &App{Runner: r, addr: addr}
}

func (a *App) URL(path string) string {
	return "http://" + a.addr + path
}
