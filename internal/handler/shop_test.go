package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func (m *shopServiceMock) Get(ctx context.Context, id string) (*shop.AdminShop, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*shop.AdminShop), args.Error(1)
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
			{ID: "shop1", Names: map[string]string{"EN": "Shop One"}},
			{ID: "shop2", Names: map[string]string{"EN": "Shop Two", "UK": "Магазин Два"}},
		}, nil)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops", nil)
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.ListShops(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t,
			`[{"id":"shop1","names":{"EN":"Shop One"}},{"id":"shop2","names":{"EN":"Shop Two","UK":"Магазин Два"}}]`,
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

	main.Run("InvalidOwner", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Create", mock.Anything, mock.Anything).Return(nil, shop.ErrInvalidOwner)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/shops",
			bytes.NewBufferString(`{"id":"myshop","names":{"EN":"My Shop"},"owner_id":"non-existent-uuid"}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.CreateShop(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"invalid owner id"}`, w.Body.String())
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

func TestGetShop(main *testing.T) {
	main.Run("ShopNotFound", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Get", mock.Anything, "myshop").Return(nil, shop.ErrShopNotFound)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops/myshop", nil)
		r.SetPathValue("id", "myshop")
		w := httptest.NewRecorder()

		h.GetShop(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error": "shop not found"}`, w.Body.String())
	})

	main.Run("ServiceError", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Get", mock.Anything, "myshop").Return(nil, errors.New("db error"))

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops/myshop", nil)
		r.SetPathValue("id", "myshop")
		w := httptest.NewRecorder()

		h.GetShop(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("AdminUserGetsFullFields", func(t *testing.T) {
		createdAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		updatedAt := time.Date(2024, 6, 20, 12, 30, 0, 0, time.UTC)

		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Get", mock.Anything, "myshop").Return(
			&shop.AdminShop{
				Shop:      shop.Shop{ID: "myshop", Names: map[string]string{"EN": "My Shop"}},
				OwnerID:   "owner-uuid-1",
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
			nil,
		)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops/myshop", nil)
		r.SetPathValue("id", "myshop")
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.GetShop(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{
			"id": "myshop",
			"names": {"EN": "My Shop"},
			"owner_id": "owner-uuid-1",
			"created_at": "2024-01-15T10:00:00Z",
			"updated_at": "2024-06-20T12:30:00Z"
		}`, w.Body.String())
	})

	main.Run("NonAdminUserGetsBasicFields", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Get", mock.Anything, "myshop").Return(
			&shop.AdminShop{
				Shop:    shop.Shop{ID: "myshop", Names: map[string]string{"EN": "My Shop"}},
				OwnerID: "owner-uuid-2",
			},
			nil,
		)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops/myshop", nil)
		r.SetPathValue("id", "myshop")
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: nil}))
		w := httptest.NewRecorder()

		h.GetShop(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"id":"myshop","names":{"EN":"My Shop"}}`, w.Body.String())
	})

	main.Run("UnauthenticatedGetsBasicFields", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Get", mock.Anything, "myshop").Return(
			&shop.AdminShop{
				Shop:    shop.Shop{ID: "myshop", Names: map[string]string{"EN": "My Shop"}},
				OwnerID: "owner-uuid-3",
			},
			nil,
		)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops/myshop", nil)
		r.SetPathValue("id", "myshop")
		// no user in context
		w := httptest.NewRecorder()

		h.GetShop(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"id":"myshop","names":{"EN":"My Shop"}}`, w.Body.String())
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

	main.Run("UserIsNotOwnerAndNotAdmin", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Get", mock.Anything, "myshop").Return(
			&shop.AdminShop{Shop: shop.Shop{ID: "myshop"}, OwnerID: "other-owner"}, nil)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{}`))
		r.SetPathValue("id", "myshop")
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: nil}))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	main.Run("UserIsOwner", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Get", mock.Anything, "myshop").Return(
			&shop.AdminShop{Shop: shop.Shop{ID: "myshop"}, OwnerID: "u1"}, nil)
		svc.On("Update", mock.Anything, "myshop", mock.Anything).Return(nil)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{"names":{"EN":"Updated"}}`))
		r.SetPathValue("id", "myshop")
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: nil}))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	main.Run("ShopNotFoundDuringOwnerCheck", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Get", mock.Anything, "myshop").Return(nil, shop.ErrShopNotFound)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{}`))
		r.SetPathValue("id", "myshop")
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: nil}))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"shop not found"}`, w.Body.String())
	})

	main.Run("GetErrorDuringOwnerCheck", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Get", mock.Anything, "myshop").Return(nil, errors.New("db error"))

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{}`))
		r.SetPathValue("id", "myshop")
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: nil}))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("ShopNotFound", func(t *testing.T) {
		svc := &shopServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(shop.ErrShopNotFound)

		h := &Handler{shop: svc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{"names":{"EN":"Test"}}`))
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
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{"names":{"EN":"Test"}}`))
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
		r := httptest.NewRequest(http.MethodPatch, "/shops/myshop", bytes.NewBufferString(`{"names":{"EN":"Updated"}}`))
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "u1", Scopes: []auth.Scope{auth.ScopeAdmin}}))
		w := httptest.NewRecorder()

		h.UpdateShop(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})
}
