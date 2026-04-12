package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/internal/shop"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type productServiceMock struct {
	mock.Mock
}

func (m *productServiceMock) Create(ctx context.Context, req product.CreateRequest) (*product.Product, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*product.Product), args.Error(1)
}

func (m *productServiceMock) Get(ctx context.Context, id string) (*product.AdminProduct, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*product.AdminProduct), args.Error(1)
}

func (m *productServiceMock) ListByShop(ctx context.Context, shopID string) ([]*product.AdminProduct, error) {
	args := m.Called(ctx, shopID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*product.AdminProduct), args.Error(1)
}

func (m *productServiceMock) Update(ctx context.Context, id string, req product.UpdateRequest) error {
	args := m.Called(ctx, id, req)
	return args.Error(0)
}

func TestListShopProducts(main *testing.T) {
	shopID := "myshop"
	ownerID := "owner-1"

	makeShop := func() *shop.AdminShop {
		return &shop.AdminShop{
			Shop:    shop.Shop{ID: shopID, Titles: map[string]string{"EN": "My Shop"}},
			OwnerID: ownerID,
		}
	}

	makeProducts := func() []*product.AdminProduct {
		return []*product.AdminProduct{
			{
				PublicProduct: product.PublicProduct{
					ID:      "018f4e3a-0000-7000-8000-000000000099",
					Data: map[string]product.DataItem{"EN": {Title: "Widget", Description: "A fine widget"}},
				},
				ShopOwnerID: ownerID,
				CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			},
		}
	}

	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, shopSvc *shopServiceMock, prodSvc *productServiceMock, user *auth.User) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{shop: shopSvc, prod: prodSvc, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shops/"+shopID+"/products", nil)
		r.SetPathValue("id", shopID)
		if user != nil {
			r = r.WithContext(auth.ContextWithUser(r.Context(), user))
		}
		w := httptest.NewRecorder()
		h.ListShopProducts(w, r)
		return w
	}

	main.Run("ShopNotFound", func(t *testing.T) {
		shopSvc := &shopServiceMock{}
		defer shopSvc.AssertExpectations(t)
		shopSvc.On("Get", mock.Anything, shopID).Return(nil, shop.ErrShopNotFound)

		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)

		w := doRequest(t, shopSvc, prodSvc, nil)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"shop not found"}`, w.Body.String())
	})

	main.Run("ShopServiceError", func(t *testing.T) {
		shopSvc := &shopServiceMock{}
		defer shopSvc.AssertExpectations(t)
		shopSvc.On("Get", mock.Anything, shopID).Return(nil, errors.New("db error"))

		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)

		w := doRequest(t, shopSvc, prodSvc, nil)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("ProductServiceError", func(t *testing.T) {
		shopSvc := &shopServiceMock{}
		defer shopSvc.AssertExpectations(t)
		shopSvc.On("Get", mock.Anything, shopID).Return(makeShop(), nil)

		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("ListByShop", mock.Anything, shopID).Return(nil, errors.New("db error"))

		w := doRequest(t, shopSvc, prodSvc, nil)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("UnauthenticatedGetsPublicFields", func(t *testing.T) {
		shopSvc := &shopServiceMock{}
		defer shopSvc.AssertExpectations(t)
		shopSvc.On("Get", mock.Anything, shopID).Return(makeShop(), nil)

		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("ListByShop", mock.Anything, shopID).Return(makeProducts(), nil)

		w := doRequest(t, shopSvc, prodSvc, nil)

		assert.Equal(t, http.StatusOK, w.Code)
		var body []map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Len(t, body, 1)
		assert.Contains(t, body[0], "id")
		assert.Contains(t, body[0], "data")
		assert.NotContains(t, body[0], "created_at")
		assert.NotContains(t, body[0], "updated_at")
	})

	main.Run("NonOwnerGetsPublicFields", func(t *testing.T) {
		shopSvc := &shopServiceMock{}
		defer shopSvc.AssertExpectations(t)
		shopSvc.On("Get", mock.Anything, shopID).Return(makeShop(), nil)

		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("ListByShop", mock.Anything, shopID).Return(makeProducts(), nil)

		otherUser := &auth.User{ID: "other-user", Scopes: nil}
		w := doRequest(t, shopSvc, prodSvc, otherUser)

		assert.Equal(t, http.StatusOK, w.Code)
		var body []map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Len(t, body, 1)
		assert.Contains(t, body[0], "id")
		assert.Contains(t, body[0], "data")
		assert.NotContains(t, body[0], "created_at")
		assert.NotContains(t, body[0], "updated_at")
	})

	main.Run("AdminGetsFullFields", func(t *testing.T) {
		shopSvc := &shopServiceMock{}
		defer shopSvc.AssertExpectations(t)
		shopSvc.On("Get", mock.Anything, shopID).Return(makeShop(), nil)

		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("ListByShop", mock.Anything, shopID).Return(makeProducts(), nil)

		admin := &auth.User{ID: "admin-1", Scopes: []auth.Scope{auth.ScopeAdmin}}
		w := doRequest(t, shopSvc, prodSvc, admin)

		assert.Equal(t, http.StatusOK, w.Code)
		var body []map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Len(t, body, 1)
		assert.Contains(t, body[0], "id")
		assert.Contains(t, body[0], "data")
		assert.Contains(t, body[0], "created_at")
		assert.Contains(t, body[0], "updated_at")
	})

	main.Run("ShopOwnerGetsFullFields", func(t *testing.T) {
		shopSvc := &shopServiceMock{}
		defer shopSvc.AssertExpectations(t)
		shopSvc.On("Get", mock.Anything, shopID).Return(makeShop(), nil)

		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("ListByShop", mock.Anything, shopID).Return(makeProducts(), nil)

		owner := &auth.User{ID: ownerID, Scopes: nil}
		w := doRequest(t, shopSvc, prodSvc, owner)

		assert.Equal(t, http.StatusOK, w.Code)
		var body []map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Len(t, body, 1)
		assert.Contains(t, body[0], "id")
		assert.Contains(t, body[0], "data")
		assert.Contains(t, body[0], "created_at")
		assert.Contains(t, body[0], "updated_at")
	})

	main.Run("EmptyList", func(t *testing.T) {
		shopSvc := &shopServiceMock{}
		defer shopSvc.AssertExpectations(t)
		shopSvc.On("Get", mock.Anything, shopID).Return(makeShop(), nil)

		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("ListByShop", mock.Anything, shopID).Return([]*product.AdminProduct{}, nil)

		w := doRequest(t, shopSvc, prodSvc, nil)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `[]`, w.Body.String())
	})
}

func TestUpdateProduct(main *testing.T) {
	productID := "018f4e3a-0000-7000-8000-000000000099"
	ownerID := "owner-1"

	makeAdminProduct := func() *product.AdminProduct {
		return &product.AdminProduct{
			PublicProduct: product.PublicProduct{
				ID:      productID,
				Data: map[string]product.DataItem{"EN": {Title: "Widget", Description: "Desc"}},
			},
			ShopOwnerID: ownerID,
			CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		}
	}

	validBody := `{"data":{"EN":{"title":"New Title","description":"New Desc"}}}`

	doRequest := func(t *testing.T, prodSvc *productServiceMock, user *auth.User) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{prod: prodSvc, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPatch, "/products/"+productID, strings.NewReader(validBody))
		r.SetPathValue("id", productID)
		if user != nil {
			r = r.WithContext(auth.ContextWithUser(r.Context(), user))
		}
		w := httptest.NewRecorder()
		h.UpdateProduct(w, r)
		return w
	}

	main.Run("Unauthenticated", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)

		w := doRequest(t, prodSvc, nil)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	main.Run("NonOwnerForbidden", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Get", mock.Anything, productID).Return(makeAdminProduct(), nil)

		other := &auth.User{ID: "other-user", Scopes: nil}
		w := doRequest(t, prodSvc, other)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	main.Run("ProductNotFoundForNonAdmin", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Get", mock.Anything, productID).Return(nil, product.ErrProductNotFound)

		other := &auth.User{ID: "other-user", Scopes: nil}
		w := doRequest(t, prodSvc, other)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"product not found"}`, w.Body.String())
	})

	main.Run("MissingTitle", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Update", mock.Anything, productID, mock.Anything).Return(product.ErrMissingTitle)

		admin := &auth.User{ID: "admin-1", Scopes: []auth.Scope{auth.ScopeAdmin}}
		w := doRequest(t, prodSvc, admin)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"at least one title is required"}`, w.Body.String())
	})

	main.Run("InvalidLanguage", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Update", mock.Anything, productID, mock.Anything).Return(&product.InvalidLanguageError{Lang: "ZZ"})

		admin := &auth.User{ID: "admin-1", Scopes: []auth.Scope{auth.ScopeAdmin}}
		w := doRequest(t, prodSvc, admin)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"invalid language code: ZZ"}`, w.Body.String())
	})

	main.Run("ProductNotFoundOnUpdate", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Update", mock.Anything, productID, mock.Anything).Return(product.ErrProductNotFound)

		admin := &auth.User{ID: "admin-1", Scopes: []auth.Scope{auth.ScopeAdmin}}
		w := doRequest(t, prodSvc, admin)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"product not found"}`, w.Body.String())
	})

	main.Run("AdminSuccess", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Update", mock.Anything, productID, mock.Anything).Return(nil)

		admin := &auth.User{ID: "admin-1", Scopes: []auth.Scope{auth.ScopeAdmin}}
		w := doRequest(t, prodSvc, admin)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	main.Run("ShopOwnerSuccess", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Get", mock.Anything, productID).Return(makeAdminProduct(), nil)
		prodSvc.On("Update", mock.Anything, productID, mock.Anything).Return(nil)

		owner := &auth.User{ID: ownerID, Scopes: nil}
		w := doRequest(t, prodSvc, owner)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}
