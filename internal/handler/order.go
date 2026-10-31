package handler

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io/fs"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ashep/simshop/internal/monobank"
	"github.com/ashep/simshop/internal/order"
	"github.com/ashep/simshop/internal/product"
)

type orderService interface {
	Submit(ctx context.Context, o order.Order) (string, error)
	AttachInvoice(ctx context.Context, orderID string, inv order.Invoice) error
	List(ctx context.Context, statuses []string) ([]order.Record, error)
	GetStatus(ctx context.Context, orderID string) (string, error)
	UpdateStatus(ctx context.Context, orderID, target, note, trackingNumber string) (bool, error)
	RecordInvoiceEvent(ctx context.Context, evt order.InvoiceEvent) error
}

type monobankClient interface {
	CreateInvoice(ctx context.Context, req monobank.CreateInvoiceRequest) (*monobank.CreateInvoiceResponse, error)
}

type createOrderResponse struct {
	PaymentURL string `json:"payment_url"`
}

// maxOrderItems and maxItemQty bound a cart; the OpenAPI spec mirrors them.
const (
	maxOrderItems = 20
	maxItemQty    = 99
)

type createOrderItem struct {
	ProductID  string            `json:"product_id"`
	Attributes map[string]string `json:"attributes"`
	Qty        int               `json:"qty"`
}

type createOrderRequest struct {
	Items       []createOrderItem `json:"items"`
	Lang        string            `json:"lang"`
	FirstName   string            `json:"first_name"`
	MiddleName  string            `json:"middle_name"`
	LastName    string            `json:"last_name"`
	Phone       string            `json:"phone"`
	Email       string            `json:"email"`
	Country     string            `json:"country"`
	City        string            `json:"city"`
	Address     string            `json:"address"`
	Notes       string            `json:"notes"`
	PromoCode   string            `json:"promo_code"`
	GAClientID  string            `json:"ga_client_id"`
	GASessionID string            `json:"ga_session_id"`
	MetaFBP     string            `json:"meta_fbp"`
	MetaFBC     string            `json:"meta_fbc"`
}

// pricedItem is a validated cart line with what the invoice needs about it.
type pricedItem struct {
	item     order.Item
	product  product.Product
	currency string
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, &BadRequestError{Reason: "invalid request body"})
		return
	}

	switch {
	case len(req.Items) == 0:
		h.writeError(w, &BadRequestError{Reason: "items are required"})
		return
	case len(req.Items) > maxOrderItems:
		h.writeError(w, &BadRequestError{Reason: "too many items"})
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

	lines := make([]pricedItem, 0, len(req.Items))
	for _, ri := range req.Items {
		line, err := h.priceItem(ri, req.Lang, req.Country)
		if err != nil {
			h.writeError(w, err)
			return
		}
		if len(lines) > 0 && line.currency != lines[0].currency {
			h.writeError(w, &BadRequestError{Reason: "items have different currencies"})
			return
		}
		lines = append(lines, line)
	}
	currency := lines[0].currency

	// A promo code applies to every item whose product lists it; the discount
	// is taken per unit so each line stays unit price × qty on the invoice.
	var promo *order.Promo
	if strings.TrimSpace(req.PromoCode) != "" {
		for i := range lines {
			code, percent, ok := lines[i].product.LookupPromo(req.PromoCode)
			if !ok {
				continue
			}
			it := &lines[i].item
			it.Discount = product.Discount(it.Price, percent) * it.Qty
			if promo == nil {
				promo = &order.Promo{Code: code, Percent: percent}
			}
			promo.Discount += it.Discount
		}
		if promo == nil {
			h.writeError(w, &BadRequestError{Reason: "invalid promo code"})
			return
		}
	}

	items := make([]order.Item, len(lines))
	totalCents := 0
	for i, line := range lines {
		items[i] = line.item
		totalCents += line.item.Total()
	}

	if totalCents <= 0 {
		h.writeError(w, &BadRequestError{Reason: "order amount must be positive"})
		return
	}

	o := order.Order{
		Items:        items,
		Email:        req.Email,
		Price:        totalCents,
		Currency:     currency,
		Lang:         req.Lang,
		FirstName:    req.FirstName,
		MiddleName:   req.MiddleName,
		LastName:     req.LastName,
		Country:      req.Country,
		City:         req.City,
		Phone:        req.Phone,
		Address:      req.Address,
		CustomerNote: req.Notes,
		Promo:        promo,
		GAClientID:   req.GAClientID,
		GASessionID:  req.GASessionID,
		MetaFBP:      req.MetaFBP,
		MetaFBC:      req.MetaFBC,
		ClientIP:     requestClientIP(r),
		UserAgent:    truncate(r.UserAgent(), maxUserAgentLen),
	}

	orderID, err := h.orders.Submit(r.Context(), o)
	if stderrors.Is(err, order.ErrPromoCodeUsed) {
		h.writeError(w, &BadRequestError{Reason: "promo code already used"})
		return
	}
	if err != nil {
		h.l.Error().Err(err).Msg("submit order failed")
		h.writeError(w, &BadGatewayError{Reason: "bad gateway"})
		return
	}

	ccy, err := monobank.MapCurrency(currency)
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

	basket := make([]monobank.BasketItem, len(lines))
	for i, line := range lines {
		it := line.item
		title := line.product.Name[req.Lang]
		if len(it.Attrs) > 0 {
			parts := make([]string, 0, len(it.Attrs))
			for _, a := range it.Attrs {
				parts = append(parts, fmt.Sprintf("%s: %s", a.Name, a.Value))
			}
			title = fmt.Sprintf("%s (%s)", title, strings.Join(parts, ", "))
		}
		var icon string
		if len(line.product.Images) > 0 && line.product.Images[0].Preview != "" {
			icon = fmt.Sprintf("%s/images/%s/%s", h.publicURL, it.ProductID, line.product.Images[0].Preview)
		}
		basket[i] = monobank.BasketItem{
			Name:  title,
			Qty:   it.Qty,
			Sum:   it.Total() / it.Qty,
			Total: it.Total(),
			Icon:  icon,
			Code:  it.ProductID,
			Tax:   h.taxIDs,
		}
	}

	inv, err := h.monobank.CreateInvoice(r.Context(), monobank.CreateInvoiceRequest{
		Amount: totalCents,
		Ccy:    ccy,
		MerchantPaymInfo: monobank.MerchantPaymInfo{
			Reference:   orderID,
			Destination: destination,
			BasketOrder: basket,
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
		Currency: currency,
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

// priceItem loads ri's product and prices one unit of it for country, with the
// selected attrs. The returned error is ready for writeError.
func (h *Handler) priceItem(ri createOrderItem, lang, country string) (pricedItem, error) {
	if ri.ProductID == "" {
		return pricedItem{}, &BadRequestError{Reason: "product_id is required"}
	}
	if ri.Qty < 1 || ri.Qty > maxItemQty {
		return pricedItem{}, &BadRequestError{Reason: "invalid qty"}
	}
	if ri.ProductID != filepath.Base(ri.ProductID) || ri.ProductID == "." {
		return pricedItem{}, &NotFoundError{Reason: "product not found"}
	}

	data, err := os.ReadFile(filepath.Join(h.dataDir, "products", ri.ProductID, "product.yaml"))
	if stderrors.Is(err, fs.ErrNotExist) {
		return pricedItem{}, &NotFoundError{Reason: "product not found"}
	}
	if err != nil {
		return pricedItem{}, err
	}

	var p product.Product
	if err := yaml.Unmarshal(data, &p); err != nil {
		return pricedItem{}, err
	}

	if p.Available != nil && !*p.Available {
		return pricedItem{}, &BadRequestError{Reason: "product not available"}
	}

	if _, ok := p.Name[lang]; !ok {
		return pricedItem{}, &BadRequestError{Reason: "language not found"}
	}

	attrKeys := make([]string, 0, len(ri.Attributes))
	for k := range ri.Attributes {
		attrKeys = append(attrKeys, k)
	}
	sort.Strings(attrKeys)

	price, ok := p.Prices[country]
	if !ok {
		price = p.Prices["default"]
	}

	unitCents := int(math.Round(price.Value * 100))
	attrs := make([]order.Attr, 0, len(attrKeys))
	for _, attrID := range attrKeys {
		valueID := ri.Attributes[attrID]

		langAttrs, ok := p.Attrs[attrID]
		if !ok {
			return pricedItem{}, &BadRequestError{Reason: fmt.Sprintf("unknown attribute: %s", attrID)}
		}
		attrLang, ok := langAttrs[lang]
		if !ok {
			return pricedItem{}, &BadRequestError{Reason: fmt.Sprintf("no %q language for attribute: %s", lang, attrID)}
		}
		attrVal, ok := attrLang.Values[valueID]
		if !ok {
			return pricedItem{}, &BadRequestError{Reason: fmt.Sprintf("unknown value %q for attribute: %s", valueID, attrID)}
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
		unitCents += addonCents

		attrs = append(attrs, order.Attr{
			Name:  attrLang.Title,
			Value: attrVal.Title,
			Price: addonCents,
		})
	}

	return pricedItem{
		item:     order.Item{ProductID: ri.ProductID, Qty: ri.Qty, Price: unitCents, Attrs: attrs},
		product:  p,
		currency: price.Currency,
	}, nil
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
	if len(short) > 13 {
		short = short[:13]
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

// maxUserAgentLen caps the stored User-Agent; real ones are well under it.
const maxUserAgentLen = 512

// requestClientIP returns the buyer's IP: Cloudflare's CF-Connecting-IP, then the
// first X-Forwarded-For hop (set by Caddy), then the TCP peer. Used only for ad
// conversion matching, so a spoofed header costs nothing but match quality.
func requestClientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); net.ParseIP(ip) != nil {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); net.ParseIP(ip) != nil {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && net.ParseIP(host) != nil {
		return host
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
