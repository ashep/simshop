package googlesheets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/oauth2/google"

	"github.com/ashep/simshop/internal/order"
)

const (
	defaultServiceURL = "https://sheets.googleapis.com/v4/spreadsheets"
	maxBodySize       = 1 << 20                                        // 1 MB
	sheetsScope       = "https://www.googleapis.com/auth/spreadsheets" // read/write access to Google Sheets
)

// Client appends rows to a Google Sheet via the Sheets API v4.
type Client struct {
	httpClient    *http.Client
	serviceURL    string
	spreadsheetID string
	sheetName     string
}

// NewClient returns a production Client authenticated with a service account JSON key.
// Pass serviceURL="" to use the production Google Sheets API.
// Pass a non-empty serviceURL to bypass OAuth2 (used in tests only — set via config.GoogleSheets.ServiceURL).
// If both credentialsJSON and serviceURL are empty, a disabled client is returned that returns an error on Write.
func NewClient(credentialsJSON, spreadsheetID, sheetName, serviceURL string) (*Client, error) {
	baseURL := serviceURL
	if baseURL == "" {
		baseURL = defaultServiceURL
	}

	var httpClient *http.Client
	if serviceURL != "" {
		// Test mode: skip OAuth2 and point at the injected fake server.
		httpClient = &http.Client{Timeout: 5 * time.Second}
	} else if credentialsJSON == "" {
		// No credentials configured — return a disabled client that errors on Write.
		return &Client{
			serviceURL:    baseURL,
			spreadsheetID: spreadsheetID,
			sheetName:     sheetName,
		}, nil
	} else {
		conf, err := google.JWTConfigFromJSON([]byte(credentialsJSON), sheetsScope)
		if err != nil {
			return nil, fmt.Errorf("parse credentials: %w", err)
		}
		// context.Background() is intentional: the OAuth2 transport is long-lived and
		// must outlive any individual request. Token refresh calls use this context.
		httpClient = conf.Client(context.Background())
	}

	return &Client{
		httpClient:    httpClient,
		serviceURL:    baseURL,
		spreadsheetID: spreadsheetID,
		sheetName:     sheetName,
	}, nil
}

// Write appends a single row for o to the configured Google Sheet.
// Implements order.Writer.
func (c *Client) Write(ctx context.Context, o order.Order) error {
	if c.httpClient == nil {
		return fmt.Errorf("google sheets not configured: credentials_json is empty")
	}

	sheetName := c.sheetName
	if sheetName == "" {
		sheetName = "Sheet1"
	}
	sheetRange := sheetName + "!A1"

	row := []any{
		strconv.FormatInt(o.DateTime.Unix(), 16),
		"New",
		o.DateTime.Format("2006-01-02"),
		o.DateTime.Format("15:04:05"),
		o.ProductName,
		o.Attributes,
		fmt.Sprintf("%.2f %s", o.Price, o.Currency),
		o.Email,
		o.FirstName,
		o.MiddleName,
		o.LastName,
		o.Phone,
		o.City,
		o.Address,
		o.Notes,
	}

	body, err := json.Marshal(map[string]any{
		"values": [][]any{row},
	})
	if err != nil {
		return fmt.Errorf("marshal row: %w", err)
	}

	endpoint := fmt.Sprintf("%s/%s/values/%s:append?valueInputOption=RAW",
		c.serviceURL,
		url.PathEscape(c.spreadsheetID),
		url.PathEscape(sheetRange),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodySize))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return nil
}
