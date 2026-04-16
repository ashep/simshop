package product

type SpecItem struct {
	Title string `json:"title" yaml:"title"`
	Value string `json:"value" yaml:"value"`
}

type PriceItem struct {
	Currency string  `json:"currency" yaml:"currency"`
	Value    float64 `json:"value"    yaml:"value"`
}

type AttrValue struct {
	Title string `json:"title" yaml:"title"`
}

type AttrLang struct {
	Title       string               `json:"title"                 yaml:"title"`
	Description string               `json:"description,omitempty" yaml:"description"`
	Values      map[string]AttrValue `json:"values"                yaml:"values"`
}

type ImageItem struct {
	Preview string `json:"preview" yaml:"preview"`
	Full    string `json:"full"    yaml:"full"`
}

type Product struct {
	ID          string                                       `json:"id"          yaml:"id"`
	Name        map[string]string                            `json:"name"        yaml:"name"`
	Description map[string]string                            `json:"description" yaml:"description"`
	Specs       map[string]map[string]SpecItem               `json:"specs"       yaml:"specs"`
	Prices      map[string]PriceItem                         `json:"prices"      yaml:"prices"`
	Attrs       map[string]map[string]AttrLang               `json:"attrs"       yaml:"attrs"`
	AttrPrices  map[string]map[string]map[string]float64     `json:"attr_prices" yaml:"attr_prices"`
	AttrImages  map[string]map[string]string                 `json:"attr_images" yaml:"attr_images"`
	Images      []ImageItem                                  `json:"images"      yaml:"images"`
}

// ProductDetail holds the lang-filtered product data returned by GET /products/{id}/{lang}.
type ProductDetail struct {
	ID          string                        `json:"id"`
	Name        string                        `json:"name"`
	Description string                        `json:"description"`
	Specs       map[string]SpecItem           `json:"specs,omitempty"`
	Prices      PriceItem                     `json:"price"`
	Attrs       map[string]AttrLang           `json:"attrs,omitempty"`
	AttrPrices  map[string]map[string]float64 `json:"attr_prices,omitempty"`
	AttrImages  map[string]map[string]string  `json:"attr_images,omitempty"`
	Images      []ImageItem                   `json:"images,omitempty"`
}

// Item holds the lightweight product metadata loaded from products.yaml.
type Item struct {
	ID          string            `json:"id"          yaml:"id"`
	Title       map[string]string `json:"title"       yaml:"title"`
	Description map[string]string `json:"description" yaml:"description"`
	Image       *string           `json:"image,omitempty" yaml:"-"`
}
