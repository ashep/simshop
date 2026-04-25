package handler

import (
	"net/http"
	"time"

	"github.com/ashep/simshop/internal/order"
)

type orderRecordResponse struct {
	ID           string                 `json:"id"`
	ProductID    string                 `json:"product_id"`
	Status       string                 `json:"status"`
	Email        string                 `json:"email"`
	Price        int                    `json:"price"`
	Currency     string                 `json:"currency"`
	FirstName    string                 `json:"first_name"`
	MiddleName   *string                `json:"middle_name,omitempty"`
	LastName     string                 `json:"last_name"`
	Country      string                 `json:"country"`
	City         string                 `json:"city"`
	Phone        string                 `json:"phone"`
	Address      string                 `json:"address"`
	AdminNote    *string                `json:"admin_note,omitempty"`
	CustomerNote *string                `json:"customer_note,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	Attrs        []orderAttrResponse    `json:"attrs"`
	History      []orderHistoryResponse `json:"history"`
}

type orderAttrResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Price int    `json:"price"`
}

type orderHistoryResponse struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Note      *string   `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ListOrders returns every persisted order with its attrs and history,
// newest first. The endpoint is only registered when an API key is configured.
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	rs, err := h.orders.List(r.Context())
	if err != nil {
		h.l.Error().Err(err).Msg("list orders failed")
		h.writeError(w, err)
		return
	}

	out := toOrdersResponse(rs)
	if err := h.resp.Write(w, r, http.StatusOK, out); err != nil {
		h.l.Error().Err(err).Msg("write list orders response failed")
		h.writeError(w, err)
	}
}

func toOrdersResponse(rs []order.Record) []orderRecordResponse {
	out := make([]orderRecordResponse, len(rs))
	for i, rec := range rs {
		out[i] = toOrderRecordResponse(rec)
	}
	return out
}

func toOrderRecordResponse(rec order.Record) orderRecordResponse {
	attrs := make([]orderAttrResponse, len(rec.Attrs))
	for i, a := range rec.Attrs {
		attrs[i] = orderAttrResponse{Name: a.Name, Value: a.Value, Price: a.Price}
	}
	history := make([]orderHistoryResponse, len(rec.History))
	for i, h := range rec.History {
		history[i] = orderHistoryResponse{
			ID:        h.ID,
			Status:    h.Status,
			Note:      h.Note,
			CreatedAt: h.CreatedAt,
		}
	}
	return orderRecordResponse{
		ID:           rec.ID,
		ProductID:    rec.ProductID,
		Status:       rec.Status,
		Email:        rec.Email,
		Price:        rec.Price,
		Currency:     rec.Currency,
		FirstName:    rec.FirstName,
		MiddleName:   rec.MiddleName,
		LastName:     rec.LastName,
		Country:      rec.Country,
		City:         rec.City,
		Phone:        rec.Phone,
		Address:      rec.Address,
		AdminNote:    rec.AdminNote,
		CustomerNote: rec.CustomerNote,
		CreatedAt:    rec.CreatedAt,
		UpdatedAt:    rec.UpdatedAt,
		Attrs:        attrs,
		History:      history,
	}
}
