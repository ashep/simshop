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
	Title  string `json:"title"  yaml:"title"`
	Prefix string `json:"prefix" yaml:"prefix"`
}

type AttrLang struct {
	Title       string               `json:"title"                 yaml:"title"`
	Description string               `json:"description,omitempty" yaml:"description"`
	Values      map[string]AttrValue `json:"values"                yaml:"values"`
}

// ImageItem is a single gallery media entry. Type defaults to "image" when empty; "video" marks an
// MP4 entry whose Preview/Full point at .mp4 files instead of images.
type ImageItem struct {
	Type    string `json:"type,omitempty" yaml:"type,omitempty"`
	Preview string `json:"preview"        yaml:"preview"`
	Full    string `json:"full"           yaml:"full"`
}

type Product struct {
	ID         string            `json:"id"                yaml:"id"`
	Available  *bool             `json:"available"         yaml:"available"`
	Categories []string          `json:"categories"        yaml:"categories"`
	Name       map[string]string `json:"name"              yaml:"name"`
	// Summary is the short per-language blurb shown on catalog cards (served as the
	// listing's "description"); Description is the full product-page text.
	Summary         map[string]string                        `json:"summary"           yaml:"summary"`
	Description     map[string]string                        `json:"description"       yaml:"description"`
	Specs           map[string]map[string]SpecItem           `json:"specs"             yaml:"specs"`
	Prices          map[string]PriceItem                     `json:"prices"            yaml:"prices"`
	Attrs           map[string]map[string]AttrLang           `json:"attrs"             yaml:"attrs"`
	AttrPrices      map[string]map[string]map[string]float64 `json:"attr_prices"       yaml:"attr_prices"`
	AttrImages      map[string]map[string]string             `json:"attr_images"       yaml:"attr_images"`
	AttrColors      map[string]map[string]string             `json:"attr_colors"       yaml:"attr_colors"`
	AttrValuesOrder map[string][]string                      `json:"attr_values_order" yaml:"attr_values_order"`
	Images          []ImageItem                              `json:"images"            yaml:"images"`
	// Cover optionally names the still image (bare filename under images/) shown on
	// product cards and the home hero instead of the first gallery image. It stays a
	// bare filename after loading; consumers build the /images/{id}/ path themselves.
	Cover string `json:"cover" yaml:"cover"`
	// PromoCodes maps a promo code to its integer discount percent. It must never be
	// serialised: exposing it would leak every code to the storefront.
	PromoCodes map[string]int `json:"-" yaml:"promo_codes"`
}

// ProductDetail holds the lang-filtered product data returned by GET /products/{id}/{lang}.
type ProductDetail struct {
	ID              string                        `json:"id"`
	Available       bool                          `json:"available"`
	Categories      []string                      `json:"categories,omitempty"`
	Name            string                        `json:"name"`
	Description     string                        `json:"description"`
	Specs           map[string]SpecItem           `json:"specs,omitempty"`
	Prices          PriceItem                     `json:"price"`
	Attrs           map[string]AttrLang           `json:"attrs,omitempty"`
	AttrPrices      map[string]map[string]float64 `json:"attr_prices,omitempty"`
	AttrImages      map[string]map[string]string  `json:"attr_images,omitempty"`
	AttrColors      map[string]map[string]string  `json:"attr_colors,omitempty"`
	AttrValuesOrder map[string][]string           `json:"attr_values_order,omitempty"`
	Images          []ImageItem                   `json:"images,omitempty"`
}

// ItemMaxImages caps Item.Images: the card shows the first and swaps to the
// second on hover, so more would only bloat the listing payload.
const ItemMaxImages = 2

// Item holds the lightweight listing metadata of a product named in shop.yaml's
// products list, built from its product.yaml at load time (Title from Name,
// Description from Summary).
type Item struct {
	ID          string            `json:"id"`
	Available   bool              `json:"available"`
	Categories  []string          `json:"categories,omitempty"`
	Title       map[string]string `json:"title"`
	Description map[string]string `json:"description"`
	Image       *string           `json:"image,omitempty"`
	// Images are up to ItemMaxImages still previews, starting with Image; the
	// card crossfades to the second on hover.
	Images []string `json:"images,omitempty"`
	// Prices is copied from the product's product.yaml at load time; the listing
	// endpoint resolves it to a single per-country Price, so it never serialises.
	Prices map[string]PriceItem `json:"-"`
}

// ListItem is one entry of the GET /products response: the listing metadata
// plus the base price resolved for the requesting client's country.
type ListItem struct {
	*Item
	Price *PriceItem `json:"price,omitempty"`
}
