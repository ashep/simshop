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

// Category describes one product category from shop.yaml: a stable id plus the
// human-readable title per language. In YAML the language keys sit flat
// alongside id (`{id: clocks, en: Clocks, uk: Годинники}`) and are collected
// into Title via the inline map.
type Category struct {
	ID    string            `json:"id"              yaml:"id"`
	Title map[string]string `json:"title,omitempty" yaml:",inline"`
}

// Link describes a single external/social link from shop.yaml. Links are grouped
// per language in Shop.Links; each entry carries a display title, an icon
// identifier the frontend maps to a glyph, and the target URL.
type Link struct {
	Title string `json:"title" yaml:"title"`
	Icon  string `json:"icon"  yaml:"icon"`
	URL   string `json:"url"   yaml:"url"`
}

// GoogleAnalytics holds the Google Analytics configuration from shop.yaml; ID is
// the measurement id (e.g. "G-XXXXXXXXXX") the frontend uses to initialise gtag.
type GoogleAnalytics struct {
	ID string `json:"id" yaml:"id"`
}

// Shop holds the store metadata loaded from shop.yaml. Countries is the
// authoritative allow-list of countries from which orders may be created
// (keyed by lowercase ISO alpha-2 code).
type Shop struct {
	Countries       map[string]*Country `json:"countries,omitempty"        yaml:"countries"`
	Name            map[string]string   `json:"name,omitempty"             yaml:"name"`
	Title           map[string]string   `json:"title,omitempty"            yaml:"title"`
	Description     map[string]string   `json:"description,omitempty"      yaml:"description"`
	Categories      []*Category         `json:"categories,omitempty"       yaml:"categories"`
	Links           map[string][]*Link  `json:"links,omitempty"            yaml:"links"`
	GoogleAnalytics *GoogleAnalytics    `json:"google_analytics,omitempty" yaml:"google-analytics"`
}
