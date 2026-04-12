package property

import (
	"errors"
	"strings"
)

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

func (r *CreateRequest) Trim() {
	trimmed := make(map[string]string, len(r.Titles))
	for k, v := range r.Titles {
		trimmed[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	r.Titles = trimmed
}

func (r *UpdateRequest) Trim() {
	trimmed := make(map[string]string, len(r.Titles))
	for k, v := range r.Titles {
		trimmed[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	r.Titles = trimmed
}
