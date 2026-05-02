// Package resend provides a thin wrapper around the Resend transactional
// email API and an async Notifier that mirrors customer-facing order status
// changes to email.
package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServiceURL = "https://api.resend.com"
	sendEmailPath     = "/emails"
	maxBodySize       = 1 << 20 // 1 MB
)

// APIError is returned by Client when the Resend API responds with a non-2xx
// HTTP status. RetryAfter is non-zero only when the API returned 429 with a
// Retry-After header (or equivalent body field).
type APIError struct {
	HTTPStatus int
	Message    string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("resend: %s (status %d)", e.Message, e.HTTPStatus)
	}
	return fmt.Sprintf("resend: status %d", e.HTTPStatus)
}

// Email is a single transactional email payload. From, To, Subject are
// required; HTML and Text are both populated by the notifier (Resend's
// deliverability guidance recommends sending both).
type Email struct {
	From    string
	To      string
	Subject string
	HTML    string
	Text    string
}

// Client posts to the Resend HTTP API.
//
// We don't use the resend-go SDK directly because (a) the SDK does not expose
// a base-URL override needed for httptest, and (b) we only need one endpoint.
// The SDK is still vendored — Task 3 added it — so future expansion can use
// it. For now, the wire format is simple JSON and matches what the SDK would
// send.
type Client struct {
	apiKey     string
	serviceURL string
	httpClient *http.Client
}

// NewClient returns a Client. Pass "" for serviceURL to use the default
// Resend API URL. Tests inject httptest.Server.URL via the serviceURL
// argument.
func NewClient(apiKey, serviceURL string) *Client {
	url := serviceURL
	if url == "" {
		url = defaultServiceURL
	}
	return &Client{
		apiKey:     apiKey,
		serviceURL: url,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

type sendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
}

type sendEmailErrorResponse struct {
	StatusCode int    `json:"statusCode"`
	Name       string `json:"name"`
	Message    string `json:"message"`
}

// SendEmail POSTs a single email to /emails with the standard Resend JSON
// envelope. Non-2xx responses produce *APIError. Transport errors are wrapped
// plain so callers can distinguish them via errors.As.
func (c *Client) SendEmail(ctx context.Context, e Email) error {
	body, err := json.Marshal(sendEmailRequest{
		From: e.From, To: []string{e.To}, Subject: e.Subject,
		HTML: e.HTML, Text: e.Text,
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.serviceURL, "/") + sendEmailPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	apiErr := &APIError{HTTPStatus: resp.StatusCode}
	var parsed sendEmailErrorResponse
	if jsonErr := json.Unmarshal(respBody, &parsed); jsonErr == nil {
		apiErr.Message = parsed.Message
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		if v := resp.Header.Get("Retry-After"); v != "" {
			if secs, perr := strconv.Atoi(strings.TrimSpace(v)); perr == nil && secs > 0 {
				apiErr.RetryAfter = time.Duration(secs) * time.Second
			}
		}
	}
	return apiErr
}
