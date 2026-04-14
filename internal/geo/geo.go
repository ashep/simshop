package geo

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	countryHeader = "CF-IPCountry"
	geoServiceURL = "https://ipinfo.io/%s/country"
	cacheTTL      = time.Hour
)

type entry struct {
	country   string
	expiresAt time.Time
}

type Detector struct {
	mu         sync.Mutex
	cache      map[string]entry
	httpClient *http.Client
	serviceURL string
}

func NewDetector() *Detector {
	return &Detector{
		cache:      make(map[string]entry),
		httpClient: &http.Client{Timeout: 5 * time.Second},
		serviceURL: geoServiceURL,
	}
}

func (d *Detector) Detect(r *http.Request) string {
	if v := r.Header.Get(countryHeader); isAlpha2(v) {
		return strings.ToLower(v)
	}

	ip := clientIP(r)
	if ip == "" {
		return ""
	}

	d.mu.Lock()
	if e, ok := d.cache[ip]; ok && time.Now().Before(e.expiresAt) {
		d.mu.Unlock()
		return e.country
	}
	d.mu.Unlock()

	country := d.lookup(ip)

	if country != "" {
		d.mu.Lock()
		d.cache[ip] = entry{country: country, expiresAt: time.Now().Add(cacheTTL)}
		d.mu.Unlock()
	}

	return country
}

func (d *Detector) lookup(ip string) string {
	url := fmt.Sprintf(d.serviceURL, ip)
	resp, err := d.httpClient.Get(url) //nolint:noctx // request context not forwarded intentionally; geo lookup runs independently of request lifecycle
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16))
	if err != nil {
		return ""
	}

	return strings.ToLower(strings.TrimSpace(string(body)))
}

// isAlpha2 returns true when s is exactly two ASCII letters (ISO 3166-1 alpha-2).
func isAlpha2(s string) bool {
	return len(s) == 2 && (s[0] >= 'A' && s[0] <= 'Z' || s[0] >= 'a' && s[0] <= 'z') &&
		(s[1] >= 'A' && s[1] <= 'Z' || s[1] >= 'a' && s[1] <= 'z')
}

// clientIP extracts the client IP for geo-lookup. X-Forwarded-For is used only
// when the CF-IPCountry header is absent; in normal deployment Cloudflare is the
// trusted front-end and XFF is not reached. Without Cloudflare, XFF is caller-
// controlled and can be spoofed.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}

	if net.ParseIP(host) != nil {
		return host
	}

	return ""
}
