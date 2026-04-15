package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/internal/novaposhta"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type novaPoshtaClientMock struct {
	mock.Mock
}

func (m *novaPoshtaClientMock) SearchCities(ctx context.Context, query string) ([]novaposhta.City, error) {
	args := m.Called(ctx, query)
	return args.Get(0).([]novaposhta.City), args.Error(1)
}

func (m *novaPoshtaClientMock) SearchBranches(ctx context.Context, cityRef, query string) ([]novaposhta.Branch, error) {
	args := m.Called(ctx, cityRef, query)
	return args.Get(0).([]novaposhta.Branch), args.Error(1)
}

func (m *novaPoshtaClientMock) SearchStreets(ctx context.Context, cityRef, query string) ([]novaposhta.Street, error) {
	args := m.Called(ctx, cityRef, query)
	return args.Get(0).([]novaposhta.Street), args.Error(1)
}

func TestSearchNPCities(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, npClient *novaPoshtaClientMock, query string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{np: npClient, resp: resp, l: zerolog.Nop()}
		target := "/nova-poshta/cities"
		if query != "" {
			target += "?q=" + query
		}
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		h.SearchNPCities(w, r)
		return w
	}

	main.Run("ReturnsCities", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		defer npClient.AssertExpectations(t)
		npClient.On("SearchCities", mock.Anything, "Київ").Return([]novaposhta.City{
			{Ref: "018f4e3a-0000-7000-8000-000000000001", Name: "м. Київ, Київська обл."},
		}, nil)

		w := doRequest(t, npClient, "Київ")
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000001", body[0]["ref"])
		assert.Equal(t, "м. Київ, Київська обл.", body[0]["name"])
	})

	main.Run("MissingQReturns400", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		w := doRequest(t, npClient, "")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("ClientErrorReturns502", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		defer npClient.AssertExpectations(t)
		npClient.On("SearchCities", mock.Anything, "Київ").Return([]novaposhta.City{}, errors.New("service down"))

		w := doRequest(t, npClient, "Київ")
		assert.Equal(t, http.StatusBadGateway, w.Code)
	})

	main.Run("EmptyResultReturnsEmptyArray", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		defer npClient.AssertExpectations(t)
		npClient.On("SearchCities", mock.Anything, "xyz").Return([]novaposhta.City{}, nil)

		w := doRequest(t, npClient, "xyz")
		assert.Equal(t, http.StatusOK, w.Code)
		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Len(t, body, 0)
	})
}

func TestSearchNPStreets(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, npClient *novaPoshtaClientMock, cityRef, query string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{np: npClient, resp: resp, l: zerolog.Nop()}
		target := "/nova-poshta/streets"
		sep := "?"
		if cityRef != "" {
			target += sep + "city_ref=" + cityRef
			sep = "&"
		}
		if query != "" {
			target += sep + "q=" + query
		}
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		h.SearchNPStreets(w, r)
		return w
	}

	main.Run("ReturnsStreets", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		defer npClient.AssertExpectations(t)
		npClient.On("SearchStreets", mock.Anything, "city-ref-1", "Хрещ").Return([]novaposhta.Street{
			{Ref: "018f4e3a-0000-7000-8000-000000000003", Name: "вул. Хрещатик"},
		}, nil)

		w := doRequest(t, npClient, "city-ref-1", "Хрещ")
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000003", body[0]["ref"])
		assert.Equal(t, "вул. Хрещатик", body[0]["name"])
	})

	main.Run("MissingCityRefReturns400", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		w := doRequest(t, npClient, "", "Хрещ")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingQReturns400", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		w := doRequest(t, npClient, "city-ref-1", "")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("ClientErrorReturns502", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		defer npClient.AssertExpectations(t)
		npClient.On("SearchStreets", mock.Anything, "city-ref-1", "Хрещ").Return([]novaposhta.Street{}, errors.New("service down"))

		w := doRequest(t, npClient, "city-ref-1", "Хрещ")
		assert.Equal(t, http.StatusBadGateway, w.Code)
	})
}

func TestSearchNPBranches(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, npClient *novaPoshtaClientMock, cityRef, query string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{np: npClient, resp: resp, l: zerolog.Nop()}
		target := "/nova-poshta/branches"
		sep := "?"
		if cityRef != "" {
			target += sep + "city_ref=" + cityRef
			sep = "&"
		}
		if query != "" {
			target += sep + "q=" + query
		}
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		h.SearchNPBranches(w, r)
		return w
	}

	main.Run("ReturnsBranches", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		defer npClient.AssertExpectations(t)
		npClient.On("SearchBranches", mock.Anything, "city-ref-1", "Хрещ").Return([]novaposhta.Branch{
			{Ref: "018f4e3a-0000-7000-8000-000000000002", Name: "Відділення №1: вул. Хрещатик, 22"},
		}, nil)

		w := doRequest(t, npClient, "city-ref-1", "Хрещ")
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000002", body[0]["ref"])
		assert.Equal(t, "Відділення №1: вул. Хрещатик, 22", body[0]["name"])
	})

	main.Run("MissingCityRefReturns400", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		w := doRequest(t, npClient, "", "Хрещ")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingQReturns400", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		w := doRequest(t, npClient, "city-ref-1", "")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("ClientErrorReturns502", func(t *testing.T) {
		npClient := &novaPoshtaClientMock{}
		defer npClient.AssertExpectations(t)
		npClient.On("SearchBranches", mock.Anything, "city-ref-1", "Хрещ").Return([]novaposhta.Branch{}, errors.New("service down"))

		w := doRequest(t, npClient, "city-ref-1", "Хрещ")
		assert.Equal(t, http.StatusBadGateway, w.Code)
	})
}
