package monobank

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodePEM(t *testing.T, pub *ecdsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func signBody(t *testing.T, priv *ecdsa.PrivateKey, body []byte) string {
	t.Helper()
	h := sha256.Sum256(body)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, h[:])
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(sig)
}

func newTestVerifier(srv *httptest.Server) *Verifier {
	return &Verifier{
		apiKey:     "test-key",
		httpClient: srv.Client(),
		serviceURL: srv.URL,
	}
}

func TestVerifier(main *testing.T) {
	main.Run("FetchOK", func(t *testing.T) {
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/merchant/pubkey", r.URL.Path)
			assert.Equal(t, "test-key", r.Header.Get("X-Token"))
			_, _ = w.Write([]byte(`{"key":"` + base64.StdEncoding.EncodeToString(encodePEM(t, &priv.PublicKey)) + `"}`))
		}))
		defer srv.Close()
		v := newTestVerifier(srv)
		require.NoError(t, v.Fetch(context.Background()))
	})

	main.Run("FetchNon200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		v := newTestVerifier(srv)
		assert.Error(t, v.Fetch(context.Background()))
	})

	main.Run("FetchBadPEM", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"key":"bm90LXBlbQ=="}`)) // base64 of "not-pem"
		}))
		defer srv.Close()
		v := newTestVerifier(srv)
		assert.Error(t, v.Fetch(context.Background()))
	})

	main.Run("VerifyValidSignature", func(t *testing.T) {
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"key":"` + base64.StdEncoding.EncodeToString(encodePEM(t, &priv.PublicKey)) + `"}`))
		}))
		defer srv.Close()
		v := newTestVerifier(srv)
		require.NoError(t, v.Fetch(context.Background()))
		body := []byte(`{"hello":"world"}`)
		assert.NoError(t, v.Verify(context.Background(), body, signBody(t, priv, body)))
	})

	main.Run("VerifyInvalidSignatureRefetches", func(t *testing.T) {
		privA, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		privB, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			pub := &privA.PublicKey
			if hits.Add(1) > 1 {
				pub = &privB.PublicKey
			}
			_, _ = w.Write([]byte(`{"key":"` + base64.StdEncoding.EncodeToString(encodePEM(t, pub)) + `"}`))
		}))
		defer srv.Close()
		v := newTestVerifier(srv)
		require.NoError(t, v.Fetch(context.Background()))

		// Body signed with key B; cached key is A; verifier refetches and retries.
		body := []byte(`{"x":1}`)
		assert.NoError(t, v.Verify(context.Background(), body, signBody(t, privB, body)))
		assert.GreaterOrEqual(t, hits.Load(), int32(2), "verifier should refetch on first failure")
	})

	main.Run("VerifyPersistentMismatch", func(t *testing.T) {
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"key":"` + base64.StdEncoding.EncodeToString(encodePEM(t, &priv.PublicKey)) + `"}`))
		}))
		defer srv.Close()
		v := newTestVerifier(srv)
		require.NoError(t, v.Fetch(context.Background()))
		body := []byte(`{"x":1}`)
		err = v.Verify(context.Background(), body, signBody(t, other, body))
		assert.ErrorIs(t, err, ErrInvalidSignature)
	})

	main.Run("VerifyRefetchTransportError", func(t *testing.T) {
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if hits.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"key":"` + base64.StdEncoding.EncodeToString(encodePEM(t, &priv.PublicKey)) + `"}`))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		v := newTestVerifier(srv)
		require.NoError(t, v.Fetch(context.Background()))
		body := []byte(`{"x":1}`)
		err = v.Verify(context.Background(), body, signBody(t, other, body))
		require.Error(t, err)
		assert.False(t, errors.Is(err, ErrInvalidSignature), "transport errors must not collapse to ErrInvalidSignature")
	})
}
