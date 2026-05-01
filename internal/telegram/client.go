// Package telegram provides a thin client for the Telegram Bot API and an
// async Notifier that mirrors order_history inserts to a Telegram chat.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultServiceURL = "https://api.telegram.org"
	sendMessagePath   = "sendMessage"
	maxBodySize       = 1 << 20 // 1 MB
)

// APIError is returned by Client when the Telegram API responds with a non-2xx
// HTTP status. RetryAfter is non-zero only when the API returned 429 with
// parameters.retry_after.
type APIError struct {
	HTTPStatus  int
	Description string
	RetryAfter  time.Duration
}

func (e *APIError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("telegram: %s (status %d)", e.Description, e.HTTPStatus)
	}
	return fmt.Sprintf("telegram: status %d", e.HTTPStatus)
}

// Client posts to the Telegram Bot API.
type Client struct {
	apiKey     string
	serviceURL string
	httpClient *http.Client
}

// NewClient returns a production Client. Pass "" for serviceURL to use the
// default Telegram API URL. Tests construct *Client directly to inject a
// custom httpClient or serviceURL.
func NewClient(token, serviceURL string) *Client {
	url := serviceURL
	if url == "" {
		url = defaultServiceURL
	}
	return &Client{
		apiKey:     token,
		serviceURL: url,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

type sendMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type sendMessageResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// SendMessage POSTs to /bot<token>/sendMessage with a JSON {chat_id, text,
// parse_mode}. Non-2xx responses produce *APIError. Transport errors are
// wrapped plain. Pass "" for parseMode to send plain text (no parse_mode field).
func (c *Client) SendMessage(ctx context.Context, chatID, text, parseMode string) error {
	body, err := json.Marshal(sendMessageRequest{ChatID: chatID, Text: text, ParseMode: parseMode})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.serviceURL, "/") + "/bot" + c.apiKey + "/" + sendMessagePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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
	var parsed sendMessageResponse
	if jsonErr := json.Unmarshal(respBody, &parsed); jsonErr == nil {
		apiErr.Description = parsed.Description
		if resp.StatusCode == http.StatusTooManyRequests && parsed.Parameters.RetryAfter > 0 {
			apiErr.RetryAfter = time.Duration(parsed.Parameters.RetryAfter) * time.Second
		}
	}
	return apiErr
}
