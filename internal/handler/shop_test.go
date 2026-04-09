package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/shop"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type shopServiceMock struct {
	mock.Mock
}

func (m *shopServiceMock) Create(ctx context.Context, req shop.CreateRequest) (*shop.Shop, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*shop.Shop), args.Error(1)
}

func (m *shopServiceMock) Update(ctx context.Context, id string, req shop.UpdateRequest) error {
	args := m.Called(ctx, id, req)
	return args.Error(0)
}

func (m *shopServiceMock) List(ctx context.Context) ([]shop.Shop, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]shop.Shop), args.Error(1)
}

func TestListShops(main *testing.T) {
	main.Run("NoUserInContext", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops", nil)
		w := httptest.NewRecorder()

		h.ListShops(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("UserNotAdmin", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops", nil)
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: nil}))
		w := httptest.NewRecorder()

		h.ListShops(w, r)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	main.Run("ServiceError", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("List", mock.Anything).Return(nil, errors.New("db error"))

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops", nil)
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.ListShops(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("Success", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("List", mock.Anything).Return([]shop.Shop{
			{ID: "shop1", Names: map[string]string{"en": "Shop One"}},
			{ID: "shop2", Names: map[string]string{"en": "Shop Two", "uk": "Магазин Два"}},
		}, nil)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops", nil)
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.ListShops(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t,
			`[{"id":"shop1","names":{"en":"Shop One"}},{"id":"shop2","names":{"en":"Shop Two","uk":"Магазин Два"}}]`,
			w.Body.String(),
		)
	})

	main.Run("SuccessEmpty", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("List", mock.Anything).Return([]shop.Shop{}, nil)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops", nil)
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.ListShops(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `[]`, w.Body.String())
	})
}

func TestCreateShop(main *testing.T) {
	main.Run("BadRequestBody", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/shops", bytes.NewBufferString("not json"))
		w := httptest.NewRecorder()

		h.CreateShop(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	main.Run("NoUserInContext", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/shops", bytes.NewBufferString(`{"id":"myshop"}`))
		w := httptest.NewRecorder()

		h.CreateShop(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})

	main.Run("UserNotAdmin", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/shops", bytes.NewBufferString(`{"id":"myshop"}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: nil}))
		w := httptest.NewRecorder()

		h.CreateShop(w, r)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
		}
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Create", mock.Anything, mock.Anything).Return(nil, shop.ErrInvalidLanguage)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/shops", bytes.NewBufferString(`{"id":"myshop","names":{"xx":"My Shop"}}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.CreateShop(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
		if body := w.Body.String(); body != `{"error": "invalid language code"}` {
			t.Errorf("unexpected body: %s", body)
		}
	})

	main.Run("ShopAlreadyExists", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Create", mock.Anything, mock.Anything).Return(nil, shop.ErrShopAlreadyExists)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/shops", bytes.NewBufferString(`{"id":"myshop"}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.CreateShop(w, r)

		if w.Code != http.StatusConflict {
			t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
		}
		if body := w.Body.String(); body != `{"error": "shop already exists"}` {
			t.Errorf("unexpected body: %s", body)
		}
	})

	main.Run("ShopCreateError", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/shops", bytes.NewBufferString(`{"id":"myshop"}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.CreateShop(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})

	main.Run("Success", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Create", mock.Anything, mock.Anything).Return(&shop.Shop{ID: "myshop"}, nil)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/shops", bytes.NewBufferString(`{"id":"myshop"}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.CreateShop(w, r)

		if w.Code != http.StatusCreated {
			t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
		}
	})
}

func TestUpdateShop(main *testing.T) {
	main.Run("BadRequestBody", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString("not json"))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	main.Run("NoUserInContext", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{}`))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})

	main.Run("UserNotAdmin", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: nil}))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
		}
	})

	main.Run("ShopNotFound", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(shop.ErrShopNotFound)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{"names":{"en":"Test"}}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
		if body := w.Body.String(); body != `{"error": "shop not found"}` {
			t.Errorf("unexpected body: %s", body)
		}
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(shop.ErrInvalidLanguage)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{"names":{"xx":"Test"}}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
		if body := w.Body.String(); body != `{"error": "invalid language code"}` {
			t.Errorf("unexpected body: %s", body)
		}
	})

	main.Run("ServiceError", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("db error"))

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{"names":{"en":"Test"}}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})

	main.Run("Success", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{"names":{"en":"Updated"}}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})
}
