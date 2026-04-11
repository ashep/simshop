package product

import "errors"

var ErrShopNotFound = errors.New("shop not found")
var ErrMissingDefaultPrice = errors.New("default country price is required")
var ErrInvalidCountry = errors.New("invalid country id")
var ErrInvalidLanguage = errors.New("invalid language code")

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
