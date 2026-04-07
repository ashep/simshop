package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMiddleware(main *testing.T) {
	nextHandler := func() http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
	}

	main.Run("MissingHeader", func(t *testing.T) {
		sm := &authSvcMock{}
		defer sm.AssertExpectations(t)

		h := auth.Middleware(&authSvcMock{})(nextHandler())
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	main.Run("UserNotFound", func(t *testing.T) {
		sm := &authSvcMock{}
		defer sm.AssertExpectations(t)
		sm.On("GetByAPIKey", mock.Anything, mock.Anything).Return(&auth.User{}, auth.ErrUserNotFound)

		h := auth.Middleware(sm)(nextHandler())
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-API-Key", "aKey")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	main.Run("ServiceError", func(t *testing.T) {
		sm := &authSvcMock{}
		defer sm.AssertExpectations(t)
		sm.On("GetByAPIKey", mock.Anything, mock.Anything).Return(&auth.User{}, errors.New("db error"))

		h := auth.Middleware(sm)(nextHandler())
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-API-Key", "aKey")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	main.Run("Success", func(t *testing.T) {
		sm := &authSvcMock{}
		defer sm.AssertExpectations(t)
		sm.On("GetByAPIKey", mock.Anything, mock.Anything).Return(&auth.User{}, nil)

		h := auth.Middleware(sm)(nextHandler())
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-API-Key", "aKey")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	main.Run("UserStoredInContext", func(t *testing.T) {
		user := &auth.User{ID: "u1", APIKey: "aKey"}
		sm := &authSvcMock{}
		defer sm.AssertExpectations(t)
		sm.On("GetByAPIKey", mock.Anything, "aKey").Return(user, nil)

		var ctxUser *auth.User
		next := func(w http.ResponseWriter, r *http.Request) {
			ctxUser = auth.GetUserFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}

		h := auth.Middleware(sm)(next)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-API-Key", "aKey")
		h.ServeHTTP(httptest.NewRecorder(), req)
		assert.Equal(t, user, ctxUser)
	})
}

func TestGetUserFromContext(main *testing.T) {
	main.Run("ReturnsNilWhenNotSet", func(t *testing.T) {
		u := auth.GetUserFromContext(context.Background())
		assert.Nil(t, u)
	})
}

type authSvcMock struct {
	mock.Mock
}

func (m *authSvcMock) GetByAPIKey(ctx context.Context, apiKey string) (*auth.User, error) {
	args := m.Called(ctx, apiKey)
	return args.Get(0).(*auth.User), args.Error(1)
}
