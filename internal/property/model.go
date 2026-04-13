package property

import "errors"

var ErrPropertyNotFound = errors.New("property not found")

type Property struct {
	ID     string            `json:"id"     yaml:"id"`
	Titles map[string]string `json:"titles" yaml:"titles"`
}
