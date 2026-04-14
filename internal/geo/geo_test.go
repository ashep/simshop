package geo

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDetector builds a Detector whose geo-service calls go to the given httptest.Server.
func newTestDetector(srv *httptest.Server) *Detector {
	return &Detector{
		cache:      make(map[string]entry),
		httpClient: srv.Client(),
		serviceURL: srv.URL + "/%s/country",
	}
}

func TestDetect(main *testing.T) {
	main.Run("HeaderPresent", func(t *testing.T) {
		// No server needed — header path makes no network call.
		d := &Detector{cache: make(map[string]entry)}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(countryHeader, "UA")
		assert.Equal(t, "ua", d.Detect(r))
	})

	main.Run("HeaderNormalisedToLower", func(t *testing.T) {
		d := &Detector{cache: make(map[string]entry)}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(countryHeader, "US")
		assert.Equal(t, "us", d.Detect(r))
	})

	main.Run("CacheHit", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("DE"))
		}))
		defer srv.Close()

		d := newTestDetector(srv)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "1.2.3.4:5678"

		// Seed the cache.
		d.mu.Lock()
		d.cache["1.2.3.4"] = entry{country: "de", expiresAt: time.Now().Add(time.Hour)}
		d.mu.Unlock()

		result := d.Detect(r)
		assert.Equal(t, "de", result)
		assert.Equal(t, int32(0), calls.Load(), "no HTTP call should be made on a cache hit")
	})

	main.Run("CacheMissCallsService", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("PL\n"))
		}))
		defer srv.Close()

		d := newTestDetector(srv)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "5.6.7.8:1234"

		result := d.Detect(r)
		require.Equal(t, "pl", result)
		assert.Equal(t, int32(1), calls.Load())

		// Second call must use cache.
		result2 := d.Detect(r)
		assert.Equal(t, "pl", result2)
		assert.Equal(t, int32(1), calls.Load(), "second call should hit cache")
	})

	main.Run("ExpiredCacheEntryRefetches", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("FR"))
		}))
		defer srv.Close()

		d := newTestDetector(srv)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "9.9.9.9:80"

		// Seed an expired cache entry.
		d.mu.Lock()
		d.cache["9.9.9.9"] = entry{country: "old", expiresAt: time.Now().Add(-time.Second)}
		d.mu.Unlock()

		result := d.Detect(r)
		assert.Equal(t, "fr", result)
		assert.Equal(t, int32(1), calls.Load(), "expired entry should trigger a fresh fetch")
	})

	main.Run("ServiceErrorReturnsEmpty", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		d := newTestDetector(srv)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "2.3.4.5:99"

		result := d.Detect(r)
		assert.Equal(t, "", result)
	})

	main.Run("ServiceErrorIsNotCached", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		d := newTestDetector(srv)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "3.4.5.6:99"

		_ = d.Detect(r)
		_ = d.Detect(r)
		assert.Equal(t, int32(2), calls.Load(), "service errors must not be cached — each call should retry")
	})

	main.Run("InvalidHeaderValueIgnored", func(t *testing.T) {
		d := &Detector{cache: make(map[string]entry)}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(countryHeader, "not-a-country-code")
		// Should fall through to IP-based detection, which returns "" for loopback with no service.
		// We just verify the invalid header value is not returned.
		result := d.Detect(r)
		assert.Equal(t, "", result, "invalid header value should be ignored; loopback with no service returns empty")
	})

	main.Run("XForwardedForUsedOverRemoteAddr", func(t *testing.T) {
		var gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("IT"))
		}))
		defer srv.Close()

		d := newTestDetector(srv)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "127.0.0.1:12345"
		r.Header.Set("X-Forwarded-For", "10.20.30.40, 192.168.1.1")

		result := d.Detect(r)
		assert.Equal(t, "it", result)
		assert.Contains(t, gotPath, "10.20.30.40", "should use first XFF IP, not RemoteAddr")
	})
}
