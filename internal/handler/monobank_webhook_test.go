package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/ashep/simshop/internal/monobank"
	"github.com/ashep/simshop/internal/order"
)

type monobankVerifierMock struct{ mock.Mock }

func (m *monobankVerifierMock) Verify(ctx context.Context, body []byte, sig string) error {
	return m.Called(ctx, body, sig).Error(0)
}

func TestMonobankStatusToInvoiceStatus(main *testing.T) {
	cases := []struct {
		in  string
		out string
		ok  bool
	}{
		{"created", "", false},
		{"processing", "processing", true},
		{"hold", "hold", true},
		{"success", "paid", true},
		{"failure", "failed", true},
		{"expired", "failed", true},
		{"reversed", "reversed", true},
		{"unknown", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		main.Run(c.in, func(t *testing.T) {
			got, ok := monobankStatusToInvoiceStatus(c.in)
			assert.Equal(t, c.out, got)
			assert.Equal(t, c.ok, ok)
		})
	}
}

func TestBuildWebhookNote(main *testing.T) {
	main.Run("Success", func(t *testing.T) {
		got := buildWebhookNote(&monobank.WebhookPayload{Status: "success", FinalAmount: 199900})
		assert.Equal(t, "monobank: success, finalAmount=199900", got)
	})
	main.Run("FailureWithCode", func(t *testing.T) {
		got := buildWebhookNote(&monobank.WebhookPayload{Status: "failure", ErrCode: "LIMIT_EXCEEDED", FailureReason: "Limit"})
		assert.Equal(t, "monobank: failure (LIMIT_EXCEEDED)", got)
	})
	main.Run("FailureWithoutCode", func(t *testing.T) {
		got := buildWebhookNote(&monobank.WebhookPayload{Status: "failure", FailureReason: "Limit"})
		assert.Equal(t, "monobank: failure (Limit)", got)
	})
	main.Run("Hold", func(t *testing.T) {
		got := buildWebhookNote(&monobank.WebhookPayload{Status: "hold"})
		assert.Equal(t, "monobank: hold", got)
	})
}

func TestMonobankWebhook(main *testing.T) {
	const orderID = "018f4e3a-0000-7000-8000-000000000099"
	bodyFor := func(status string) []byte {
		return []byte(`{"invoiceId":"inv-1","status":"` + status + `","reference":"` + orderID + `","amount":12345,"ccy":980,"finalAmount":12345,"createdDate":"2026-04-26T10:00:00Z","modifiedDate":"2026-04-26T10:05:00Z"}`)
	}
	doRequest := func(t *testing.T, h *Handler, body []byte, sig string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/monobank/webhook", bytes.NewReader(body))
		r.Header.Set("X-Sign", sig)
		w := httptest.NewRecorder()
		h.MonobankWebhook(w, r)
		return w
	}
	build := func(svc *orderServiceMock, ver *monobankVerifierMock) *Handler {
		return &Handler{orders: svc, mbVerifier: ver, l: zerolog.Nop()}
	}

	main.Run("BadSignatureReturns401", func(t *testing.T) {
		svc := &orderServiceMock{}
		ver := &monobankVerifierMock{}
		ver.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(monobank.ErrInvalidSignature)
		h := build(svc, ver)
		w := doRequest(t, h, bodyFor("success"), "garbage")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		ver.AssertExpectations(t)
		svc.AssertNotCalled(t, "RecordInvoiceEvent")
	})

	main.Run("MalformedJSONReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		ver := &monobankVerifierMock{}
		ver.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		h := build(svc, ver)
		w := doRequest(t, h, []byte(`not json`), "sig")
		assert.Equal(t, http.StatusBadRequest, w.Code)
		svc.AssertNotCalled(t, "RecordInvoiceEvent")
	})

	main.Run("UnknownReferenceReturns200", func(t *testing.T) {
		svc := &orderServiceMock{}
		ver := &monobankVerifierMock{}
		ver.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		svc.On("RecordInvoiceEvent", mock.Anything, mock.Anything).Return(order.ErrNotFound)
		h := build(svc, ver)
		w := doRequest(t, h, bodyFor("success"), "sig")
		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})

	main.Run("InformationalCreatedReturns200NoDBCall", func(t *testing.T) {
		svc := &orderServiceMock{}
		ver := &monobankVerifierMock{}
		ver.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		h := build(svc, ver)
		w := doRequest(t, h, bodyFor("created"), "sig")
		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertNotCalled(t, "RecordInvoiceEvent")
	})

	main.Run("ForwardSuccessRecordsEvent", func(t *testing.T) {
		svc := &orderServiceMock{}
		ver := &monobankVerifierMock{}
		body := bodyFor("success")
		eventAt, _ := time.Parse(time.RFC3339, "2026-04-26T10:05:00Z")
		ver.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		svc.On("RecordInvoiceEvent", mock.Anything, mock.MatchedBy(func(evt order.InvoiceEvent) bool {
			return evt.OrderID == orderID &&
				evt.InvoiceID == "inv-1" &&
				evt.Provider == "monobank" &&
				evt.Status == order.InvoiceStatusPaid &&
				evt.Note == "monobank: success, finalAmount=12345" &&
				evt.EventAt.Equal(eventAt) &&
				bytes.Equal(evt.Payload, body)
		})).Return(nil)
		h := build(svc, ver)
		w := doRequest(t, h, body, "sig")
		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})

	main.Run("ForwardFailureWithErrCodeNote", func(t *testing.T) {
		svc := &orderServiceMock{}
		ver := &monobankVerifierMock{}
		body := []byte(`{"invoiceId":"inv-1","status":"failure","reference":"` + orderID + `","errCode":"LIMIT_EXCEEDED","failureReason":"limit","amount":1,"ccy":980,"finalAmount":0,"createdDate":"2026-04-26T10:00:00Z","modifiedDate":"2026-04-26T10:05:00Z"}`)
		ver.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		svc.On("RecordInvoiceEvent", mock.Anything, mock.MatchedBy(func(evt order.InvoiceEvent) bool {
			return evt.Status == order.InvoiceStatusFailed &&
				evt.Note == "monobank: failure (LIMIT_EXCEEDED)"
		})).Return(nil)
		h := build(svc, ver)
		w := doRequest(t, h, body, "sig")
		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})

	main.Run("VerifierTransportErrorReturns500", func(t *testing.T) {
		svc := &orderServiceMock{}
		ver := &monobankVerifierMock{}
		ver.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("transport: connection refused"))
		h := build(svc, ver)
		w := doRequest(t, h, bodyFor("success"), "sig")
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		svc.AssertNotCalled(t, "RecordInvoiceEvent")
	})

	main.Run("DBErrorReturns500", func(t *testing.T) {
		svc := &orderServiceMock{}
		ver := &monobankVerifierMock{}
		ver.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		svc.On("RecordInvoiceEvent", mock.Anything, mock.Anything).Return(errors.New("db down"))
		h := build(svc, ver)
		w := doRequest(t, h, bodyFor("success"), "sig")
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("OversizeBodyTriggersJSONErrorReturning400", func(t *testing.T) {
		svc := &orderServiceMock{}
		ver := &monobankVerifierMock{}
		ver.On("Verify", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		h := build(svc, ver)
		// 1 MB + 1 byte; reader is capped at 1 MB so JSON parse of truncated payload fails → 400.
		big := bytes.Repeat([]byte("a"), 1<<20+1)
		req := httptest.NewRequest(http.MethodPost, "/monobank/webhook", io.NopCloser(bytes.NewReader(big)))
		req.Header.Set("X-Sign", "sig")
		w := httptest.NewRecorder()
		h.MonobankWebhook(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
