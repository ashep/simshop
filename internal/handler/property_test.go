package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/api"
	"github.com/ashep/simshop/internal/openapi"
	"github.com/ashep/simshop/internal/property"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type propertyServiceMock struct {
	mock.Mock
}

func (m *propertyServiceMock) List(ctx context.Context) ([]property.Property, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]property.Property), args.Error(1)
}

func buildTestResponder(t *testing.T) *openapi.Responder {
	t.Helper()
	oas, err := openapi.New(api.Spec)
	require.NoError(t, err)
	return oas.Responder()
}

func TestListProperties(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, propSvc *propertyServiceMock) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{prop: propSvc, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/properties", nil)
		w := httptest.NewRecorder()
		h.ListProperties(w, r)
		return w
	}

	main.Run("EmptyList", func(t *testing.T) {
		propSvc := &propertyServiceMock{}
		defer propSvc.AssertExpectations(t)
		propSvc.On("List", mock.Anything).Return([]property.Property{}, nil)

		w := doRequest(t, propSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.NotNil(t, body)
	})

	main.Run("WithProperties", func(t *testing.T) {
		propSvc := &propertyServiceMock{}
		defer propSvc.AssertExpectations(t)
		propSvc.On("List", mock.Anything).Return([]property.Property{
			{ID: "018f4e3a-0000-7000-8000-000000000001", Titles: map[string]string{"EN": "Color"}},
		}, nil)

		w := doRequest(t, propSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000001", body[0]["id"])
	})
}
