package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OrderAttr is a rendered order attribute (title/value in the order's language).
type OrderAttr struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Price int    `json:"price"`
}

// OrderHistoryEntry is one order status-history row.
type OrderHistoryEntry struct {
	Status    string  `json:"status"`
	Note      *string `json:"note"`
	CreatedAt string  `json:"created_at"`
}

// OrderInvoice is one provider invoice attached to an order.
type OrderInvoice struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	PageURL  string `json:"page_url"`
	Amount   int    `json:"amount"`
	Currency string `json:"currency"`
}

// Order mirrors the API OrderRecord schema (read side).
type Order struct {
	ID             string              `json:"id"`
	ProductID      string              `json:"product_id"`
	Status         string              `json:"status"`
	Email          string              `json:"email"`
	Price          int                 `json:"price"`
	Currency       string              `json:"currency"`
	Lang           string              `json:"lang"`
	FirstName      string              `json:"first_name"`
	MiddleName     *string             `json:"middle_name"`
	LastName       string              `json:"last_name"`
	Country        string              `json:"country"`
	City           string              `json:"city"`
	Phone          string              `json:"phone"`
	Address        string              `json:"address"`
	AdminNote      *string             `json:"admin_note"`
	CustomerNote   *string             `json:"customer_note"`
	TrackingNumber *string             `json:"tracking_number"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
	Attrs          []OrderAttr         `json:"attrs"`
	History        []OrderHistoryEntry `json:"history"`
	Invoices       []OrderInvoice      `json:"invoices"`
}

// Client is a thin Bearer-authenticated HTTP client for the simshop API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewClient builds a client for baseURL authenticated with apiKey.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    http.DefaultClient,
	}
}

// ListOrders returns all orders, optionally filtered by status.
func (c *Client) ListOrders(ctx context.Context, statuses []string) ([]Order, error) {
	u := c.baseURL + "/orders"
	if len(statuses) > 0 {
		q := url.Values{}
		q.Set("status", strings.Join(statuses, ","))
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request orders: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return nil, apiError(resp)
	}
	var orders []Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("decode orders: %w", err)
	}
	return orders, nil
}

// GetOrder returns a single order by id (filters the list; no single-record endpoint exists).
func (c *Client) GetOrder(ctx context.Context, id string) (*Order, error) {
	orders, err := c.ListOrders(ctx, nil)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		if orders[i].ID == id {
			return &orders[i], nil
		}
	}
	return nil, fmt.Errorf("order %q not found", id)
}

// SetStatus updates an order's status and returns the resulting status.
func (c *Client) SetStatus(ctx context.Context, id, status, tracking, note string) (string, error) {
	payload := map[string]string{"status": status}
	if tracking != "" {
		payload["tracking_number"] = tracking
	}
	if note != "" {
		payload["note"] = note
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/orders/"+id+"/status", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("request status update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return "", apiError(resp)
	}
	var out struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return out.Status, nil
}

// apiError turns a non-2xx response into an error using the {"error": ...} body when present.
func apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var er struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &er) == nil && er.Error != "" {
		return fmt.Errorf("%s (HTTP %d)", er.Error, resp.StatusCode)
	}
	return fmt.Errorf("HTTP %d", resp.StatusCode)
}
