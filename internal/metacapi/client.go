// Package metacapi sends server-side events to the Meta (Facebook) Conversions
// API, and provides an async Notifier that reports paid orders as `Purchase`.
package metacapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultServiceURL = "https://graph.facebook.com"
	// graphVersion is the pinned Graph API version. Meta retires versions about two
	// years after release; bump it before then.
	graphVersion = "v25.0"
	maxBodySize  = 1 << 20 // 1 MB
)

// APIError is returned by Client when the Graph API responds with a non-2xx
// HTTP status. Message is Graph's error.message, when it sent one.
type APIError struct {
	HTTPStatus int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("meta conversions api: %s (status %d)", e.Message, e.HTTPStatus)
	}
	return fmt.Sprintf("meta conversions api: status %d", e.HTTPStatus)
}

// Event is one Conversions API server event.
type Event struct {
	EventName    string     `json:"event_name"`
	EventTime    int64      `json:"event_time"`
	EventID      string     `json:"event_id,omitempty"`
	ActionSource string     `json:"action_source"`
	UserData     UserData   `json:"user_data"`
	CustomData   CustomData `json:"custom_data"`
}

// UserData carries the buyer matching keys. List fields are SHA-256 hex of the
// normalized value; fbp/fbc/IP/User-Agent are sent as-is, per Meta's spec.
type UserData struct {
	Email           []string `json:"em,omitempty"`
	Phone           []string `json:"ph,omitempty"`
	FirstName       []string `json:"fn,omitempty"`
	LastName        []string `json:"ln,omitempty"`
	City            []string `json:"ct,omitempty"`
	Country         []string `json:"country,omitempty"`
	ExternalID      []string `json:"external_id,omitempty"`
	FBP             string   `json:"fbp,omitempty"`
	FBC             string   `json:"fbc,omitempty"`
	ClientIPAddress string   `json:"client_ip_address,omitempty"`
	ClientUserAgent string   `json:"client_user_agent,omitempty"`
}

// CustomData carries the purchase details.
type CustomData struct {
	Currency    string   `json:"currency"`
	Value       float64  `json:"value"`
	ContentIDs  []string `json:"content_ids,omitempty"`
	ContentType string   `json:"content_type,omitempty"`
	OrderID     string   `json:"order_id,omitempty"`
	NumItems    int      `json:"num_items,omitempty"`
}

type sendRequest struct {
	Data          []Event `json:"data"`
	AccessToken   string  `json:"access_token"`
	TestEventCode string  `json:"test_event_code,omitempty"`
}

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Client posts events to one pixel's Conversions API endpoint.
type Client struct {
	pixelID       string
	accessToken   string
	testEventCode string
	serviceURL    string
	httpClient    *http.Client
}

// NewClient returns a Client for the given pixel. testEventCode routes events to
// Events Manager's "Test events" tab instead of live data; pass "" in production.
// Pass "" for serviceURL to use graph.facebook.com.
func NewClient(pixelID, accessToken, testEventCode, serviceURL string) *Client {
	if serviceURL == "" {
		serviceURL = defaultServiceURL
	}
	return &Client{
		pixelID:       pixelID,
		accessToken:   accessToken,
		testEventCode: testEventCode,
		serviceURL:    serviceURL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Send posts events. The access token goes in the body, not the URL, so it
// never lands in proxy or error logs.
func (c *Client) Send(ctx context.Context, events []Event) error {
	body, err := json.Marshal(sendRequest{Data: events, AccessToken: c.accessToken, TestEventCode: c.testEventCode})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s/%s/events", c.serviceURL, graphVersion, c.pixelID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var er errorResponse
		_ = json.Unmarshal(respBody, &er)
		return &APIError{HTTPStatus: resp.StatusCode, Message: er.Error.Message}
	}
	return nil
}
