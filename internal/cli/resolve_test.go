package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsFullUUID(main *testing.T) {
	main.Run("canonical uuid is full", func(t *testing.T) {
		assert.True(t, isFullUUID("019e9de8-c3c0-7000-8000-000000000001"))
	})

	main.Run("short prefix is not full", func(t *testing.T) {
		assert.False(t, isFullUUID("019e9de8-c3c0"))
	})

	main.Run("wrong length is not full", func(t *testing.T) {
		assert.False(t, isFullUUID("019e9de8-c3c0-7000-8000-00000000000")) // 35 chars
	})

	main.Run("misplaced hyphens are not full", func(t *testing.T) {
		assert.False(t, isFullUUID("019e9de8c3c0-7000-8000-0000000000011"))
	})

	main.Run("non-hex characters are not full", func(t *testing.T) {
		assert.False(t, isFullUUID("019e9de8-c3c0-7000-8000-00000000000z"))
	})
}

func TestMatchOrder(main *testing.T) {
	orders := []Order{
		{ID: "019e9de8-c3c0-7000-8000-000000000001"},
		{ID: "019e9de8-c3c0-7000-8000-000000000002"},
		{ID: "019e9df0-1111-7000-8000-000000000003"},
	}

	main.Run("exact full id matches", func(t *testing.T) {
		o, err := matchOrder(orders, "019e9de8-c3c0-7000-8000-000000000002")
		require.NoError(t, err)
		assert.Equal(t, "019e9de8-c3c0-7000-8000-000000000002", o.ID)
	})

	main.Run("unique prefix matches", func(t *testing.T) {
		o, err := matchOrder(orders, "019e9df0")
		require.NoError(t, err)
		assert.Equal(t, "019e9df0-1111-7000-8000-000000000003", o.ID)
	})

	main.Run("prefix is case-insensitive", func(t *testing.T) {
		o, err := matchOrder(orders, "019E9DF0")
		require.NoError(t, err)
		assert.Equal(t, "019e9df0-1111-7000-8000-000000000003", o.ID)
	})

	main.Run("ambiguous prefix errors", func(t *testing.T) {
		_, err := matchOrder(orders, "019e9de8-c3c0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ambiguous")
	})

	main.Run("no match errors not found", func(t *testing.T) {
		_, err := matchOrder(orders, "deadbeef")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	main.Run("exact match wins over being a prefix of another", func(t *testing.T) {
		short := []Order{{ID: "ab"}, {ID: "abc"}}
		o, err := matchOrder(short, "ab")
		require.NoError(t, err)
		assert.Equal(t, "ab", o.ID)
	})
}

func TestClientResolveOrderID(main *testing.T) {
	main.Run("full uuid is returned without a request", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Errorf("server must not be called for a full uuid")
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		id, err := NewClient(srv.URL, "k").ResolveOrderID(context.Background(), "019e9de8-c3c0-7000-8000-000000000001")
		require.NoError(t, err)
		assert.Equal(t, "019e9de8-c3c0-7000-8000-000000000001", id)
	})

	main.Run("short id resolves to the full id via the list", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`[{"id":"019e9de8-c3c0-7000-8000-000000000001"},{"id":"019e9df0-1111-7000-8000-000000000003"}]`))
		}))
		defer srv.Close()

		id, err := NewClient(srv.URL, "k").ResolveOrderID(context.Background(), "019e9df0")
		require.NoError(t, err)
		assert.Equal(t, "019e9df0-1111-7000-8000-000000000003", id)
	})
}
