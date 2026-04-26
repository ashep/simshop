package monobank

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// WebhookPayload is the parsed body of a Monobank invoice-status webhook.
// RawBody is the verbatim request body — byte-for-byte equal to the input
// passed to ParseWebhook — so callers can persist it as jsonb without
// re-encoding.
type WebhookPayload struct {
	InvoiceID     string
	Status        string // created | processing | hold | success | failure | reversed | expired
	FailureReason string // optional; set on status=failure
	ErrCode       string // optional; set on status=failure
	Amount        int    // minor units
	Ccy           int    // ISO 4217 numeric
	FinalAmount   int    // minor units
	CreatedDate   time.Time
	ModifiedDate  time.Time
	Reference     string // our orderID, set when invoice was created
	RawBody       json.RawMessage
}

type webhookBody struct {
	InvoiceID     string `json:"invoiceId"`
	Status        string `json:"status"`
	FailureReason string `json:"failureReason"`
	ErrCode       string `json:"errCode"`
	Amount        int    `json:"amount"`
	Ccy           int    `json:"ccy"`
	FinalAmount   int    `json:"finalAmount"`
	CreatedDate   string `json:"createdDate"`
	ModifiedDate  string `json:"modifiedDate"`
	Reference     string `json:"reference"`
}

// ParseWebhook decodes a Monobank webhook delivery body. Returns an error on
// malformed JSON or when any of invoiceId, status, or reference is missing.
// The returned WebhookPayload.RawBody equals body byte-for-byte.
func ParseWebhook(body []byte) (*WebhookPayload, error) {
	var raw webhookBody
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse webhook: %w", err)
	}
	if raw.InvoiceID == "" {
		return nil, errors.New("parse webhook: missing invoiceId")
	}
	if raw.Status == "" {
		return nil, errors.New("parse webhook: missing status")
	}
	if raw.Reference == "" {
		return nil, errors.New("parse webhook: missing reference")
	}
	created, err := parseWebhookTime(raw.CreatedDate)
	if err != nil {
		return nil, fmt.Errorf("parse webhook createdDate: %w", err)
	}
	modified, err := parseWebhookTime(raw.ModifiedDate)
	if err != nil {
		return nil, fmt.Errorf("parse webhook modifiedDate: %w", err)
	}
	return &WebhookPayload{
		InvoiceID:     raw.InvoiceID,
		Status:        raw.Status,
		FailureReason: raw.FailureReason,
		ErrCode:       raw.ErrCode,
		Amount:        raw.Amount,
		Ccy:           raw.Ccy,
		FinalAmount:   raw.FinalAmount,
		CreatedDate:   created,
		ModifiedDate:  modified,
		Reference:     raw.Reference,
		RawBody:       append(json.RawMessage(nil), body...),
	}, nil
}

func parseWebhookTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}
