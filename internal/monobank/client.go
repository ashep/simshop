// Package monobank is a thin client for the Monobank acquiring API.
package monobank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultServiceURL = "https://api.monobank.ua/"
	createInvoicePath = "api/merchant/invoice/create"
	maxBodySize       = 1 << 20 // 1 MB
	maxBodyForensic   = 4096
)

// CreateInvoiceRequest is the application-level input for CreateInvoice.
type CreateInvoiceRequest struct {
	Amount           int
	Ccy              int
	MerchantPaymInfo MerchantPaymInfo
	RedirectURL      string
}

// MerchantPaymInfo is the merchant-side metadata Monobank attaches to the invoice.
// BasketOrder must be non-empty: Monobank rejects invoices without it as
// `INVALID_MERCHANT_PAYM_INFO` because the field is mandatory for fiscalization.
type MerchantPaymInfo struct {
	Reference   string // our order id
	Destination string // shown in the bank app
	BasketOrder []BasketItem
}

// BasketItem is a single line item in MerchantPaymInfo.BasketOrder. The sum is
// the total for this line in minor units (not the unit price); the sum of all
// BasketItem.Sum values must equal the invoice's Amount. Code is the merchant's
// internal SKU; Monobank rejects items without it as `INVALID_MERCHANT_PAYM_INFO`
// when fiscalization is enabled on the merchant account. Tax is the list of
// tax-registration IDs (merchant-specific, configured in the Monobank business
// cabinet) — also required when fiscalization is enabled.
type BasketItem struct {
	Name string // human-readable, shown on the fiscal receipt
	Qty  int    // quantity
	Sum  int    // total for this line in minor units (kopecks/cents)
	Code string // merchant SKU / internal product code
	Tax  []int  // merchant-side tax registration IDs
}

// CreateInvoiceResponse is the parsed Monobank response.
type CreateInvoiceResponse struct {
	InvoiceID string
	PageURL   string
}

// APIError is returned by the client when the Monobank API responds with an
// application-level error or a non-2xx HTTP status. Callers extract structured
// fields with errors.As to log forensically without leaking detail in
// user-facing responses.
type APIError struct {
	Status  int    // HTTP status if applicable (0 if N/A)
	ErrCode string // Monobank "errCode" field if present
	ErrText string // Monobank "errText" field if present
	Body    string // up to 4096 bytes of the response body for forensics (truncated)
}

func (e *APIError) Error() string {
	if e.ErrCode != "" {
		return fmt.Sprintf("monobank: %s (%s)", e.ErrCode, e.ErrText)
	}
	if e.Status != 0 {
		return fmt.Sprintf("monobank: status %d", e.Status)
	}
	return "monobank: api error"
}

// Client calls the Monobank acquiring API.
type Client struct {
	apiKey     string
	httpClient *http.Client
	serviceURL string
}

// NewClient returns a production Client. Pass "" for serviceURL to use the
// default Monobank URL. Tests construct *Client directly to inject a test server.
func NewClient(apiKey, serviceURL string) *Client {
	url := serviceURL
	if url == "" {
		url = defaultServiceURL
	}
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		serviceURL: url,
	}
}

type createInvoiceBody struct {
	Amount           int                     `json:"amount"`
	Ccy              int                     `json:"ccy"`
	MerchantPaymInfo merchantPaymInfoPayload `json:"merchantPaymInfo"`
	RedirectURL      string                  `json:"redirectUrl,omitempty"`
}

type merchantPaymInfoPayload struct {
	Reference   string              `json:"reference,omitempty"`
	Destination string              `json:"destination,omitempty"`
	BasketOrder []basketItemPayload `json:"basketOrder,omitempty"`
}

type basketItemPayload struct {
	Name string `json:"name"`
	Qty  int    `json:"qty"`
	Sum  int    `json:"sum"`
	Code string `json:"code"`
	Tax  []int  `json:"tax"`
}

type createInvoiceResponseBody struct {
	InvoiceID string `json:"invoiceId"`
	PageURL   string `json:"pageUrl"`
	ErrCode   string `json:"errCode"`
	ErrText   string `json:"errText"`
}

// CreateInvoice issues POST /api/merchant/invoice/create and returns the
// parsed invoiceId/pageUrl. Application-level and HTTP errors are wrapped in
// *APIError so handlers can extract structured fields with errors.As.
func (c *Client) CreateInvoice(ctx context.Context, req CreateInvoiceRequest) (*CreateInvoiceResponse, error) {
	basket := make([]basketItemPayload, len(req.MerchantPaymInfo.BasketOrder))
	for i, b := range req.MerchantPaymInfo.BasketOrder {
		basket[i] = basketItemPayload{Name: b.Name, Qty: b.Qty, Sum: b.Sum, Code: b.Code, Tax: b.Tax}
	}
	payload := createInvoiceBody{
		Amount: req.Amount,
		Ccy:    req.Ccy,
		MerchantPaymInfo: merchantPaymInfoPayload{
			Reference:   req.MerchantPaymInfo.Reference,
			Destination: req.MerchantPaymInfo.Destination,
			BasketOrder: basket,
		},
		RedirectURL: req.RedirectURL,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(c.serviceURL, createInvoicePath), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Token", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if readErr != nil {
		return nil, fmt.Errorf("read body: %w", readErr)
	}

	// Try to parse the body for errCode/errText regardless of status — Monobank
	// returns the structured error envelope on both 200 (application errors) and
	// non-2xx (validation/auth errors). Parse failures on a non-2xx response are
	// expected (the body may be HTML/plain text); fall through to the bare-status
	// APIError in that case.
	var parsed createInvoiceResponseBody
	parseErr := json.Unmarshal(raw, &parsed)

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{
			Status:  resp.StatusCode,
			ErrCode: parsed.ErrCode,
			ErrText: parsed.ErrText,
			Body:    forensicBody(raw),
		}
	}

	if parseErr != nil {
		return nil, fmt.Errorf("parse response: %w", parseErr)
	}
	if parsed.ErrCode != "" {
		return nil, &APIError{Status: resp.StatusCode, ErrCode: parsed.ErrCode, ErrText: parsed.ErrText, Body: forensicBody(raw)}
	}
	if parsed.InvoiceID == "" || parsed.PageURL == "" {
		return nil, errors.New("monobank: empty invoiceId or pageUrl in response")
	}
	return &CreateInvoiceResponse{InvoiceID: parsed.InvoiceID, PageURL: parsed.PageURL}, nil
}

func joinURL(base, path string) string {
	if strings.HasSuffix(base, "/") {
		return base + path
	}
	return base + "/" + path
}

func forensicBody(b []byte) string {
	if len(b) > maxBodyForensic {
		return string(b[:maxBodyForensic])
	}
	return string(b)
}
