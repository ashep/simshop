package product

import "errors"

var ErrProductNotFound = errors.New("product not found")

type Product struct {
	ID     string              `json:"id" yaml:"id"`
	Data   map[string]DataItem `json:"data" yaml:"data"`
	Prices map[string]int      `json:"prices" yaml:"prices"`
	Files  []string            `json:"files" yaml:"files"`
}

type DataItem struct {
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description" yaml:"description"`
}

type PriceResult struct {
	CountryID string `json:"country_id"`
	Value     int    `json:"value"`
}
