package property

import "errors"

var ErrInvalidLanguage = errors.New("invalid language code")
var ErrDuplicateTitle = errors.New("title already exists for this language")
var ErrMissingEnTitle = errors.New("EN title is required")
var ErrPropertyNotFound = errors.New("property not found")

type CreateRequest struct {
	Titles map[string]string `json:"titles"`
}

type UpdateRequest struct {
	Titles map[string]string `json:"titles"`
}

type Property struct {
	ID     string            `json:"id"`
	Titles map[string]string `json:"titles"`
}

type CreateResponse struct {
	ID string `json:"id"`
}
