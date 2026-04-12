package property

import "errors"

var ErrInvalidLanguage = errors.New("invalid language code")

type CreateRequest struct {
	Titles map[string]string `json:"titles"`
}

type Property struct {
	ID string `json:"id"`
}

type CreateResponse struct {
	ID string `json:"id"`
}
