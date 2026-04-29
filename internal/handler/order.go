package handler

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/ashep/simshop/internal/monobank"
	"github.com/ashep/simshop/internal/order"
	"github.com/ashep/simshop/internal/product"
)

type orderService interface {
	Submit(ctx context.Context, o order.Order) (string, error)
	AttachInvoice(ctx context.Context, orderID string, inv order.Invoice) error
	List(ctx context.Context) ([]order.Record, error)
	GetStatus(ctx context.Context, orderID string) (string, error)
	RecordInvoiceEvent(ctx context.Context, evt order.InvoiceEvent) error
}

type monobankClient interface {
	CreateInvoice(ctx context.Context, req monobank.CreateInvoiceRequest) (*monobank.CreateInvoiceResponse, error)
}

type createOrderResponse struct {
	PaymentURL string `json:"payment_url"`
}

type createOrderRequest struct {
	ProductID  string            `json:"product_id"`
	Lang       string            `json:"lang"`
	Attributes map[string]string `json:"attributes"`
	FirstName  string            `json:"first_name"`
	MiddleName string            `json:"middle_name"`
	LastName   string            `json:"last_name"`
	Phone      string            `json:"phone"`
	Email      string            `json:"email"`
	Country    string            `json:"country"`
	City       string            `json:"city"`
	Address    string            `json:"address"`
	Notes      string            `json:"notes"`
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, &BadRequestError{Reason: "invalid request body"})
		return
	}

	switch {
	case req.ProductID == "":
		h.writeError(w, &BadRequestError{Reason: "product_id is required"})
		return
	case req.Lang == "":
		h.writeError(w, &BadRequestError{Reason: "lang is required"})
		return
	case req.FirstName == "":
		h.writeError(w, &BadRequestError{Reason: "first_name is required"})
		return
	case req.LastName == "":
		h.writeError(w, &BadRequestError{Reason: "last_name is required"})
		return
	case req.Phone == "":
		h.writeError(w, &BadRequestError{Reason: "phone is required"})
		return
	case req.Email == "":
		h.writeError(w, &BadRequestError{Reason: "email is required"})
		return
	case req.Country == "":
		h.writeError(w, &BadRequestError{Reason: "country is required"})
		return
	case req.City == "":
		h.writeError(w, &BadRequestError{Reason: "city is required"})
		return
	case req.Address == "":
		h.writeError(w, &BadRequestError{Reason: "address is required"})
		return
	}

	s, err := h.shop.Get(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	if _, ok := s.Countries[req.Country]; !ok {
		h.writeError(w, &BadRequestError{Reason: "invalid country"})
		return
	}

	if req.ProductID != filepath.Base(req.ProductID) || req.ProductID == "." {
		h.writeError(w, &NotFoundError{Reason: "product not found"})
		return
	}

	data, err := os.ReadFile(filepath.Join(h.dataDir, "products", req.ProductID, "product.yaml"))
	if stderrors.Is(err, fs.ErrNotExist) {
		h.writeError(w, &NotFoundError{Reason: "product not found"})
		return
	}
	if err != nil {
		h.writeError(w, err)
		return
	}

	var p product.Product
	if err := yaml.Unmarshal(data, &p); err != nil {
		h.writeError(w, err)
		return
	}

	if _, ok := p.Name[req.Lang]; !ok {
		h.writeError(w, &BadRequestError{Reason: "language not found"})
		return
	}

	attrKeys := make([]string, 0, len(req.Attributes))
	for k := range req.Attributes {
		attrKeys = append(attrKeys, k)
	}
	sort.Strings(attrKeys)

	country := req.Country
	price, ok := p.Prices[country]
	if !ok {
		price = p.Prices["default"]
	}

	totalCents := int(math.Round(price.Value * 100))
	attrs := make([]order.Attr, 0, len(attrKeys))
	for _, attrID := range attrKeys {
		valueID := req.Attributes[attrID]

		langAttrs, ok := p.Attrs[attrID]
		if !ok {
			h.writeError(w, &BadRequestError{Reason: fmt.Sprintf("unknown attribute: %s", attrID)})
			return
		}
		attrLang, ok := langAttrs[req.Lang]
		if !ok {
			h.writeError(w, &BadRequestError{Reason: fmt.Sprintf("no %q language for attribute: %s", req.Lang, attrID)})
			return
		}
		attrVal, ok := attrLang.Values[valueID]
		if !ok {
			h.writeError(w, &BadRequestError{Reason: fmt.Sprintf("unknown value %q for attribute: %s", valueID, attrID)})
			return
		}

		var addonValue float64
		if valuePrices, ok := p.AttrPrices[attrID]; ok {
			if countryPrices, ok := valuePrices[valueID]; ok {
				addonValue, ok = countryPrices[country]
				if !ok {
					addonValue = countryPrices["default"]
				}
			}
		}
		addonCents := int(math.Round(addonValue * 100))
		totalCents += addonCents

		attrs = append(attrs, order.Attr{
			Name:  attrLang.Title,
			Value: attrVal.Title,
			Price: addonCents,
		})
	}

	if totalCents <= 0 {
		h.writeError(w, &BadRequestError{Reason: "order amount must be positive"})
		return
	}

	o := order.Order{
		ProductID:    req.ProductID,
		Email:        req.Email,
		Price:        totalCents,
		Currency:     price.Currency,
		FirstName:    req.FirstName,
		MiddleName:   req.MiddleName,
		LastName:     req.LastName,
		Country:      country,
		City:         req.City,
		Phone:        req.Phone,
		Address:      req.Address,
		Attrs:        attrs,
		CustomerNote: req.Notes,
	}

	orderID, err := h.orders.Submit(r.Context(), o)
	if err != nil {
		h.l.Error().Err(err).Msg("submit order failed")
		h.writeError(w, &BadGatewayError{Reason: "bad gateway"})
		return
	}

	ccy, err := monobank.MapCurrency(price.Currency)
	if err != nil {
		h.logMonobankError(err, orderID)
		h.writeError(w, &BadGatewayError{Reason: "bad gateway"})
		return
	}

	destination := buildDestination(s.Name, req.Lang, orderID)
	redirect, err := buildRedirect(h.redirectURL, orderID)
	if err != nil {
		h.l.Error().Err(err).Str("order_id", orderID).Msg("build redirect url failed")
		h.writeError(w, &BadGatewayError{Reason: "bad gateway"})
		return
	}

	productTitle := p.Name[req.Lang]
	inv, err := h.monobank.CreateInvoice(r.Context(), monobank.CreateInvoiceRequest{
		Amount: totalCents,
		Ccy:    ccy,
		MerchantPaymInfo: monobank.MerchantPaymInfo{
			Reference:   orderID,
			Destination: destination,
			BasketOrder: []monobank.BasketItem{{Name: productTitle, Qty: 1, Sum: totalCents, Code: req.ProductID, Tax: h.taxIDs}},
		},
		RedirectURL: redirect,
		WebHookURL:  h.webhookURL,
	})
	if err != nil {
		h.logMonobankError(err, orderID)
		h.writeError(w, &BadGatewayError{Reason: "bad gateway"})
		return
	}

	if err := h.orders.AttachInvoice(r.Context(), orderID, order.Invoice{
		Provider: "monobank",
		ID:       inv.InvoiceID,
		PageURL:  inv.PageURL,
		Amount:   totalCents,
		Currency: price.Currency,
	}); err != nil {
		h.l.Error().Err(err).
			Str("order_id", orderID).
			Str("invoice_id", inv.InvoiceID).
			Str("page_url", inv.PageURL).
			Msg("attach invoice failed; monobank invoice will expire unattached")
		h.writeError(w, &BadGatewayError{Reason: "bad gateway"})
		return
	}

	if err := h.resp.Write(w, r, http.StatusCreated, &createOrderResponse{PaymentURL: inv.PageURL}); err != nil {
		h.l.Error().Err(err).Msg("write create order response failed")
		h.writeError(w, err)
	}
}

// logMonobankError emits a structured error log including any *monobank.APIError
// fields.
func (h *Handler) logMonobankError(err error, orderID string) {
	ev := h.l.Error().Err(err).Str("order_id", orderID)
	var apiErr *monobank.APIError
	if stderrors.As(err, &apiErr) {
		ev = ev.Int("monobank_status", apiErr.Status).
			Str("monobank_err_code", apiErr.ErrCode).
			Str("monobank_err_text", apiErr.ErrText).
			Str("monobank_body", apiErr.Body)
	}
	ev.Msg("monobank invoice flow failed")
}

// buildDestination renders the Monobank "destination" string shown to the
// customer in their bank app. Falls back to the alphabetically first language
// in shopName if req.Lang is missing.
func buildDestination(shopName map[string]string, lang, orderID string) string {
	name := shopName[lang]
	if name == "" {
		keys := make([]string, 0, len(shopName))
		for k := range shopName {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			name = shopName[keys[0]]
		}
	}
	short := orderID
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("%s, order %s", name, short)
}

// buildRedirect appends ?order_id=<orderID> to base.
func buildRedirect(base, orderID string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse redirect url: %w", err)
	}
	q := u.Query()
	q.Set("order_id", orderID)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
