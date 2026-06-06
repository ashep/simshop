package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveStatusFilter(main *testing.T) {
	main.Run("no flag defaults to active statuses", func(t *testing.T) {
		assert.Equal(t, activeStatuses, resolveStatusFilter(nil))
		assert.Equal(t, activeStatuses, resolveStatusFilter([]string{}))
	})

	main.Run("all means no filter", func(t *testing.T) {
		assert.Nil(t, resolveStatusFilter([]string{"all"}))
	})

	main.Run("all wins even mixed with other values", func(t *testing.T) {
		assert.Nil(t, resolveStatusFilter([]string{"processing", "all"}))
	})

	main.Run("explicit statuses pass through unchanged", func(t *testing.T) {
		in := []string{"processing", "shipped"}
		assert.Equal(t, in, resolveStatusFilter(in))
	})

	main.Run("active set excludes terminal statuses", func(t *testing.T) {
		for _, terminal := range []string{"delivered", "cancelled", "returned", "refunded"} {
			assert.NotContains(t, activeStatuses, terminal)
		}
	})
}

func TestOrderListStatusWiring(main *testing.T) {
	run := func(t *testing.T, args ...string) string {
		t.Helper()
		var gotStatus string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotStatus = r.URL.Query().Get("status")
			_, _ = w.Write([]byte(`[]`))
		}))
		t.Cleanup(srv.Close)

		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		require.NoError(t, os.WriteFile(cfgPath,
			[]byte("s1:\n  url: "+srv.URL+"\n  api_key: k1\n"), 0o600))

		root := NewRootCmd()
		root.SetArgs(append([]string{"--config", cfgPath, "order", "list"}, args...))
		require.NoError(t, root.Execute())
		return gotStatus
	}

	main.Run("no flag sends the active status set", func(t *testing.T) {
		assert.Equal(t, "new,awaiting_payment,payment_processing,payment_hold,paid,processing,shipped,refund_requested", run(t))
	})

	main.Run("--status all sends no filter", func(t *testing.T) {
		assert.Equal(t, "", run(t, "--status", "all"))
	})

	main.Run("explicit --status is forwarded", func(t *testing.T) {
		assert.Equal(t, "processing,shipped", run(t, "--status", "processing,shipped"))
	})
}

func TestOrderSetStatusShortID(main *testing.T) {
	main.Run("resolves a short id to the full id before patching", func(t *testing.T) {
		const fullID = "019e9df0-1111-7000-8000-000000000003"
		var patchedPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(`[{"id":"019e9de8-c3c0-7000-8000-000000000001"},{"id":"` + fullID + `"}]`))
			case http.MethodPatch:
				patchedPath = r.URL.Path
				_, _ = w.Write([]byte(`{"status":"shipped"}`))
			}
		}))
		t.Cleanup(srv.Close)

		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		require.NoError(t, os.WriteFile(cfgPath,
			[]byte("s1:\n  url: "+srv.URL+"\n  api_key: k1\n"), 0o600))

		root := NewRootCmd()
		root.SetArgs([]string{"--config", cfgPath, "order", "set-status", "019e9df0", "shipped", "--tracking", "TRK1"})
		require.NoError(t, root.Execute())
		assert.Equal(t, "/orders/"+fullID+"/status", patchedPath)
	})
}
