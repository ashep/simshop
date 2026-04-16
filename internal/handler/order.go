package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ashep/simshop/internal/order"
	"github.com/ashep/simshop/internal/product"
)

type orderService interface {
	Submit(ctx context.Context, o order.Order) error
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
	case req.City == "":
		h.writeError(w, &BadRequestError{Reason: "city is required"})
		return
	case req.Address == "":
		h.writeError(w, &BadRequestError{Reason: "address is required"})
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

	productName, ok := p.Name[req.Lang]
	if !ok {
		h.writeError(w, &BadRequestError{Reason: "language not found"})
		return
	}

	// Validate attributes and format as "AttrTitle: ValueTitle, ..." sorted by attribute key.
	attrKeys := make([]string, 0, len(req.Attributes))
	for k := range req.Attributes {
		attrKeys = append(attrKeys, k)
	}
	sort.Strings(attrKeys)

	var attrParts []string
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
		attrParts = append(attrParts, attrLang.Title+": "+attrVal.Title)
	}

	// Resolve base prices by geo-detected country with "default" fallback.
	country := h.geo.Detect(r)
	price, ok := p.Prices[country]
	if !ok {
		price = p.Prices["default"]
	}

	// Add attribute add-on prices.
	totalPrice := price.Value
	for _, attrID := range attrKeys {
		valueID := req.Attributes[attrID]
		if valuePrices, ok := p.AttrPrices[attrID]; ok {
			if countryPrices, ok := valuePrices[valueID]; ok {
				ap, ok := countryPrices[country]
				if !ok {
					ap = countryPrices["default"]
				}
				totalPrice += ap
			}
		}
	}

	now := time.Now().UTC()

	o := order.Order{
		ID:          strconv.FormatInt(now.Unix(), 16),
		Status:      order.StatusNew,
		DateTime:    now,
		ProductName: productName,
		Attributes:  strings.Join(attrParts, ", "),
		Price:       totalPrice,
		Currency:    price.Currency,
		FirstName:   req.FirstName,
		MiddleName:  req.MiddleName,
		LastName:    req.LastName,
		Phone:       req.Phone,
		Email:       req.Email,
		City:        req.City,
		Address:     req.Address,
		Notes:       req.Notes,
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
