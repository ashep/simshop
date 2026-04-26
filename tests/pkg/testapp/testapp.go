//go:build functest

package testapp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ashep/go-app/runner"
	"github.com/ashep/go-app/testrunner"
	"github.com/ashep/simshop/internal/app"
)

// DefaultDSN is used when APP_DB_DSN is not set. Matches the docker-compose.tests.yaml network.
const DefaultDSN = "postgres://postgres:postgres@postgres:5432/postgres?sslmode=disable"

type App struct {
	*testrunner.Runner[func(*runner.Runtime[app.Config]) error, app.Config]
	addr string
	dsn  string
}

// New creates a test app instance. The database DSN is read from APP_DB_DSN
// (defaulting to DefaultDSN). Tests can override it via opts.
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

	dsn := os.Getenv("APP_DB_DSN")
	if dsn == "" {
		dsn = DefaultDSN
	}

	cfg := app.Config{
		Server: app.Server{
			Addr: addr,
		},
		DataDir: dataDir,
		Database: app.DBConfig{
			DSN: dsn,
		},
	}

	if v := os.Getenv("APP_MONOBANK_API_KEY"); v != "" {
		cfg.Monobank.APIKey = v
	} else {
		cfg.Monobank.APIKey = "test-key"
	}
	cfg.Monobank.RedirectURL = "https://test.example/thanks"
	cfg.Monobank.WebhookURL = "https://test.example/monobank/webhook"

	// Start a default stub Monobank server so Verifier.Fetch succeeds at app startup.
	// Tests that need custom Monobank behaviour override cfg.Monobank.ServiceURL via opts.
	priv, privErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if privErr != nil {
		panic(fmt.Sprintf("generate ecdsa key: %s", privErr))
	}
	der, derErr := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if derErr != nil {
		panic(fmt.Sprintf("marshal pubkey: %s", derErr))
	}
	pubKeyPayload := []byte(`{"key":"` + base64.StdEncoding.EncodeToString(
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}),
	) + `"}`)
	defaultMBSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/merchant/pubkey" {
			_, _ = w.Write(pubKeyPayload)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"invoiceId":"inv-default","pageUrl":"https://pay.example/inv-default"}`))
	}))
	t.Cleanup(defaultMBSrv.Close)
	cfg.Monobank.ServiceURL = defaultMBSrv.URL

	for _, opt := range opts {
		opt(&cfg)
	}

	r := testrunner.New(t, app.Run, cfg).SetHTTPReadyStartWaiter("http://"+addr, time.Second)

	return &App{Runner: r, addr: addr, dsn: cfg.Database.DSN}
}

func (a *App) URL(path string) string {
	return "http://" + a.addr + path
}

// DSN returns the postgres DSN this app instance is configured with.
func (a *App) DSN() string { return a.dsn }
