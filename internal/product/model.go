package product

import "errors"

var ErrProductNotFound = errors.New("product not found")

type DataItem struct {
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description" yaml:"description"`
}

type Product struct {
	ID   string              `json:"id" yaml:"id"`
	Data map[string]DataItem `json:"data" yaml:"data"`
}
