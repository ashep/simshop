package monobank

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWebhook(main *testing.T) {
	main.Run("Success", func(t *testing.T) {
		body := []byte(`{
			"invoiceId":"abc",
			"status":"success",
			"amount":12345,
			"ccy":980,
			"finalAmount":12345,
			"createdDate":"2026-04-26T10:00:00Z",
			"modifiedDate":"2026-04-26T10:05:00Z",
			"reference":"order-1"
		}`)
		got, err := ParseWebhook(body)
		require.NoError(t, err)
		assert.Equal(t, "abc", got.InvoiceID)
		assert.Equal(t, "success", got.Status)
		assert.Equal(t, 12345, got.Amount)
		assert.Equal(t, 980, got.Ccy)
		assert.Equal(t, 12345, got.FinalAmount)
		assert.Equal(t, "order-1", got.Reference)
		assert.Equal(t, time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC), got.CreatedDate)
		assert.Equal(t, time.Date(2026, 4, 26, 10, 5, 0, 0, time.UTC), got.ModifiedDate)
		assert.Equal(t, string(body), string(got.RawBody), "RawBody must equal input bytes byte-for-byte")
	})

	main.Run("Failure", func(t *testing.T) {
		body := []byte(`{"invoiceId":"abc","status":"failure","reference":"order-1","failureReason":"Limit exceeded","errCode":"LIMIT_EXCEEDED","amount":100,"ccy":980,"finalAmount":0,"createdDate":"2026-04-26T10:00:00Z","modifiedDate":"2026-04-26T10:05:00Z"}`)
		got, err := ParseWebhook(body)
		require.NoError(t, err)
		assert.Equal(t, "failure", got.Status)
		assert.Equal(t, "Limit exceeded", got.FailureReason)
		assert.Equal(t, "LIMIT_EXCEEDED", got.ErrCode)
	})

	main.Run("MalformedJSON", func(t *testing.T) {
		_, err := ParseWebhook([]byte(`not json`))
		assert.Error(t, err)
	})

	main.Run("MissingInvoiceID", func(t *testing.T) {
		_, err := ParseWebhook([]byte(`{"status":"success","reference":"order-1"}`))
		assert.Error(t, err)
	})

	main.Run("MissingStatus", func(t *testing.T) {
		_, err := ParseWebhook([]byte(`{"invoiceId":"abc","reference":"order-1"}`))
		assert.Error(t, err)
	})

	main.Run("MissingReference", func(t *testing.T) {
		_, err := ParseWebhook([]byte(`{"invoiceId":"abc","status":"success"}`))
		assert.Error(t, err)
	})
}
