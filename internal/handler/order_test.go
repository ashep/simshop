package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ashep/simshop/internal/order"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type orderServiceMock struct{ mock.Mock }

func (m *orderServiceMock) Submit(ctx context.Context, o order.Order) error {
	return m.Called(ctx, o).Error(0)
}

func TestCreateOrder(main *testing.T) {
	// Shared product directory with testProductYAML (no attrs).
	baseDataDir := main.TempDir()
	productDir := filepath.Join(baseDataDir, "products", "widget")
	require.NoError(main, os.MkdirAll(productDir, 0755))
	require.NoError(main, os.WriteFile(filepath.Join(productDir, "product.yaml"), []byte(testProductYAML), 0644))

	// Separate directory with testProductWithAttrPricesYAML for attribute tests.
	attrDataDir := main.TempDir()
	attrProductDir := filepath.Join(attrDataDir, "products", "widget")
	require.NoError(main, os.MkdirAll(attrProductDir, 0755))
	require.NoError(main, os.WriteFile(filepath.Join(attrProductDir, "product.yaml"), []byte(testProductWithAttrPricesYAML), 0644))

	doRequest := func(t *testing.T, dataDir string, svc *orderServiceMock, body string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{orders: svc, geo: &geoDetectorStub{}, dataDir: dataDir, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateOrder(w, r)
		return w
	}

	main.Run("Returns201OnSuccess", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Submit", mock.Anything, mock.MatchedBy(func(o order.Order) bool {
			return o.ProductName == "Widget" &&
				o.Attributes == "" &&
				o.Price == 49.99 &&
				o.Currency == "USD" &&
				o.FirstName == "Іван" &&
				o.LastName == "Іваненко" &&
				o.Phone == "+380501234567" &&
				o.Email == "ivan@example.com" &&
				o.City == "Київ" &&
				o.Address == "Відділення №5"
		})).Return(nil)

		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "widget",
			"lang": "en",
			"first_name": "Іван",
			"last_name": "Іваненко",
			"phone": "+380501234567",
			"email": "ivan@example.com",
			"city": "Київ",
			"address": "Відділення №5"
		}`)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	main.Run("ResolvesAndFormatsAttributes", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		// display_color red: base 49.99 + attr_price default 10 = 59.99; geo returns "" → falls back to default
		svc.On("Submit", mock.Anything, mock.MatchedBy(func(o order.Order) bool {
			return o.Attributes == "Display color: Red" &&
				o.Price == 59.99 &&
				o.Currency == "USD"
		})).Return(nil)

		w := doRequest(t, attrDataDir, svc, `{
			"product_id": "widget",
			"lang": "en",
			"attributes": {"display_color": "red"},
			"first_name": "A",
			"last_name": "B",
			"phone": "1",
			"email": "a@example.com",
			"city": "C",
			"address": "D"
		}`)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	main.Run("MissingProductIDReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, baseDataDir, svc, `{
			"lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingLangReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "widget",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingFirstNameReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "widget", "lang": "en",
			"last_name": "B", "phone": "1", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingLastNameReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "phone": "1", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingPhoneReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingEmailReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingCityReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingAddressReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "city": "C"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("UnknownProductReturns404", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "no-such-product", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("UnknownLangReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "widget", "lang": "fr",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("UnknownAttrIDReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, attrDataDir, svc, `{
			"product_id": "widget", "lang": "en",
			"attributes": {"no_such_attr": "red"},
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("UnknownAttrValueReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, attrDataDir, svc, `{
			"product_id": "widget", "lang": "en",
			"attributes": {"display_color": "no_such_value"},
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("PathTraversalInProductIDReturns404", func(t *testing.T) {
		svc := &orderServiceMock{}
		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "../widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("ServiceErrorReturns502", func(t *testing.T) {
		svc := &orderServiceMock{}
		defer svc.AssertExpectations(t)
		svc.On("Submit", mock.Anything, mock.Anything).Return(assert.AnError)

		w := doRequest(t, baseDataDir, svc, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadGateway, w.Code)
	})
}
