package handler

import (
	"context"
	"errors"
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

func (m *propertyServiceMock) Create(ctx context.Context, req property.CreateRequest) (*property.Property, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*property.Property), args.Error(1)
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

func TestPropertyList(main *testing.T) {
	main.Run("ServiceError", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)

		svc.On("List", mock.Anything).Return(nil, errors.New("db failure"))

		h := &Handler{prop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/properties", nil)
		w := httptest.NewRecorder()

		h.PropertyList(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("Success", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)

		props := []property.Property{
			{ID: "018f4e3a-0000-7000-8000-000000000001", Titles: map[string]string{"en": "Color"}},
		}
		svc.On("List", mock.Anything).Return(props, nil)

		h := &Handler{prop: svc, resp: buildTestResponder(t), l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/properties", nil)
		w := httptest.NewRecorder()

		h.PropertyList(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `[{"id":"018f4e3a-0000-7000-8000-000000000001","titles":{"en":"Color"}}]`, w.Body.String())
	})

	main.Run("SuccessEmpty", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)

		svc.On("List", mock.Anything).Return([]property.Property{}, nil)

		h := &Handler{prop: svc, resp: buildTestResponder(t), l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/properties", nil)
		w := httptest.NewRecorder()

		h.PropertyList(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `[]`, w.Body.String())
	})
}
