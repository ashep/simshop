package product

import (
	"errors"
	"strings"
	"time"
)

var ErrShopNotFound = errors.New("shop not found")
var ErrShopProductLimitReached = errors.New("shop product limit reached")
var ErrProductNotFound = errors.New("product not found")
var ErrMissingTitle = errors.New("at least one title is required")

type InvalidLanguageError struct{ Lang string }

func (e *InvalidLanguageError) Error() string { return "invalid language code: " + e.Lang }

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
	Data map[string]DataItem `json:"data"`
}

type AdminProduct struct {
	PublicProduct
	ShopOwnerID string    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DataItem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type CreateRequest struct {
	ShopID  string                 `json:"shop_id"`
	Data map[string]DataItem `json:"data"`
}

type CreateResponse struct {
	ID string `json:"id"`
}

func (r *CreateRequest) Trim() {
	r.ShopID = strings.TrimSpace(r.ShopID)
	trimmedContent := make(map[string]DataItem, len(r.Data))
	for k, v := range r.Data {
		trimmedContent[strings.TrimSpace(k)] = DataItem{
			Title:       strings.TrimSpace(v.Title),
			Description: strings.TrimSpace(v.Description),
		}
	}
	r.Data = trimmedContent
}

type UpdateRequest struct {
	Data map[string]DataItem `json:"data"`
}

func (r *UpdateRequest) Trim() {
	trimmed := make(map[string]DataItem, len(r.Data))
	for k, v := range r.Data {
		trimmed[strings.TrimSpace(k)] = DataItem{
			Title:       strings.TrimSpace(v.Title),
			Description: strings.TrimSpace(v.Description),
		}
	}
	r.Data = trimmed
}

var ErrFileNotFound = errors.New("file not found")
var ErrFileOwnerMismatch = errors.New("file owner mismatch")

type SetFilesRequest struct {
	FileIDs []string `json:"file_ids"`
	IsAdmin bool     `json:"-"`
}

type InvalidCountryError struct{ Country string }

func (e *InvalidCountryError) Error() string { return "invalid country code: " + e.Country }

type PriceResult struct {
	CountryID string `json:"country_id"`
	Value     int    `json:"value"`
}

type SetPricesRequest struct {
	Prices map[string]int `json:"prices"`
}

func (r *SetPricesRequest) Trim() {
	trimmed := make(map[string]int, len(r.Prices))
	for k, v := range r.Prices {
		trimmed[strings.TrimSpace(k)] = v
	}
	r.Prices = trimmed
}
