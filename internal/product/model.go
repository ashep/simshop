package product

import (
	"errors"
	"strings"
	"time"
)

var ErrShopNotFound = errors.New("shop not found")
var ErrMissingDefaultPrice = errors.New("default country price is required")
var ErrInvalidCountry = errors.New("invalid country id")
var ErrInvalidLanguage = errors.New("invalid language code")
var ErrShopProductLimitReached = errors.New("shop product limit reached")
var ErrProductNotFound = errors.New("product not found")

// MissingContentError is returned when the request content map is missing an
// entry for a language that the target shop has.
type MissingContentError struct {
	Lang string
}

func (e *MissingContentError) Error() string {
	return "content missing for language: " + e.Lang
}

type Product struct {
	ID string `json:"id"`
}

type PublicProduct struct {
	ID      string                 `json:"id"`
	Content map[string]ContentItem `json:"content"`
}

type AdminProduct struct {
	PublicProduct
	ShopOwnerID string    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ContentItem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type CreateRequest struct {
	ShopID  string                 `json:"shop_id"`
	Prices  map[string]int         `json:"prices"`
	Content map[string]ContentItem `json:"content"`
}

type CreateResponse struct {
	ID string `json:"id"`
}

func (r *CreateRequest) Trim() {
	r.ShopID = strings.TrimSpace(r.ShopID)
	trimmedContent := make(map[string]ContentItem, len(r.Content))
	for k, v := range r.Content {
		trimmedContent[strings.TrimSpace(k)] = ContentItem{
			Title:       strings.TrimSpace(v.Title),
			Description: strings.TrimSpace(v.Description),
		}
	}
	r.Content = trimmedContent
	trimmedPrices := make(map[string]int, len(r.Prices))
	for k, v := range r.Prices {
		trimmedPrices[strings.TrimSpace(k)] = v
	}
	r.Prices = trimmedPrices
}
