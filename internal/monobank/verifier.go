package monobank

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const pubkeyPath = "api/merchant/pubkey"

// ErrInvalidSignature is returned by Verifier.Verify when the X-Sign signature
// cannot be verified against the cached public key, even after one lazy
// refetch. Callers map this to HTTP 401.
var ErrInvalidSignature = errors.New("monobank: invalid signature")

// Verifier verifies Monobank webhook X-Sign headers. The merchant public key
// is fetched once at startup via Fetch and cached. On a verification failure,
// the key is refetched once before retrying — this auto-recovers from key
// rotation without operator intervention. Concurrent refetches are deduped
// via the refetching atomic flag.
type Verifier struct {
	apiKey     string
	httpClient *http.Client
	serviceURL string
	mu         sync.RWMutex
	pubKey     *ecdsa.PublicKey
	refetching atomic.Bool
}

// NewVerifier returns a production Verifier. Pass "" for serviceURL to use
// the default Monobank URL. Tests construct *Verifier directly to inject an
// httptest.Server URL via the unexported fields, mirroring Client.
func NewVerifier(apiKey, serviceURL string) *Verifier {
	url := serviceURL
	if url == "" {
		url = defaultServiceURL
	}
	return &Verifier{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		serviceURL: url,
	}
}

// Fetch pulls the merchant public key from /api/merchant/pubkey and caches
// it. Called once at startup so the app fails fast on misconfiguration.
func (v *Verifier) Fetch(ctx context.Context) error {
	pub, err := v.fetchKey(ctx)
	if err != nil {
		return err
	}
	v.mu.Lock()
	v.pubKey = pub
	v.mu.Unlock()
	return nil
}

// Verify checks signatureB64 (base64-encoded ASN.1 ECDSA, value of the X-Sign
// HTTP header) over body using the cached public key. On failure the key is
// refetched once and verification is retried. Returns nil on success,
// ErrInvalidSignature on persistent mismatch, or a wrapped error on transport
// or parse failure during the refetch.
func (v *Verifier) Verify(ctx context.Context, body []byte, signatureB64 string) error {
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return ErrInvalidSignature
	}
	hash := sha256.Sum256(body)

	v.mu.RLock()
	pub := v.pubKey
	v.mu.RUnlock()
	if pub != nil && ecdsa.VerifyASN1(pub, hash[:], sig) {
		return nil
	}

	// Refetch and retry once. Dedupe concurrent refetches: only the first
	// caller fetches; later callers wait for the first to finish, then retry
	// against the freshly-cached key.
	if v.refetching.CompareAndSwap(false, true) {
		defer v.refetching.Store(false)
		newPub, ferr := v.fetchKey(ctx)
		if ferr != nil {
			return ferr
		}
		v.mu.Lock()
		v.pubKey = newPub
		v.mu.Unlock()
		pub = newPub
	} else {
		// Another goroutine is refetching; spin briefly waiting for it.
		for v.refetching.Load() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
		}
		v.mu.RLock()
		pub = v.pubKey
		v.mu.RUnlock()
	}

	if pub != nil && ecdsa.VerifyASN1(pub, hash[:], sig) {
		return nil
	}
	return ErrInvalidSignature
}

func (v *Verifier) fetchKey(ctx context.Context) (*ecdsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(v.serviceURL, pubkeyPath), nil)
	if err != nil {
		return nil, fmt.Errorf("monobank pubkey: build request: %w", err)
	}
	req.Header.Set("X-Token", v.apiKey)
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("monobank pubkey: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("monobank pubkey: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("monobank pubkey: status %d: %s", resp.StatusCode, forensicBody(raw))
	}
	var responseBody struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(raw, &responseBody); err != nil {
		return nil, fmt.Errorf("monobank pubkey: parse body: %w", err)
	}
	pemBytes, err := base64.StdEncoding.DecodeString(responseBody.Key)
	if err != nil {
		return nil, fmt.Errorf("monobank pubkey: decode base64: %w", err)
	}
	block, _ := pem.Decode(bytes.TrimSpace(pemBytes))
	if block == nil {
		return nil, errors.New("monobank pubkey: no PEM block found")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("monobank pubkey: parse PKIX: %w", err)
	}
	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("monobank pubkey: unexpected key type %T", pub)
	}
	return ecdsaPub, nil
}
