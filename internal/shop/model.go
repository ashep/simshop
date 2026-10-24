package shop

import "gopkg.in/yaml.v3"

// Country describes one entry in Shop.Countries: the human-readable per-language
// name and currency symbol, the international phone-code prefix used by the
// frontend's phone input, and a flag glyph (typically a Unicode flag emoji).
type Country struct {
	Name      map[string]string `json:"name,omitempty"       yaml:"name"`
	Currency  map[string]string `json:"currency,omitempty"   yaml:"currency"`
	PhoneCode string            `json:"phone_code,omitempty" yaml:"phone_code"`
	Flag      string            `json:"flag,omitempty"       yaml:"flag"`
}

// Category describes one product category from shop.yaml: a stable id, the
// human-readable title per language. In YAML the language keys sit flat
// alongside id (`{id: clocks, en: Clocks, uk: Годинники}`); every key other
// than id is a language code collected into Title.
type Category struct {
	ID    string            `json:"id"`
	Title map[string]string `json:"title,omitempty"`
}

// categoryReservedKeys are the Category YAML keys that are not language codes.
var categoryReservedKeys = map[string]bool{"id": true}

// UnmarshalYAML splits the flat YAML mapping into ID and per-language Title.
func (c *Category) UnmarshalYAML(n *yaml.Node) error {
	var raw map[string]string
	if err := n.Decode(&raw); err != nil {
		return err
	}
	c.ID = raw["id"]
	c.Title = nil
	for k, v := range raw {
		if categoryReservedKeys[k] {
			continue
		}
		if c.Title == nil {
			c.Title = make(map[string]string, len(raw))
		}
		c.Title[k] = v
	}
	return nil
}

// Link describes a single external/social link from shop.yaml. Links are
// language-independent; each entry carries a display title, an icon
// identifier, and the target URL.
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

// HeroImage is one hero slide: File is a bare filename under the data dir's
// assets/ folder (served at GET /assets/{name}); Category optionally names a shop
// category id the slide links to (its filtered catalog), unset = whole catalog.
// In YAML a slide is either a bare filename or a `{file, category}` mapping.
type HeroImage struct {
	File     string `json:"file"`
	Category string `json:"category,omitempty"`
}

// UnmarshalYAML accepts both the bare-filename and the mapping form.
func (h *HeroImage) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*h = HeroImage{File: n.Value}
		return nil
	}
	var raw struct {
		File     string `yaml:"file"`
		Category string `yaml:"category"`
	}
	if err := n.Decode(&raw); err != nil {
		return err
	}
	*h = HeroImage{File: raw.File, Category: raw.Category}
	return nil
}

// Hero is the storefront's home-page lead block: per-language headline and
// supporting text plus an optional ordered slideshow the frontend rotates under
// the same text.
type Hero struct {
	Eyebrow map[string]string `json:"eyebrow,omitempty" yaml:"eyebrow"`
	Title   map[string]string `json:"title,omitempty"   yaml:"title"`
	Text    map[string]string `json:"text,omitempty"    yaml:"text"`
	Images  []*HeroImage      `json:"images,omitempty"  yaml:"images"`
}

// Feature is one entry of the home-page benefits strip. Icon is an identifier
// the frontend maps to a glyph (unknown ones render without an icon).
type Feature struct {
	Icon  string            `json:"icon,omitempty"  yaml:"icon"`
	Title map[string]string `json:"title,omitempty" yaml:"title"`
	Text  map[string]string `json:"text,omitempty"  yaml:"text"`
}

// Story is the home-page "about us" band. Page optionally names a content page
// id (from pages.yaml) the band links to. Image is a bare assets/ filename.
type Story struct {
	Eyebrow map[string]string `json:"eyebrow,omitempty" yaml:"eyebrow"`
	Title   map[string]string `json:"title,omitempty"   yaml:"title"`
	Text    map[string]string `json:"text,omitempty"    yaml:"text"`
	Image   string            `json:"image,omitempty"   yaml:"image"`
	Page    string            `json:"page,omitempty"    yaml:"page"`
}

// Shop holds the store metadata loaded from shop.yaml. Countries is the
// authoritative allow-list of countries from which orders may be created
// (keyed by lowercase ISO alpha-2 code). Announcement, AnnouncementPage, Hero,
// Features, Story, Gallery and Menu are optional storefront content; Gallery lists
// bare assets/ filenames, Menu lists content page ids (pages.yaml) for the header
// nav and AnnouncementPage is the content page id the announcement bar links to.
type Shop struct {
	Countries        map[string]*Country `json:"countries,omitempty"        yaml:"countries"`
	Name             map[string]string   `json:"name,omitempty"             yaml:"name"`
	Title            map[string]string   `json:"title,omitempty"            yaml:"title"`
	Description      map[string]string   `json:"description,omitempty"      yaml:"description"`
	Categories       []*Category         `json:"categories,omitempty"       yaml:"categories"`
	Links            []*Link             `json:"links,omitempty"            yaml:"links"`
	GoogleAnalytics  *GoogleAnalytics    `json:"google_analytics,omitempty" yaml:"google-analytics"`
	Announcement     map[string]string   `json:"announcement,omitempty"     yaml:"announcement"`
	AnnouncementPage string              `json:"announcement_page,omitempty" yaml:"announcement_page"`
	Hero             *Hero               `json:"hero,omitempty"             yaml:"hero"`
	Features         []*Feature          `json:"features,omitempty"         yaml:"features"`
	Story            *Story              `json:"story,omitempty"            yaml:"story"`
	Gallery          []string            `json:"gallery,omitempty"          yaml:"gallery"`
	Menu             []string            `json:"menu,omitempty"             yaml:"menu"`
	// Products lists product ids (product directory names) in catalog order; only
	// listed products appear in GET /products. Consumed by the loader, not served.
	Products []string `json:"-" yaml:"products"`
}
