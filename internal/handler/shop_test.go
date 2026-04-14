package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/internal/shop"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type shopServiceMock struct {
	mock.Mock
}

func (m *shopServiceMock) Get(ctx context.Context) (*shop.Shop, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*shop.Shop), args.Error(1)
}

func TestServeShop(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, shopSvc *shopServiceMock) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{shop: shopSvc, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/shop", nil)
		w := httptest.NewRecorder()
		h.ServeShop(w, r)
		return w
	}

	main.Run("ReturnsShopData", func(t *testing.T) {
		shopSvc := &shopServiceMock{}
		defer shopSvc.AssertExpectations(t)
		shopSvc.On("Get", mock.Anything).Return(&shop.Shop{
			Name:        map[string]string{"en": "My Shop", "uk": "Мій магазин"},
			Title:       map[string]string{"en": "Best Shop", "uk": "Найкращий магазин"},
			Description: map[string]string{"en": "We sell things", "uk": "Ми продаємо речі"},
		}, nil)

		w := doRequest(t, shopSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		name, ok := body["name"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "My Shop", name["en"])
		assert.Equal(t, "Мій магазин", name["uk"])
		title, ok := body["title"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Best Shop", title["en"])
		description, ok := body["description"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "We sell things", description["en"])
	})

	main.Run("EmptyShopYieldsEmptyObject", func(t *testing.T) {
		shopSvc := &shopServiceMock{}
		defer shopSvc.AssertExpectations(t)
		shopSvc.On("Get", mock.Anything).Return(&shop.Shop{}, nil)

		w := doRequest(t, shopSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Empty(t, body)
	})

	main.Run("ServiceError", func(t *testing.T) {
		shopSvc := &shopServiceMock{}
		defer shopSvc.AssertExpectations(t)
		shopSvc.On("Get", mock.Anything).Return(nil, errors.New("internal error"))

		w := doRequest(t, shopSvc)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
