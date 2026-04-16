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
				ID:          "widget",
				Title:       map[string]string{"en": "Widget"},
				Description: map[string]string{"en": "A test product"},
			},
		}, nil)

		w := doRequest(t, prodSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "widget", body[0]["id"])
		title, ok := body[0]["title"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Widget", title["en"])
		assert.Nil(t, body[0]["image"])
	})

	main.Run("WithProductsWithImage", func(t *testing.T) {
		imageURL := "/images/widget/thumb.png"
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("List", mock.Anything).Return([]*product.Item{
			{
				ID:          "widget",
				Title:       map[string]string{"en": "Widget"},
				Description: map[string]string{"en": "A test product"},
				Image:       &imageURL,
			},
		}, nil)

		w := doRequest(t, prodSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "/images/widget/thumb.png", body[0]["image"])
	})
}

const testProductYAML = `
name:
  en: Widget
  uk: Віджет
description:
  en: A test product
  uk: Тестовий продукт
prices:
  default:
    currency: USD
    value: 49.99
`

const testProductWithImagesYAML = `
name:
  en: Widget
  uk: Віджет
description:
  en: A test product
  uk: Тестовий продукт
prices:
  default:
    currency: USD
    value: 49.99
images:
  - preview: thumb.jpg
    full: full.jpg
`

const testProductWithCountryPriceYAML = `
name:
  en: Widget
  uk: Віджет
description:
  en: A test product
  uk: Тестовий продукт
prices:
  default:
    currency: USD
    value: 49.99
  ua:
    currency: UAH
    value: 1999.99
`

const testProductWithAttrImagesYAML = `
name:
  en: Widget
  uk: Віджет
description:
  en: A test product
  uk: Тестовий продукт
prices:
  default:
    currency: USD
    value: 49.99
attr_images:
  display_color:
    red: red-thumb.jpg
    green: green-thumb.jpg
`

const testProductWithAttrPricesYAML = `
name:
  en: Widget
  uk: Віджет
description:
  en: A test product
  uk: Тестовий продукт
prices:
  default:
    currency: USD
    value: 49.99
attrs:
  display_color:
    en:
      title: Display color
      values:
        red:
          title: Red
        green:
          title: Green
    uk:
      title: Колір дисплея
      values:
        red:
          title: Червоний
        green:
          title: Зелений
attr_prices:
  display_color:
    red:
      default: 10
      ua: 5
    green:
      default: 8
      ua: 3
`

const testProductWithAttrValuesOrderYAML = `
name:
  en: Widget
  uk: Віджет
description:
  en: A test product
  uk: Тестовий продукт
prices:
  default:
    currency: USD
    value: 49.99
attrs:
  display_color:
    en:
      title: Display color
      values:
        red:
          title: Red
        green:
          title: Green
        blue:
          title: Blue
    uk:
      title: Колір дисплея
      values:
        red:
          title: Червоний
        green:
          title: Зелений
        blue:
          title: Синій
attr_values_order:
  display_color:
    - blue
    - red
    - green
`

const testProductWithAttrDescriptionYAML = `
name:
  en: Widget
  uk: Віджет
description:
  en: A test product
  uk: Тестовий продукт
prices:
  default:
    currency: USD
    value: 49.99
attrs:
  display_color:
    en:
      title: Display color
      description: "Pick a color carefully."
      values:
        red:
          title: Red
    uk:
      title: Колір дисплея
      description: "Оберіть колір уважно."
      values:
        red:
          title: Червоний
`

func TestServeProductContent(main *testing.T) {
	resp := buildTestResponder(main)
	dataDir := main.TempDir()
	productDir := filepath.Join(dataDir, "products", "widget")
	require.NoError(main, os.MkdirAll(productDir, 0755))
	require.NoError(main, os.WriteFile(filepath.Join(productDir, "product.yaml"), []byte(testProductYAML), 0644))

	doRequest := func(t *testing.T, id, lang string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{dataDir: dataDir, resp: resp, geo: &geoDetectorStub{}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/"+id+"/"+lang, nil)
		r.SetPathValue("id", id)
		r.SetPathValue("lang", lang)
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)
		return w
	}

	main.Run("ReturnsProductDetail", func(t *testing.T) {
		w := doRequest(t, "widget", "en")
		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "widget", body["id"])
		assert.Equal(t, "Widget", body["name"])
		assert.Equal(t, "A test product", body["description"])
	})

	main.Run("ReturnsCorrectLanguage", func(t *testing.T) {
		w := doRequest(t, "widget", "uk")
		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "Віджет", body["name"])
		assert.Equal(t, "Тестовий продукт", body["description"])
	})

	main.Run("NotFoundWhenIDMissing", func(t *testing.T) {
		w := doRequest(t, "no-such-product", "en")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundWhenLangMissing", func(t *testing.T) {
		w := doRequest(t, "widget", "fr")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnIDPathTraversal", func(t *testing.T) {
		h := &Handler{dataDir: dataDir, resp: resp, geo: &geoDetectorStub{}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/widget/en", nil)
		r.SetPathValue("id", "../widget")
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnLangPathTraversal", func(t *testing.T) {
		h := &Handler{dataDir: dataDir, resp: resp, geo: &geoDetectorStub{}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/widget/en", nil)
		r.SetPathValue("id", "widget")
		r.SetPathValue("lang", "../en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("ImagePathsAreTransformed", func(t *testing.T) {
		imgDataDir := t.TempDir()
		imgProductDir := filepath.Join(imgDataDir, "products", "widget")
		require.NoError(t, os.MkdirAll(imgProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(imgProductDir, "product.yaml"), []byte(testProductWithImagesYAML), 0644))

		h := &Handler{dataDir: imgDataDir, resp: resp, geo: &geoDetectorStub{}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/widget/en", nil)
		r.SetPathValue("id", "widget")
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
		assert.Equal(t, "/images/widget/thumb.jpg", img["preview"])
		assert.Equal(t, "/images/widget/full.jpg", img["full"])
	})

	main.Run("ReturnsCountryPrice", func(t *testing.T) {
		cpDataDir := t.TempDir()
		cpProductDir := filepath.Join(cpDataDir, "products", "widget")
		require.NoError(t, os.MkdirAll(cpProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(cpProductDir, "product.yaml"), []byte(testProductWithCountryPriceYAML), 0644))

		h := &Handler{dataDir: cpDataDir, resp: resp, geo: &geoDetectorStub{country: "ua"}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/widget/en", nil)
		r.SetPathValue("id", "widget")
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		price, ok := body["price"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "UAH", price["currency"])
		assert.Equal(t, 1999.99, price["value"])
	})

	main.Run("FallsBackToDefaultPrice", func(t *testing.T) {
		cpDataDir := t.TempDir()
		cpProductDir := filepath.Join(cpDataDir, "products", "widget")
		require.NoError(t, os.MkdirAll(cpProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(cpProductDir, "product.yaml"), []byte(testProductWithCountryPriceYAML), 0644))

		h := &Handler{dataDir: cpDataDir, resp: resp, geo: &geoDetectorStub{country: "xx"}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/widget/en", nil)
		r.SetPathValue("id", "widget")
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		price, ok := body["price"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "USD", price["currency"])
		assert.Equal(t, 49.99, price["value"])
	})

	main.Run("ReturnsAttrPricesResolvedByCountry", func(t *testing.T) {
		apDataDir := t.TempDir()
		apProductDir := filepath.Join(apDataDir, "products", "widget")
		require.NoError(t, os.MkdirAll(apProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(apProductDir, "product.yaml"), []byte(testProductWithAttrPricesYAML), 0644))

		h := &Handler{dataDir: apDataDir, resp: resp, geo: &geoDetectorStub{country: "ua"}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/widget/en", nil)
		r.SetPathValue("id", "widget")
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		attrPrices, ok := body["attr_prices"].(map[string]any)
		require.True(t, ok, "attr_prices should be present")
		displayColor, ok := attrPrices["display_color"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, 5.0, displayColor["red"])
		assert.Equal(t, 3.0, displayColor["green"])
	})

	main.Run("AttrImagePathsAreTransformed", func(t *testing.T) {
		aiDataDir := t.TempDir()
		aiProductDir := filepath.Join(aiDataDir, "products", "widget")
		require.NoError(t, os.MkdirAll(aiProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(aiProductDir, "product.yaml"),
			[]byte(testProductWithAttrImagesYAML), 0644))

		h := &Handler{dataDir: aiDataDir, resp: resp, geo: &geoDetectorStub{}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/widget/en", nil)
		r.SetPathValue("id", "widget")
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

		attrImages, ok := body["attr_images"].(map[string]any)
		require.True(t, ok, "attr_images field should be present")
		displayColor, ok := attrImages["display_color"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "/images/widget/red-thumb.jpg", displayColor["red"])
		assert.Equal(t, "/images/widget/green-thumb.jpg", displayColor["green"])
	})

	main.Run("ReturnsAttrDescription", func(t *testing.T) {
		adDataDir := t.TempDir()
		adProductDir := filepath.Join(adDataDir, "products", "widget")
		require.NoError(t, os.MkdirAll(adProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(adProductDir, "product.yaml"), []byte(testProductWithAttrDescriptionYAML), 0644))

		h := &Handler{dataDir: adDataDir, resp: resp, geo: &geoDetectorStub{}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/widget/en", nil)
		r.SetPathValue("id", "widget")
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		attrs, ok := body["attrs"].(map[string]any)
		require.True(t, ok, "attrs should be present")
		displayColor, ok := attrs["display_color"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Display color", displayColor["title"])
		assert.Equal(t, "Pick a color carefully.", displayColor["description"])
	})

	main.Run("ReturnsAttrValuesOrder", func(t *testing.T) {
		avoDataDir := t.TempDir()
		avoProductDir := filepath.Join(avoDataDir, "products", "widget")
		require.NoError(t, os.MkdirAll(avoProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(avoProductDir, "product.yaml"), []byte(testProductWithAttrValuesOrderYAML), 0644))

		h := &Handler{dataDir: avoDataDir, resp: resp, geo: &geoDetectorStub{}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/widget/en", nil)
		r.SetPathValue("id", "widget")
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		attrValuesOrder, ok := body["attr_values_order"].(map[string]any)
		require.True(t, ok, "attr_values_order should be present")
		displayColor, ok := attrValuesOrder["display_color"].([]any)
		require.True(t, ok, "display_color order should be present")
		assert.Equal(t, []any{"blue", "red", "green"}, displayColor)
	})

	main.Run("ReturnsAttrPricesWithDefaultFallback", func(t *testing.T) {
		apDataDir := t.TempDir()
		apProductDir := filepath.Join(apDataDir, "products", "widget")
		require.NoError(t, os.MkdirAll(apProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(apProductDir, "product.yaml"), []byte(testProductWithAttrPricesYAML), 0644))

		h := &Handler{dataDir: apDataDir, resp: resp, geo: &geoDetectorStub{country: "xx"}, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/widget/en", nil)
		r.SetPathValue("id", "widget")
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServeProductContent(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		attrPrices, ok := body["attr_prices"].(map[string]any)
		require.True(t, ok, "attr_prices should be present")
		displayColor, ok := attrPrices["display_color"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, 10.0, displayColor["red"])
		assert.Equal(t, 8.0, displayColor["green"])
	})
}
