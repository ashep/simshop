// Package cloudflare provides a thin GraphQL client for Cloudflare Web
// Analytics (RUM) page-load data.
package cloudflare

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
	defaultServiceURL = "https://api.cloudflare.com/client/v4"
	graphqlPath       = "/graphql"
	maxBodySize       = 8 << 20 // 8 MB
	dailyVisitsLimit  = 100
)

// GraphQL queries. Field names are kept here, in one place, so they are easy
// to adjust against the live schema.
const (
	dailyVisitsQuery = `query ($accountTag: String!, $siteTag: String!, $from: Time!, $to: Time!, $limit: Int!) {
  viewer {
    accounts(filter: {accountTag: $accountTag}) {
      rumPageloadEventsAdaptiveGroups(
        filter: {AND: [{datetime_geq: $from, datetime_lt: $to}, {siteTag: $siteTag}]}
        orderBy: [date_ASC]
        limit: $limit
      ) {
        dimensions { date }
        sum { visits }
      }
    }
  }
}`

	pathViewsQuery = `query ($accountTag: String!, $siteTag: String!, $from: Time!, $to: Time!, $limit: Int!) {
  viewer {
    accounts(filter: {accountTag: $accountTag}) {
      rumPageloadEventsAdaptiveGroups(
        filter: {AND: [{datetime_geq: $from, datetime_lt: $to}, {siteTag: $siteTag}]}
        orderBy: [count_DESC]
        limit: $limit
      ) {
        dimensions { requestPath }
        count
      }
    }
  }
}`
)

// APIError is returned when the API responds with a non-2xx status (Messages
// empty or parsed from the body) or with GraphQL-level errors (Status is the
// HTTP status, usually 200).
type APIError struct {
	Status   int
	Messages []string
}

func (e *APIError) Error() string {
	if len(e.Messages) > 0 {
		return fmt.Sprintf("cloudflare: %s (status %d)", strings.Join(e.Messages, "; "), e.Status)
	}
	return fmt.Sprintf("cloudflare: status %d", e.Status)
}

// DayVisits is the number of visits on one UTC day.
type DayVisits struct {
	Date   string // "2006-01-02" (UTC)
	Visits int
}

// PathViews is the number of page views of one request path.
type PathViews struct {
	Path  string
	Views int
}

// Client posts to the Cloudflare GraphQL Analytics API.
type Client struct {
	token      string
	accountID  string
	siteTag    string
	serviceURL string
	httpClient *http.Client
}

// NewClient returns a Client. Pass "" for serviceURL to use the real API.
func NewClient(token, accountID, siteTag, serviceURL string) *Client {
	if serviceURL == "" {
		serviceURL = defaultServiceURL
	}
	return &Client{
		token:      token,
		accountID:  accountID,
		siteTag:    siteTag,
		serviceURL: serviceURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type gqlResponse struct {
	Data struct {
		Viewer struct {
			Accounts []struct {
				Groups []struct {
					Dimensions struct {
						Date        string `json:"date"`
						RequestPath string `json:"requestPath"`
					} `json:"dimensions"`
					Count int `json:"count"`
					Sum   struct {
						Visits int `json:"visits"`
					} `json:"sum"`
				} `json:"rumPageloadEventsAdaptiveGroups"`
			} `json:"accounts"`
		} `json:"viewer"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// DailyVisits returns visits per UTC day in [from, to), ordered by date.
func (c *Client) DailyVisits(ctx context.Context, from, to time.Time) ([]DayVisits, error) {
	resp, err := c.query(ctx, dailyVisitsQuery, from, to, dailyVisitsLimit)
	if err != nil {
		return nil, err
	}
	out := []DayVisits{}
	for _, a := range resp.Data.Viewer.Accounts {
		for _, g := range a.Groups {
			out = append(out, DayVisits{Date: g.Dimensions.Date, Visits: g.Sum.Visits})
		}
	}
	return out, nil
}

// PathViews returns the most viewed request paths in [from, to), most viewed
// first.
func (c *Client) PathViews(ctx context.Context, from, to time.Time, limit int) ([]PathViews, error) {
	resp, err := c.query(ctx, pathViewsQuery, from, to, limit)
	if err != nil {
		return nil, err
	}
	out := []PathViews{}
	for _, a := range resp.Data.Viewer.Accounts {
		for _, g := range a.Groups {
			out = append(out, PathViews{Path: g.Dimensions.RequestPath, Views: g.Count})
		}
	}
	return out, nil
}

func (c *Client) query(ctx context.Context, query string, from, to time.Time, limit int) (*gqlResponse, error) {
	body, err := json.Marshal(map[string]any{
		"query": query,
		"variables": map[string]any{
			"accountTag": c.accountID,
			"siteTag":    c.siteTag,
			"from":       from.UTC().Format(time.RFC3339),
			"to":         to.UTC().Format(time.RFC3339),
			"limit":      limit,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.serviceURL, "/") + graphqlPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		apiErr := &APIError{Status: httpResp.StatusCode}
		var parsed gqlResponse
		if json.Unmarshal(respBody, &parsed) == nil {
			for _, e := range parsed.Errors {
				apiErr.Messages = append(apiErr.Messages, e.Message)
			}
		}
		return nil, apiErr
	}

	var parsed gqlResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		apiErr := &APIError{Status: httpResp.StatusCode}
		for _, e := range parsed.Errors {
			apiErr.Messages = append(apiErr.Messages, e.Message)
		}
		return nil, apiErr
	}
	return &parsed, nil
}
