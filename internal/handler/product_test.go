package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/internal/product"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type productServiceMock struct {
	mock.Mock
}

func (m *productServiceMock) List(ctx context.Context) ([]*product.Product, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*product.Product), args.Error(1)
}

func TestListProducts(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, prodSvc *productServiceMock) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{prod: prodSvc, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products", nil)
		w := httptest.NewRecorder()
		h.ListProducts(w, r)
		return w
	}

	main.Run("EmptyList", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("List", mock.Anything).Return([]*product.Product{}, nil)

		w := doRequest(t, prodSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.NotNil(t, body)
		assert.Len(t, body, 0)
	})

	main.Run("WithProducts", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("List", mock.Anything).Return([]*product.Product{
			{
				ID:          "widget",
				Name:        map[string]string{"en": "Widget"},
				Description: map[string]string{"en": "A widget"},
				Price:       map[string]product.PriceItem{"default": {Currency: "EUR", Value: 10}},
			},
		}, nil)

		w := doRequest(t, prodSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "widget", body[0]["id"])
	})
}
