// Package ganalytics sends server-side ecommerce events to Google Analytics 4
// through the Measurement Protocol, and provides an async Notifier that turns
// order status changes into `purchase` and `refund` events.
package ganalytics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultServiceURL = "https://www.google-analytics.com"
	collectPath       = "/mp/collect"
)

// APIError is returned by Client when the Measurement Protocol endpoint
// responds with a non-2xx HTTP status.
type APIError struct {
	HTTPStatus int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("google analytics: status %d", e.HTTPStatus)
}

// Payload is one Measurement Protocol request body.
type Payload struct {
	ClientID string  `json:"client_id"`
	Events   []Event `json:"events"`
}

// Event is one GA4 event inside a Payload.
type Event struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
}

// Item is one entry of an ecommerce event's `items` param.
type Item struct {
	ItemID      string  `json:"item_id"`
	ItemName    string  `json:"item_name"`
	ItemVariant string  `json:"item_variant,omitempty"`
	Price       float64 `json:"price"`
	Quantity    int     `json:"quantity"`
}

// Client posts events to the GA4 Measurement Protocol.
type Client struct {
	measurementID string
	apiSecret     string
	serviceURL    string
	httpClient    *http.Client
}

// NewClient returns a Client for the given GA4 measurement id ("G-XXXXXXX")
// and Measurement Protocol API secret. Pass "" for serviceURL to use Google's
// endpoint; tests pass an httptest server URL.
func NewClient(measurementID, apiSecret, serviceURL string) *Client {
	if serviceURL == "" {
		serviceURL = defaultServiceURL
	}
	return &Client{
		measurementID: measurementID,
		apiSecret:     apiSecret,
		serviceURL:    serviceURL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Send posts p. The endpoint validates nothing and answers 2xx even for
// malformed events, so a nil error only means the request was delivered.
func (c *Client) Send(ctx context.Context, p Payload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	q := url.Values{}
	q.Set("measurement_id", c.measurementID)
	q.Set("api_secret", c.apiSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.serviceURL+collectPath+"?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{HTTPStatus: resp.StatusCode}
	}
	return nil
}
