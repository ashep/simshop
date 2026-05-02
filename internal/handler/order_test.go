package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ashep/simshop/internal/monobank"
	"github.com/ashep/simshop/internal/order"
	"github.com/ashep/simshop/internal/shop"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type orderServiceMock struct{ mock.Mock }

func (m *orderServiceMock) Submit(ctx context.Context, o order.Order) (string, error) {
	args := m.Called(ctx, o)
	return args.String(0), args.Error(1)
}

func (m *orderServiceMock) AttachInvoice(ctx context.Context, orderID string, inv order.Invoice) error {
	return m.Called(ctx, orderID, inv).Error(0)
}

func (m *orderServiceMock) List(ctx context.Context) ([]order.Record, error) {
	args := m.Called(ctx)
	v, _ := args.Get(0).([]order.Record)
	return v, args.Error(1)
}

func (m *orderServiceMock) GetStatus(ctx context.Context, id string) (string, error) {
	args := m.Called(ctx, id)
	return args.String(0), args.Error(1)
}

func (m *orderServiceMock) RecordInvoiceEvent(ctx context.Context, evt order.InvoiceEvent) error {
	return m.Called(ctx, evt).Error(0)
}

func (m *orderServiceMock) UpdateStatus(
	ctx context.Context,
	orderID, target, note, trackingNumber string,
) (bool, error) {
	args := m.Called(ctx, orderID, target, note, trackingNumber)
	return args.Bool(0), args.Error(1)
}

type monobankClientMock struct{ mock.Mock }

func (m *monobankClientMock) CreateInvoice(ctx context.Context, req monobank.CreateInvoiceRequest) (*monobank.CreateInvoiceResponse, error) {
	args := m.Called(ctx, req)
	v, _ := args.Get(0).(*monobank.CreateInvoiceResponse)
	return v, args.Error(1)
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

	resp := buildTestResponder(main)
	shopStub := &shopServiceStub{shop: &shop.Shop{
		Name: map[string]string{"en": "Test Shop"},
		Countries: map[string]*shop.Country{
			"ua": {Name: map[string]string{"en": "Ukraine"}, PhoneCode: "+380"},
			"us": {Name: map[string]string{"en": "United States"}, PhoneCode: "+1"},
		},
	}}

	doRequest := func(t *testing.T, dataDir string, svc *orderServiceMock, mb *monobankClientMock, body string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{
			orders:      svc,
			monobank:    mb,
			shop:        shopStub,
			geo:         &geoDetectorStub{},
			dataDir:     dataDir,
			redirectURL: "https://test.example/thanks",
			publicURL:   "https://test.example",
			webhookURL:  "https://test.example/monobank/webhook",
			resp:        resp,
			l:           zerolog.Nop(),
		}
		r := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateOrder(w, r)
		return w
	}

	main.Run("Returns201OnSuccess", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		defer svc.AssertExpectations(t)
		defer mb.AssertExpectations(t)
		svc.On("Submit", mock.Anything, mock.MatchedBy(func(o order.Order) bool {
			return o.ProductID == "widget" &&
				len(o.Attrs) == 0 &&
				o.Price == 4999 &&
				o.Currency == "USD" &&
				o.Lang == "en" &&
				o.Country == "ua" &&
				o.FirstName == "Іван" &&
				o.LastName == "Іваненко" &&
				o.Phone == "+380501234567" &&
				o.Email == "ivan@example.com" &&
				o.City == "Київ" &&
				o.Address == "Відділення №5"
		})).Return("018f4e3a-0000-7000-8000-000000000001", nil)
		mb.On("CreateInvoice", mock.Anything, mock.MatchedBy(func(req monobank.CreateInvoiceRequest) bool {
			return req.WebHookURL == "https://test.example/monobank/webhook"
		})).Return(&monobank.CreateInvoiceResponse{InvoiceID: "inv-1", PageURL: "https://pay.example/inv-1"}, nil)
		svc.On("AttachInvoice", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget",
			"lang": "en",
			"first_name": "Іван",
			"last_name": "Іваненко",
			"phone": "+380501234567",
			"email": "ivan@example.com",
			"country": "ua",
			"city": "Київ",
			"address": "Відділення №5"
		}`)
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.JSONEq(t, `{"payment_url": "https://pay.example/inv-1"}`, w.Body.String())
	})

	main.Run("ResolvesAndFormatsAttributes", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		defer svc.AssertExpectations(t)
		defer mb.AssertExpectations(t)
		// display_color red: base 49.99 + attr_price default 10 = 59.99 → 5999 cents.
		svc.On("Submit", mock.Anything, mock.MatchedBy(func(o order.Order) bool {
			return len(o.Attrs) == 1 &&
				o.Attrs[0].Name == "Display color" &&
				o.Attrs[0].Value == "Red" &&
				o.Attrs[0].Price == 1000 &&
				o.Price == 5999 &&
				o.Currency == "USD"
		})).Return("018f4e3a-0000-7000-8000-000000000001", nil)
		mb.On("CreateInvoice", mock.Anything, mock.Anything).Return(&monobank.CreateInvoiceResponse{InvoiceID: "inv-1", PageURL: "https://pay.example/inv-1"}, nil)
		svc.On("AttachInvoice", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		w := doRequest(t, attrDataDir, svc, mb, `{
			"product_id": "widget",
			"lang": "en",
			"attributes": {"display_color": "red"},
			"first_name": "A",
			"last_name": "B",
			"phone": "1",
			"email": "a@example.com",
			"country": "us",
			"city": "C",
			"address": "D"
		}`)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	main.Run("BasketItemNameIncludesAttrs", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		defer svc.AssertExpectations(t)
		defer mb.AssertExpectations(t)
		svc.On("Submit", mock.Anything, mock.Anything).Return("018f4e3a-0000-7000-8000-000000000001", nil)
		mb.On("CreateInvoice", mock.Anything, mock.MatchedBy(func(req monobank.CreateInvoiceRequest) bool {
			return len(req.MerchantPaymInfo.BasketOrder) == 1 &&
				req.MerchantPaymInfo.BasketOrder[0].Name == "Widget (Display color: Red)"
		})).Return(&monobank.CreateInvoiceResponse{InvoiceID: "inv-1", PageURL: "https://pay.example/inv-1"}, nil)
		svc.On("AttachInvoice", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		w := doRequest(t, attrDataDir, svc, mb, `{
			"product_id": "widget",
			"lang": "en",
			"attributes": {"display_color": "red"},
			"first_name": "A",
			"last_name": "B",
			"phone": "1",
			"email": "a@example.com",
			"country": "us",
			"city": "C",
			"address": "D"
		}`)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	main.Run("BasketItemIconWhenPublicURLSet", func(t *testing.T) {
		// Product YAML with images, no attrs.
		imgDir := main.TempDir()
		imgProductDir := filepath.Join(imgDir, "products", "widget")
		require.NoError(t, os.MkdirAll(imgProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(imgProductDir, "product.yaml"), []byte(`
name:
  en: Widget
description:
  en: A test product
prices:
  default:
    currency: USD
    value: 49.99
images:
  - preview: thumb.jpg
    full: full.jpg
`), 0644))

		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		defer svc.AssertExpectations(t)
		defer mb.AssertExpectations(t)
		svc.On("Submit", mock.Anything, mock.Anything).Return("018f4e3a-0000-7000-8000-000000000001", nil)
		mb.On("CreateInvoice", mock.Anything, mock.MatchedBy(func(req monobank.CreateInvoiceRequest) bool {
			return len(req.MerchantPaymInfo.BasketOrder) == 1 &&
				req.MerchantPaymInfo.BasketOrder[0].Icon == "https://shop.example/images/widget/thumb.jpg"
		})).Return(&monobank.CreateInvoiceResponse{InvoiceID: "inv-1", PageURL: "https://pay.example/inv-1"}, nil)
		svc.On("AttachInvoice", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		h := &Handler{
			orders:      svc,
			monobank:    mb,
			shop:        shopStub,
			geo:         &geoDetectorStub{},
			dataDir:     imgDir,
			redirectURL: "https://test.example/thanks",
			webhookURL:  "https://test.example/monobank/webhook",
			publicURL:   "https://shop.example",
			resp:        resp,
			l:           zerolog.Nop(),
		}
		r := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com",
			"country": "ua", "city": "C", "address": "D"
		}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateOrder(w, r)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	main.Run("BasketItemNoIconWhenProductHasNoImages", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		defer svc.AssertExpectations(t)
		defer mb.AssertExpectations(t)
		svc.On("Submit", mock.Anything, mock.Anything).Return("018f4e3a-0000-7000-8000-000000000001", nil)
		mb.On("CreateInvoice", mock.Anything, mock.MatchedBy(func(req monobank.CreateInvoiceRequest) bool {
			return req.MerchantPaymInfo.BasketOrder[0].Icon == ""
		})).Return(&monobank.CreateInvoiceResponse{InvoiceID: "inv-1", PageURL: "https://pay.example/inv-1"}, nil)
		svc.On("AttachInvoice", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// baseDataDir uses testProductYAML which has no images; publicURL is set but should not produce icon.
		h := &Handler{
			orders:      svc,
			monobank:    mb,
			shop:        shopStub,
			geo:         &geoDetectorStub{},
			dataDir:     baseDataDir,
			redirectURL: "https://test.example/thanks",
			webhookURL:  "https://test.example/monobank/webhook",
			publicURL:   "https://shop.example",
			resp:        resp,
			l:           zerolog.Nop(),
		}
		r := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com",
			"country": "ua", "city": "C", "address": "D"
		}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateOrder(w, r)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	main.Run("BasketItemNameWithoutAttrsIsBareTitle", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		defer svc.AssertExpectations(t)
		defer mb.AssertExpectations(t)
		svc.On("Submit", mock.Anything, mock.Anything).Return("018f4e3a-0000-7000-8000-000000000001", nil)
		mb.On("CreateInvoice", mock.Anything, mock.MatchedBy(func(req monobank.CreateInvoiceRequest) bool {
			return len(req.MerchantPaymInfo.BasketOrder) == 1 &&
				req.MerchantPaymInfo.BasketOrder[0].Name == "Widget"
		})).Return(&monobank.CreateInvoiceResponse{InvoiceID: "inv-1", PageURL: "https://pay.example/inv-1"}, nil)
		svc.On("AttachInvoice", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget",
			"lang": "en",
			"first_name": "A",
			"last_name": "B",
			"phone": "1",
			"email": "a@example.com",
			"country": "ua",
			"city": "C",
			"address": "D"
		}`)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	main.Run("PassesRequestCountryToOrder", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		defer svc.AssertExpectations(t)
		defer mb.AssertExpectations(t)
		svc.On("Submit", mock.Anything, mock.MatchedBy(func(o order.Order) bool {
			return o.Country == "ua"
		})).Return("018f4e3a-0000-7000-8000-000000000001", nil)
		mb.On("CreateInvoice", mock.Anything, mock.Anything).Return(&monobank.CreateInvoiceResponse{InvoiceID: "inv-1", PageURL: "https://pay.example/inv-1"}, nil)
		svc.On("AttachInvoice", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// Geo stub returns "xx"; the request says "ua"; the stored country must be "ua".
		h := &Handler{
			orders:      svc,
			monobank:    mb,
			shop:        shopStub,
			geo:         &geoDetectorStub{country: "xx"},
			dataDir:     baseDataDir,
			redirectURL: "https://test.example/thanks",
			webhookURL:  "https://test.example/monobank/webhook",
			resp:        resp,
			l:           zerolog.Nop(),
		}
		r := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "ua", "city": "C", "address": "D"
		}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateOrder(w, r)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	main.Run("MissingProductIDReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingLangReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingFirstNameReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget", "lang": "en",
			"last_name": "B", "phone": "1", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingLastNameReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "phone": "1", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingPhoneReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingEmailReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingCountryReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingCityReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "ua", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("MissingAddressReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "ua", "city": "C"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("CountryNotInAllowedListReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		// "fr" is not in shopStub.Countries (ua, us)
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "fr", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid country")
	})

	main.Run("UnknownProductReturns404", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "no-such-product", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("UnknownLangReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget", "lang": "fr",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("UnknownAttrIDReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, attrDataDir, svc, mb, `{
			"product_id": "widget", "lang": "en",
			"attributes": {"no_such_attr": "red"},
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("UnknownAttrValueReturns400", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, attrDataDir, svc, mb, `{
			"product_id": "widget", "lang": "en",
			"attributes": {"display_color": "no_such_value"},
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	main.Run("PathTraversalInProductIDReturns404", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "../widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("ServiceErrorReturns502", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		defer svc.AssertExpectations(t)
		svc.On("Submit", mock.Anything, mock.Anything).Return("", assert.AnError)

		w := doRequest(t, baseDataDir, svc, mb, `{
			"product_id": "widget", "lang": "en",
			"first_name": "A", "last_name": "B", "phone": "1", "email": "a@b.com", "country": "ua", "city": "C", "address": "D"
		}`)
		assert.Equal(t, http.StatusBadGateway, w.Code)
	})

	main.Run("ZeroAmount_Returns400", func(t *testing.T) {
		// product with price 0 → reject before any DB call.
		zeroDir := main.TempDir()
		zeroProductDir := filepath.Join(zeroDir, "products", "widget")
		require.NoError(t, os.MkdirAll(zeroProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(zeroProductDir, "product.yaml"), []byte(`
name:
  en: Widget
description:
  en: A test product
prices:
  default:
    currency: USD
    value: 0.00
`), 0644))

		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		w := doRequest(t, zeroDir, svc, mb, `{"product_id":"widget","lang":"en","first_name":"A","last_name":"B","phone":"1","email":"a@b","country":"us","city":"X","address":"Y"}`)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "order amount must be positive")
		svc.AssertNotCalled(t, "Submit", mock.Anything, mock.Anything)
		mb.AssertNotCalled(t, "CreateInvoice", mock.Anything, mock.Anything)
	})

	main.Run("MonobankSuccess_Returns201WithPageURL", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}

		svc.On("Submit", mock.Anything, mock.Anything).Return("018f4e3a-0000-7000-8000-000000000099", nil)
		mb.On("CreateInvoice", mock.Anything, mock.MatchedBy(func(req monobank.CreateInvoiceRequest) bool {
			return req.Amount == 4999 &&
				req.Ccy == 840 &&
				req.MerchantPaymInfo.Reference == "018f4e3a-0000-7000-8000-000000000099" &&
				req.MerchantPaymInfo.Destination == "Test Shop, order 018f4e3a-0000" &&
				req.RedirectURL == "https://test.example/thanks?order_id=018f4e3a-0000-7000-8000-000000000099"
		})).Return(&monobank.CreateInvoiceResponse{InvoiceID: "inv-1", PageURL: "https://pay.example/inv-1"}, nil)
		svc.On("AttachInvoice", mock.Anything, "018f4e3a-0000-7000-8000-000000000099", mock.MatchedBy(func(inv order.Invoice) bool {
			return inv.Provider == "monobank" && inv.ID == "inv-1" && inv.PageURL == "https://pay.example/inv-1" && inv.Amount == 4999 && inv.Currency == "USD"
		})).Return(nil)

		w := doRequest(t, baseDataDir, svc, mb, `{"product_id":"widget","lang":"en","first_name":"A","last_name":"B","phone":"1","email":"a@b","country":"us","city":"X","address":"Y"}`)

		require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
		assert.JSONEq(t, `{"payment_url":"https://pay.example/inv-1"}`, w.Body.String())
		svc.AssertExpectations(t)
		mb.AssertExpectations(t)
	})

	main.Run("MonobankFailure_Returns502_NoAttach", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		svc.On("Submit", mock.Anything, mock.Anything).Return("018f4e3a-0000-7000-8000-000000000098", nil)
		mb.On("CreateInvoice", mock.Anything, mock.Anything).Return((*monobank.CreateInvoiceResponse)(nil), &monobank.APIError{Status: 500, ErrCode: "limit_exceeded", ErrText: "x"})

		w := doRequest(t, baseDataDir, svc, mb, `{"product_id":"widget","lang":"en","first_name":"A","last_name":"B","phone":"1","email":"a@b","country":"us","city":"X","address":"Y"}`)

		require.Equal(t, http.StatusBadGateway, w.Code)
		assert.JSONEq(t, `{"error":"bad gateway"}`, w.Body.String())
		svc.AssertCalled(t, "Submit", mock.Anything, mock.Anything)
		svc.AssertNotCalled(t, "AttachInvoice", mock.Anything, mock.Anything, mock.Anything)
	})

	main.Run("CurrencyUnsupported_Returns502", func(t *testing.T) {
		// Product with currency JPY (not in MapCurrency) → 502, never call Monobank.
		jpyDir := main.TempDir()
		jpyProductDir := filepath.Join(jpyDir, "products", "widget")
		require.NoError(t, os.MkdirAll(jpyProductDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(jpyProductDir, "product.yaml"), []byte(`
name:
  en: Widget
description:
  en: A test product
prices:
  default:
    currency: JPY
    value: 49.99
`), 0644))

		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		svc.On("Submit", mock.Anything, mock.Anything).Return("018f4e3a-0000-7000-8000-000000000097", nil)

		w := doRequest(t, jpyDir, svc, mb, `{"product_id":"widget","lang":"en","first_name":"A","last_name":"B","phone":"1","email":"a@b","country":"us","city":"X","address":"Y"}`)

		require.Equal(t, http.StatusBadGateway, w.Code)
		assert.JSONEq(t, `{"error":"bad gateway"}`, w.Body.String())
		mb.AssertNotCalled(t, "CreateInvoice", mock.Anything, mock.Anything)
	})

	main.Run("AttachInvoiceFailure_Returns502", func(t *testing.T) {
		svc := &orderServiceMock{}
		mb := &monobankClientMock{}
		svc.On("Submit", mock.Anything, mock.Anything).Return("018f4e3a-0000-7000-8000-000000000096", nil)
		mb.On("CreateInvoice", mock.Anything, mock.Anything).Return(&monobank.CreateInvoiceResponse{InvoiceID: "inv-2", PageURL: "https://pay.example/inv-2"}, nil)
		svc.On("AttachInvoice", mock.Anything, "018f4e3a-0000-7000-8000-000000000096", mock.Anything).Return(errors.New("db down"))

		w := doRequest(t, baseDataDir, svc, mb, `{"product_id":"widget","lang":"en","first_name":"A","last_name":"B","phone":"1","email":"a@b","country":"us","city":"X","address":"Y"}`)

		require.Equal(t, http.StatusBadGateway, w.Code)
		assert.JSONEq(t, `{"error":"bad gateway"}`, w.Body.String())
	})
}
