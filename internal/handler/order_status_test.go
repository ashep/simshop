package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/order"
)

func TestGetOrderStatus(main *testing.T) {
	resp := buildTestResponder(main)

	const orderID = "018f4e3a-0000-7000-8000-000000000099"

	doRequest := func(t *testing.T, svc *orderServiceMock, id string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{orders: svc, geo: &geoDetectorStub{}, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/orders/"+id, nil)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		h.GetOrderStatus(w, r)
		return w
	}

	main.Run("Found", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("GetStatus", mock.Anything, orderID).Return("payment_processing", nil)

		w := doRequest(t, svc, orderID)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"status":"payment_processing"}`, w.Body.String())
	})

	main.Run("NotFound", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("GetStatus", mock.Anything, orderID).Return("", order.ErrNotFound)

		w := doRequest(t, svc, orderID)
		assert.Equal(t, http.StatusNotFound, w.Code)
		var body map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "order not found", body["error"])
	})

	main.Run("ServiceErrorReturns500", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("GetStatus", mock.Anything, orderID).Return("", errors.New("boom"))

		w := doRequest(t, svc, orderID)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
