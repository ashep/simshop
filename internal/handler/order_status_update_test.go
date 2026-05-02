package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/order"
)

func TestUpdateOrderStatus(main *testing.T) {
	resp := buildTestResponder(main)
	const orderID = "018f4e3a-0000-7000-8000-000000000099"

	doRequest := func(t *testing.T, svc *orderServiceMock, body string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{orders: svc, geo: &geoDetectorStub{}, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/orders/"+orderID+"/status", strings.NewReader(body))
		r.SetPathValue("id", orderID)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.UpdateOrderStatus(w, r)
		return w
	}

	main.Run("OKAppliedTransition", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("UpdateStatus", mock.Anything, orderID, "processing", "starting", "").
			Return(true, nil)

		w := doRequest(t, svc, `{"status":"processing","note":"starting"}`)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"status":"processing"}`, w.Body.String())
	})

	main.Run("OKShippedWithTracking", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("UpdateStatus", mock.Anything, orderID, "shipped", "", "TRK-XYZ").
			Return(true, nil)

		w := doRequest(t, svc, `{"status":"shipped","tracking_number":"TRK-XYZ"}`)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"status":"shipped"}`, w.Body.String())
	})

	main.Run("OKIdempotentSameStatus", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("UpdateStatus", mock.Anything, orderID, "shipped", "", "TRK-XYZ").
			Return(false, nil)

		w := doRequest(t, svc, `{"status":"shipped","tracking_number":"TRK-XYZ"}`)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"status":"shipped"}`, w.Body.String())
	})

	main.Run("BadJSON", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)

		w := doRequest(t, svc, `not json`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var body map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "invalid request body", body["error"])
		svc.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything)
	})

	main.Run("UnknownStatus", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)

		w := doRequest(t, svc, `{"status":"unicorn"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		svc.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything)
	})

	main.Run("ShippedMissingTracking", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)

		w := doRequest(t, svc, `{"status":"shipped"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var body map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "tracking_number required", body["error"])
		svc.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything)
	})

	main.Run("TrackingOnNonShipped", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)

		w := doRequest(t, svc, `{"status":"delivered","tracking_number":"TRK-XYZ"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var body map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "tracking_number only valid for shipped", body["error"])
		svc.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything)
	})

	main.Run("OversizeNote", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)

		big := strings.Repeat("x", 501)
		w := doRequest(t, svc, `{"status":"processing","note":"`+big+`"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		svc.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything)
	})

	main.Run("OversizeTrackingNumber", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)

		big := strings.Repeat("y", 65)
		w := doRequest(t, svc, `{"status":"shipped","tracking_number":"`+big+`"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		svc.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything)
	})

	main.Run("NotFound", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("UpdateStatus", mock.Anything, orderID, "processing", "", "").
			Return(false, order.ErrNotFound)

		w := doRequest(t, svc, `{"status":"processing"}`)
		assert.Equal(t, http.StatusNotFound, w.Code)
		var body map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "order not found", body["error"])
	})

	main.Run("Conflict", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("UpdateStatus", mock.Anything, orderID, "delivered", "", "").
			Return(false, order.ErrTransitionNotAllowed)

		w := doRequest(t, svc, `{"status":"delivered"}`)
		assert.Equal(t, http.StatusConflict, w.Code)
		var body map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "transition not allowed", body["error"])
	})

	main.Run("ServiceErrorReturns500", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("UpdateStatus", mock.Anything, orderID, "processing", "", "").
			Return(false, errors.New("boom"))

		w := doRequest(t, svc, `{"status":"processing"}`)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
