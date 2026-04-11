package shop

import (
	"errors"
	"time"
)

var ErrShopAlreadyExists = errors.New("shop already exists")
var ErrInvalidLanguage = errors.New("invalid language code")
var ErrShopNotFound = errors.New("shop not found")
var ErrInvalidOwner = errors.New("invalid owner")

type Shop struct {
	ID           string            `json:"id"`
	Names        map[string]string `json:"names"`
	Descriptions map[string]string `json:"descriptions,omitempty"`
}

type AdminShop struct {
	Shop
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
