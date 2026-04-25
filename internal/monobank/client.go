// Package monobank is a thin client for the Monobank acquiring API.
package monobank

import "fmt"

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
