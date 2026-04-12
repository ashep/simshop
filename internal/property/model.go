package property

import (
	"errors"
	"strings"
)

type InvalidLanguageError struct{ Lang string }

func (e *InvalidLanguageError) Error() string { return "invalid language code: " + e.Lang }

type DuplicateTitleError struct{ Lang string }

func (e *DuplicateTitleError) Error() string { return "title already exists for language: " + e.Lang }

var ErrMissingTitle = errors.New("at least one title is required")
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
