package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/internal/product"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type productServiceMock struct {
	mock.Mock
}

func (m *productServiceMock) List(ctx context.Context) ([]*product.Item, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*product.Item), args.Error(1)
}

func TestListProducts(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, prodSvc *productServiceMock) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{prod: prodSvc, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products", nil)
		w := httptest.NewRecorder()
		h.ListProducts(w, r)
		return w
	}

	main.Run("EmptyList", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("List", mock.Anything).Return([]*product.Item{}, nil)

		w := doRequest(t, prodSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.NotNil(t, body)
		assert.Len(t, body, 0)
	})

	main.Run("WithProducts", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("List", mock.Anything).Return([]*product.Item{
			{
				ID:          "cronus",
				Title:       map[string]string{"en": "Cronus"},
				Description: map[string]string{"en": "A wooden desktop clock"},
			},
		}, nil)

		w := doRequest(t, prodSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "cronus", body[0]["id"])
		title, ok := body[0]["title"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Cronus", title["en"])
		assert.Nil(t, body[0]["image"])
	})

	main.Run("WithProductsWithImage", func(t *testing.T) {
		imageURL := "/images/cronus/thumb.png"
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("List", mock.Anything).Return([]*product.Item{
			{
				ID:          "cronus",
				Title:       map[string]string{"en": "Cronus"},
				Description: map[string]string{"en": "A wooden desktop clock"},
				Image:       &imageURL,
			},
		}, nil)

		w := doRequest(t, prodSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "/images/cronus/thumb.png", body[0]["image"])
	})
}

const testProductYAML = `
name:
  en: Cronus
  uk: Кронос
description:
  en: A wooden desktop clock
  uk: Настільний годинник
price:
  default:
    currency: USD
    value: 49.99
`

const testProductWithImagesYAML = `
name:
  en: Cronus
  uk: Кронос
description:
  en: A wooden desktop clock
  uk: Настільний годинник
price:
  default:
    currency: USD
    value: 49.99
images:
  - preview: thumb.jpg
    full: full.jpg
`

func TestServeProductContent(main *testing.T) {
	resp := buildTestResponder(main)
	dataDir := main.TempDir()
	productDir := filepath.Join(dataDir, "products", "cronus")
	require.NoError(main, os.MkdirAll(productDir, 0755))
	require.NoError(main, os.WriteFile(filepath.Join(productDir, "product.yaml"), []byte(testProductYAML), 0644))

	doRequest := func(t *testing.T, id, lang string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{dataDir: dataDir, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/"+id+"/"+lang, nil)
		r.SetPathValue("id", id)
		r.SetPathValue("lang", lang)
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)
		return w
	}

	main.Run("ReturnsProductDetail", func(t *testing.T) {
		w := doRequest(t, "cronus", "en")
		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "cronus", body["id"])
		assert.Equal(t, "Cronus", body["name"])
		assert.Equal(t, "A wooden desktop clock", body["description"])
	})

	main.Run("ReturnsCorrectLanguage", func(t *testing.T) {
		w := doRequest(t, "cronus", "uk")
		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "Кронос", body["name"])
		assert.Equal(t, "Настільний годинник", body["description"])
	})

	main.Run("NotFoundWhenIDMissing", func(t *testing.T) {
		w := doRequest(t, "no-such-product", "en")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundWhenLangMissing", func(t *testing.T) {
		w := doRequest(t, "cronus", "fr")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnIDPathTraversal", func(t *testing.T) {
		h := &Handler{dataDir: dataDir, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/cronus/en", nil)
		r.SetPathValue("id", "../cronus")
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnLangPathTraversal", func(t *testing.T) {
		h := &Handler{dataDir: dataDir, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/cronus/en", nil)
		r.SetPathValue("id", "cronus")
		r.SetPathValue("lang", "../en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("ImagePathsAreTransformed", func(t *testing.T) {
		imgDataDir := t.TempDir()
		imgProductDir := filepath.Join(imgDataDir, "products", "cronus")
		require.NoError(t, os.MkdirAll(imgProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(imgProductDir, "product.yaml"), []byte(testProductWithImagesYAML), 0644))

		h := &Handler{dataDir: imgDataDir, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/cronus/en", nil)
		r.SetPathValue("id", "cronus")
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

		images, ok := body["images"].([]any)
		require.True(t, ok, "images field should be present")
		require.Len(t, images, 1)
		img, ok := images[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "/images/cronus/thumb.jpg", img["preview"])
		assert.Equal(t, "/images/cronus/full.jpg", img["full"])
	})
}
