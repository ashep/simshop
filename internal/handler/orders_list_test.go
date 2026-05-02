package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/order"
)

func TestListOrders(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, svc *orderServiceMock, query string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{orders: svc, geo: &geoDetectorStub{}, resp: resp, l: zerolog.Nop()}
		target := "/orders"
		if query != "" {
			target += "?" + query
		}
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		h.ListOrders(w, r)
		return w
	}

	main.Run("EmptyListReturnsEmptyArray", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("List", mock.Anything, ([]string)(nil)).Return([]order.Record{}, nil)

		w := doRequest(t, svc, "")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `[]`, w.Body.String())
	})

	main.Run("PopulatedListReturnsRecords", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)

		middle := "Q"
		note := "ok"
		hist := "noted"
		records := []order.Record{
			{
				ID:           "018f4e3a-0000-7000-8000-000000000099",
				ProductID:    "widget",
				Status:       "new",
				Email:        "alice@example.com",
				Price:        12345,
				Currency:     "USD",
				Lang:         "uk",
				FirstName:    "Alice",
				MiddleName:   &middle,
				LastName:     "Smith",
				Country:      "ua",
				City:         "Kyiv",
				Phone:        "+380",
				Address:      "addr",
				CustomerNote: &note,
				CreatedAt:    time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
				UpdatedAt:    time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
				Attrs: []order.Attr{
					{Name: "Display color", Value: "Red", Price: 100},
				},
				History: []order.HistoryEntry{
					{
						ID:        "018f4e3a-0000-7000-8000-0000000000aa",
						Status:    "new",
						Note:      &hist,
						CreatedAt: time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
					},
				},
				Invoices: []order.Invoice{},
			},
		}
		svc.On("List", mock.Anything, ([]string)(nil)).Return(records, nil)

		w := doRequest(t, svc, "")
		assert.Equal(t, http.StatusOK, w.Code)

		var got []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Len(t, got, 1)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000099", got[0]["id"])
		assert.Equal(t, "widget", got[0]["product_id"])
		assert.Equal(t, "new", got[0]["status"])
		assert.Equal(t, "uk", got[0]["lang"])
		assert.Equal(t, "Q", got[0]["middle_name"])
		assert.Equal(t, "ok", got[0]["customer_note"])
		assert.Equal(t, float64(12345), got[0]["price"])

		attrs, ok := got[0]["attrs"].([]any)
		require.True(t, ok)
		require.Len(t, attrs, 1)
		assert.Equal(t, "Display color", attrs[0].(map[string]any)["name"])

		history, ok := got[0]["history"].([]any)
		require.True(t, ok)
		require.Len(t, history, 1)
		assert.Equal(t, "noted", history[0].(map[string]any)["note"])
	})

	main.Run("NullableFieldsAreOmittedWhenNil", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		records := []order.Record{
			{
				ID:        "018f4e3a-0000-7000-8000-0000000000bb",
				ProductID: "widget",
				Status:    "new",
				Email:     "a@b.com",
				Price:     100,
				Currency:  "USD",
				Lang:      "en",
				FirstName: "A",
				LastName:  "B",
				Country:   "ua",
				City:      "C",
				Phone:     "1",
				Address:   "D",
				CreatedAt: time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
				Attrs:     []order.Attr{},
				History:   []order.HistoryEntry{},
				Invoices:  []order.Invoice{},
			},
		}
		svc.On("List", mock.Anything, ([]string)(nil)).Return(records, nil)

		w := doRequest(t, svc, "")
		assert.Equal(t, http.StatusOK, w.Code)

		var got []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Len(t, got, 1)
		_, hasMiddle := got[0]["middle_name"]
		assert.False(t, hasMiddle, "middle_name should be omitted when nil")
		_, hasAdmin := got[0]["admin_note"]
		assert.False(t, hasAdmin, "admin_note should be omitted when nil")
		_, hasCustomer := got[0]["customer_note"]
		assert.False(t, hasCustomer, "customer_note should be omitted when nil")
	})

	main.Run("ServiceErrorReturns500", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("List", mock.Anything, ([]string)(nil)).Return(([]order.Record)(nil), errors.New("boom"))

		w := doRequest(t, svc, "")
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("StatusParamNotForwardedWhenAbsent", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("List", mock.Anything, ([]string)(nil)).Return([]order.Record{}, nil)

		w := doRequest(t, svc, "")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	main.Run("StatusSingleValueForwarded", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("List", mock.Anything, []string{"paid"}).Return([]order.Record{}, nil)

		w := doRequest(t, svc, "status=paid")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	main.Run("StatusCSVMultiValueForwarded", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("List", mock.Anything, []string{"paid", "shipped"}).Return([]order.Record{}, nil)

		w := doRequest(t, svc, "status=paid,shipped")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	main.Run("StatusEmptyValueTreatedAsNoFilter", func(t *testing.T) {
		// Pins the handler-level fallback: parseStatusFilter("") → nil → no filter.
		// In production, ?status= never reaches the handler — the kin-openapi
		// middleware decodes [""] and rejects it with 400 (covered in
		// tests/api/order/get_test.go::StatusFilterEmptyValueReturns400). This
		// unit test exercises the handler in isolation.
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("List", mock.Anything, ([]string)(nil)).Return([]order.Record{}, nil)

		w := doRequest(t, svc, "status=")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	main.Run("StatusWhitespaceTrimmedAndEmptiesDropped", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("List", mock.Anything, []string{"paid", "shipped"}).Return([]order.Record{}, nil)

		// " paid , , shipped " → ["paid","shipped"]
		w := doRequest(t, svc, "status=%20paid%20,%20,%20shipped%20")
		assert.Equal(t, http.StatusOK, w.Code)
	})

}
