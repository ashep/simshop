package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/api"
	"github.com/ashep/simshop/internal/auth"
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

func (m *propertyServiceMock) Update(ctx context.Context, id string, req property.UpdateRequest) error {
	args := m.Called(ctx, id, req)
	return args.Error(0)
}

func buildTestResponder(t *testing.T) *openapi.Responder {
	t.Helper()
	oas, err := openapi.New(api.Spec)
	require.NoError(t, err)
	return oas.Responder()
}

func TestCreateProperty(main *testing.T) {
	main.Run("Forbidden_Unauthenticated", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{prop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/properties", bytes.NewBufferString(`{"titles":{"EN":"Color"}}`))
		w := httptest.NewRecorder()

		h.CreateProperty(w, r)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	main.Run("Forbidden_NonAdmin", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{prop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/properties", bytes.NewBufferString(`{"titles":{"EN":"Color"}}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: nil}))
		w := httptest.NewRecorder()

		h.CreateProperty(w, r)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	main.Run("Success", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Create", mock.Anything, mock.Anything).Return(
			&property.Property{ID: "018f4e3a-0000-7000-8000-000000000001"}, nil,
		)

		h := &Handler{prop: svc, resp: buildTestResponder(main), l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/properties", bytes.NewBufferString(`{"titles":{"EN":"Color"}}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.CreateProperty(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.JSONEq(t, `{"id":"018f4e3a-0000-7000-8000-000000000001"}`, w.Body.String())
	})
}

func TestListProperties(main *testing.T) {
	main.Run("ServiceError", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)

		svc.On("List", mock.Anything).Return(nil, errors.New("db failure"))

		h := &Handler{prop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/properties", nil)
		w := httptest.NewRecorder()

		h.ListProperties(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("Success", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)

		props := []property.Property{
			{ID: "018f4e3a-0000-7000-8000-000000000001", Titles: map[string]string{"EN": "Color"}},
		}
		svc.On("List", mock.Anything).Return(props, nil)

		h := &Handler{prop: svc, resp: buildTestResponder(t), l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/properties", nil)
		w := httptest.NewRecorder()

		h.ListProperties(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `[{"id":"018f4e3a-0000-7000-8000-000000000001","titles":{"EN":"Color"}}]`, w.Body.String())
	})

	main.Run("SuccessEmpty", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)

		svc.On("List", mock.Anything).Return([]property.Property{}, nil)

		h := &Handler{prop: svc, resp: buildTestResponder(t), l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/properties", nil)
		w := httptest.NewRecorder()

		h.ListProperties(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `[]`, w.Body.String())
	})
}

func TestUpdateProperty(main *testing.T) {
	main.Run("Forbidden_Unauthenticated", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{prop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/properties/some-id", bytes.NewBufferString(`{"titles":{"EN":"Updated"}}`))
		w := httptest.NewRecorder()

		h.UpdateProperty(w, r)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	main.Run("Forbidden_NonAdmin", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{prop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/properties/some-id", bytes.NewBufferString(`{"titles":{"EN":"Updated"}}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: nil}))
		w := httptest.NewRecorder()

		h.UpdateProperty(w, r)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	main.Run("MissingTitle", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(property.ErrMissingTitle)

		h := &Handler{prop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/properties/some-id", bytes.NewBufferString(`{"titles":{}}`))
		r.SetPathValue("id", "some-id")
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.UpdateProperty(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"at least one title is required"}`, w.Body.String())
	})

	main.Run("PropertyNotFound", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Update", mock.Anything, "some-id", mock.Anything).Return(property.ErrPropertyNotFound)

		h := &Handler{prop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/properties/some-id", bytes.NewBufferString(`{"titles":{"EN":"Updated"}}`))
		r.SetPathValue("id", "some-id")
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.UpdateProperty(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"property not found"}`, w.Body.String())
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(&property.InvalidLanguageError{Lang: "zz"})

		h := &Handler{prop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/properties/some-id", bytes.NewBufferString(`{"titles":{"zz":"Bad"}}`))
		r.SetPathValue("id", "some-id")
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.UpdateProperty(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"invalid language code: zz"}`, w.Body.String())
	})

	main.Run("ServiceError", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("db failure"))

		h := &Handler{prop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/properties/some-id", bytes.NewBufferString(`{"titles":{"EN":"Updated"}}`))
		r.SetPathValue("id", "some-id")
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.UpdateProperty(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("Success", func(t *testing.T) {
		svc := &propertyServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Update", mock.Anything, "some-id", property.UpdateRequest{Titles: map[string]string{"EN": "Updated"}}).Return(nil)

		h := &Handler{prop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/properties/some-id", bytes.NewBufferString(`{"titles":{"EN":"Updated"}}`))
		r.SetPathValue("id", "some-id")
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.UpdateProperty(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}
