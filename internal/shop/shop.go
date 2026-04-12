package shop

import (
	"errors"
	"time"
)

var ErrShopAlreadyExists = errors.New("shop already exists")
var ErrShopNotFound = errors.New("shop not found")
var ErrInvalidOwner = errors.New("invalid owner")

type InvalidLanguageError struct{ Lang string }

func (e *InvalidLanguageError) Error() string { return "invalid language code: " + e.Lang }

type Shop struct {
	ID           string            `json:"id"`
	Titles       map[string]string `json:"titles"`
	Descriptions map[string]string `json:"descriptions,omitempty"`
}

type AdminShop struct {
	Shop
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
