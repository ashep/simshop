package novaposhta

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
	defaultServiceURL = "https://api.novaposhta.ua/v2.0/json/"
	maxBodySize       = 1 << 20 // 1 MB
)

// City is a Nova Poshta settlement returned by SearchCities.
type City struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
}

// Branch is a Nova Poshta warehouse returned by SearchBranches.
type Branch struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
}

// Client calls the Nova Poshta JSON API v2.
type Client struct {
	apiKey     string
	httpClient *http.Client
	serviceURL string
}

// NewClient returns a production Client. Tests construct *Client directly to inject a test server.
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

type npRequest struct {
	APIKey           string         `json:"apiKey"`
	ModelName        string         `json:"modelName"`
	CalledMethod     string         `json:"calledMethod"`
	MethodProperties map[string]any `json:"methodProperties"`
}

type searchSettlementsResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		Addresses []struct {
			Ref     string `json:"Ref"`
			Present string `json:"Present"`
		} `json:"Addresses"`
	} `json:"data"`
}

type getWarehousesResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		Ref         string `json:"Ref"`
		Description string `json:"Description"`
	} `json:"data"`
}

func (c *Client) post(ctx context.Context, req npRequest) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serviceURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return data, nil
}

// SearchCities calls Address.searchSettlements and returns matching cities.
func (c *Client) SearchCities(ctx context.Context, query string) ([]City, error) {
	data, err := c.post(ctx, npRequest{
		APIKey:       c.apiKey,
		ModelName:    "Address",
		CalledMethod: "searchSettlements",
		MethodProperties: map[string]any{
			"CityName": query,
			"Limit":    20,
		},
	})
	if err != nil {
		return nil, err
	}

	var parsed searchSettlementsResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if !parsed.Success {
		return nil, fmt.Errorf("nova poshta api returned success=false")
	}

	cities := []City{}
	if len(parsed.Data) > 0 {
		for _, a := range parsed.Data[0].Addresses {
			cities = append(cities, City{Ref: a.Ref, Name: a.Present})
		}
	}
	return cities, nil
}

// SearchBranches calls AddressGeneral.getWarehouses and returns matching warehouses in the given city.
func (c *Client) SearchBranches(ctx context.Context, cityRef, query string) ([]Branch, error) {
	data, err := c.post(ctx, npRequest{
		APIKey:       c.apiKey,
		ModelName:    "AddressGeneral",
		CalledMethod: "getWarehouses",
		MethodProperties: map[string]any{
			"CityRef":      cityRef,
			"FindByString": query,
			"Limit":        20,
		},
	})
	if err != nil {
		return nil, err
	}

	var parsed getWarehousesResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if !parsed.Success {
		return nil, fmt.Errorf("nova poshta api returned success=false")
	}

	branches := []Branch{}
	for _, b := range parsed.Data {
		branches = append(branches, Branch{Ref: b.Ref, Name: b.Description})
	}
	return branches, nil
}
