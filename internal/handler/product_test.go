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

func (m *productServiceMock) Get(ctx context.Context, id string) (*product.Product, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*product.Product), args.Error(1)
}

func (m *productServiceMock) List(ctx context.Context) ([]*product.Product, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*product.Product), args.Error(1)
}

func (m *productServiceMock) GetPrice(ctx context.Context, id string, countryID string) (*product.PriceResult, error) {
	args := m.Called(ctx, id, countryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*product.PriceResult), args.Error(1)
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
				ID:   "018f4e3a-0000-7000-8000-000000000099",
				Data: map[string]product.DataItem{"EN": {Title: "Widget", Description: "A widget"}},
			},
		}, nil)

		w := doRequest(t, prodSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000099", body[0]["id"])
	})
}

func TestGetProduct(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, prodSvc *productServiceMock, id string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{prod: prodSvc, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/"+id, nil)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		h.GetProduct(w, r)
		return w
	}

	main.Run("NotFound", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Get", mock.Anything, "00000000-0000-0000-0000-000000000000").
			Return(nil, product.ErrProductNotFound)

		w := doRequest(t, prodSvc, "00000000-0000-0000-0000-000000000000")
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"product not found"}`, w.Body.String())
	})

	main.Run("Success", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Get", mock.Anything, "018f4e3a-0000-7000-8000-000000000099").
			Return(&product.Product{
				ID:   "018f4e3a-0000-7000-8000-000000000099",
				Data: map[string]product.DataItem{"EN": {Title: "Widget", Description: "A widget"}},
			}, nil)

		w := doRequest(t, prodSvc, "018f4e3a-0000-7000-8000-000000000099")
		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000099", body["id"])
		assert.Contains(t, body, "data")
	})
}

func TestGetProductPrice(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, prodSvc *productServiceMock, id, country string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{prod: prodSvc, resp: resp, l: zerolog.Nop()}
		url := "/products/" + id + "/prices"
		if country != "" {
			url += "?country=" + country
		}
		r := httptest.NewRequest(http.MethodGet, url, nil)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		h.GetProductPrice(w, r)
		return w
	}

	main.Run("NotFound", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("GetPrice", mock.Anything, "00000000-0000-0000-0000-000000000000", "DEFAULT").
			Return(nil, product.ErrProductNotFound)

		w := doRequest(t, prodSvc, "00000000-0000-0000-0000-000000000000", "")
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"product not found"}`, w.Body.String())
	})

	main.Run("Success_ExactCountry", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("GetPrice", mock.Anything, "018f4e3a-0000-7000-8000-000000000099", "US").
			Return(&product.PriceResult{CountryID: "US", Value: 1200}, nil)

		w := doRequest(t, prodSvc, "018f4e3a-0000-7000-8000-000000000099", "US")
		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "US", body["country_id"])
		assert.Equal(t, float64(1200), body["value"])
	})

	main.Run("Success_DefaultFallback", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("GetPrice", mock.Anything, "018f4e3a-0000-7000-8000-000000000099", "UA").
			Return(&product.PriceResult{CountryID: "DEFAULT", Value: 1000}, nil)

		w := doRequest(t, prodSvc, "018f4e3a-0000-7000-8000-000000000099", "UA")
		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		// Handler overrides CountryID with the requested country
		assert.Equal(t, "UA", body["country_id"])
		assert.Equal(t, float64(1000), body["value"])
	})
}
