package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/ashep/simshop/internal/order"
	"github.com/ashep/simshop/internal/product"
)

type orderService interface {
	Submit(ctx context.Context, o order.Order) error
	List(ctx context.Context) ([]order.Record, error)
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
	if errors.Is(err, fs.ErrNotExist) {
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

	// Resolve attributes into rendered title pairs and validate them, sorted by attribute key for stable output.
	attrKeys := make([]string, 0, len(req.Attributes))
	for k := range req.Attributes {
		attrKeys = append(attrKeys, k)
	}
	sort.Strings(attrKeys)

	// Resolve base price by request-supplied country with "default" fallback.
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

	if err := h.orders.Submit(r.Context(), o); err != nil {
		h.l.Error().Err(err).Msg("submit order failed")
		h.writeError(w, &BadGatewayError{Reason: "failed to submit order"})
		return
	}

	if err := h.resp.Write(w, r, http.StatusCreated, &createOrderResponse{PaymentURL: "https://foo.bar"}); err != nil {
		h.l.Error().Err(err).Msg("write create order response failed")
		h.writeError(w, err)
	}
}
