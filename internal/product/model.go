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
	Title    string  `json:"title"     yaml:"title"`
	AddPrice float64 `json:"add_price" yaml:"add_price"`
}

type AttrLang struct {
	Title  string               `json:"title"  yaml:"title"`
	Values map[string]AttrValue `json:"values" yaml:"values"`
}

type ImageItem struct {
	Preview string `json:"preview" yaml:"preview"`
	Full    string `json:"full"    yaml:"full"`
}

type Product struct {
	ID          string                         `json:"id"          yaml:"id"`
	Name        map[string]string              `json:"name"        yaml:"name"`
	Description map[string]string              `json:"description" yaml:"description"`
	Specs       map[string]map[string]SpecItem `json:"specs"       yaml:"specs"`
	Price       map[string]PriceItem           `json:"price"       yaml:"price"`
	Attrs       map[string]map[string]AttrLang `json:"attrs"       yaml:"attrs"`
	Images      []ImageItem                    `json:"images"      yaml:"images"`
}

// Item holds the lightweight product metadata loaded from products.yaml.
type Item struct {
	ID          string            `json:"id"          yaml:"id"`
	Title       map[string]string `json:"title"       yaml:"title"`
	Description map[string]string `json:"description" yaml:"description"`
}
