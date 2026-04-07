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
)


type mockShopService struct {
	createFn func(req shop.CreateRequest) (*shop.Shop, error)
}

func (m *mockShopService) Create(_ context.Context, req shop.CreateRequest) (*shop.Shop, error) {
	return m.createFn(req)
}

func TestCreateShop(main *testing.T) {
	main.Run("BadRequestBody", func(t *testing.T) {
		h := &Handler{l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/shops", bytes.NewBufferString("not json"))
		w := httptest.NewRecorder()

		h.CreateShop(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	main.Run("NoUserInContext", func(t *testing.T) {
		h := &Handler{l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/shops", bytes.NewBufferString(`{"id":"myshop"}`))
		w := httptest.NewRecorder()

		h.CreateShop(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
		}
	})

	main.Run("UserNotAdmin", func(t *testing.T) {
		h := &Handler{l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/shops", bytes.NewBufferString(`{"id":"myshop"}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: nil}))
		w := httptest.NewRecorder()

		h.CreateShop(w, r)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
		}
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		svc := &mockShopService{
			createFn: func(req shop.CreateRequest) (*shop.Shop, error) {
				return nil, shop.ErrInvalidLanguage
			},
		}
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
		svc := &mockShopService{
			createFn: func(req shop.CreateRequest) (*shop.Shop, error) {
				return nil, shop.ErrShopAlreadyExists
			},
		}
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
		svc := &mockShopService{
			createFn: func(req shop.CreateRequest) (*shop.Shop, error) {
				return nil, errors.New("db error")
			},
		}
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
		svc := &mockShopService{
			createFn: func(req shop.CreateRequest) (*shop.Shop, error) {
				return &shop.Shop{ID: req.ID}, nil
			},
		}
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
