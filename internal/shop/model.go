package shop

// Country describes one entry in Shop.Countries: the human-readable per-language
// name and currency symbol, the international phone-code prefix used by the
// frontend's phone input, and a flag glyph (typically a Unicode flag emoji).
type Country struct {
	Name      map[string]string `json:"name,omitempty"       yaml:"name"`
	Currency  map[string]string `json:"currency,omitempty"   yaml:"currency"`
	PhoneCode string            `json:"phone_code,omitempty" yaml:"phone_code"`
	Flag      string            `json:"flag,omitempty"       yaml:"flag"`
}

// Shop holds the store metadata loaded from shop.yaml. Countries is the
// authoritative allow-list of countries from which orders may be created
// (keyed by lowercase ISO alpha-2 code).
type Shop struct {
	Countries   map[string]*Country `json:"countries,omitempty"   yaml:"countries"`
	Name        map[string]string   `json:"name,omitempty"        yaml:"name"`
	Title       map[string]string   `json:"title,omitempty"       yaml:"title"`
	Description map[string]string   `json:"description,omitempty" yaml:"description"`
}
