package shop

import "errors"

var ErrShopAlreadyExists = errors.New("shop already exists")
var ErrInvalidLanguage = errors.New("invalid language code")
var ErrShopNotFound = errors.New("shop not found")

type Shop struct {
	ID    string            `json:"id"`
	Names map[string]string `json:"names"`
}
