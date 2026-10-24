package loader

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/ashep/simshop/internal/page"
	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/internal/resend"
	"github.com/ashep/simshop/internal/shop"
	"gopkg.in/yaml.v3"
)

// Catalog holds all content loaded from the data directory at startup.
type Catalog struct {
	Products       []*product.Product
	ProductItems   []*product.Item
	Pages          []*page.Page
	Shop           *shop.Shop
	EmailTemplates *resend.TemplateStore
}

// Load reads data_dir, returning a populated Catalog.
// A missing data_dir is not an error — it results in an empty catalog.
// A malformed YAML file or a validation error is fatal.
func Load(dataDir string) (*Catalog, error) {
	c := &Catalog{}

	if err := loadProducts(dataDir, c); err != nil {
		return nil, err
	}
	if err := loadShop(dataDir, c); err != nil {
		return nil, err
	}
	if err := buildProductItems(c); err != nil {
		return nil, err
	}
	dropEmptyCategories(c)
	if err := loadPages(dataDir, c); err != nil {
		return nil, err
	}
	if err := loadEmailTemplates(dataDir, c); err != nil {
		return nil, err
	}
	return c, nil
}

func loadEmailTemplates(dataDir string, c *Catalog) error {
	store, err := resend.LoadTemplates(filepath.Join(dataDir, "emails"))
	if err != nil {
		return fmt.Errorf("load email templates: %w", err)
	}
	c.EmailTemplates = store
	return nil
}

type pagesFile struct {
	Pages []*page.Page `yaml:"pages"`
}

// buildProductItems builds the catalog listing from shop.yaml's products list:
// one Item per listed id, in list order, populated from that product's
// product.yaml. Unlisted products are loaded (and reachable by id) but not
// listed. An unknown or duplicate id is fatal.
func buildProductItems(c *Catalog) error {
	byID := make(map[string]*product.Product, len(c.Products))
	for _, p := range c.Products {
		byID[p.ID] = p
	}
	seen := make(map[string]bool, len(c.Shop.Products))
	for _, id := range c.Shop.Products {
		p, ok := byID[id]
		if !ok {
			return fmt.Errorf("shop.yaml products: unknown product %q", id)
		}
		if seen[id] {
			return fmt.Errorf("shop.yaml products: duplicate product %q", id)
		}
		seen[id] = true
		c.ProductItems = append(c.ProductItems, newProductItem(p))
	}
	return nil
}

// dropEmptyCategories removes shop categories that no listed product belongs to,
// so the storefront never offers a category with nothing in it. Order is kept.
func dropEmptyCategories(c *Catalog) {
	used := make(map[string]bool)
	for _, item := range c.ProductItems {
		for _, id := range item.Categories {
			used[id] = true
		}
	}
	kept := c.Shop.Categories[:0]
	for _, cat := range c.Shop.Categories {
		if used[cat.ID] {
			kept = append(kept, cat)
		}
	}
	c.Shop.Categories = kept
}

// newProductItem builds a listing Item from a loaded product. The thumbnail is
// the cover when set, else the first still image's preview; Images holds the
// thumbnail followed by the next still previews, up to product.ItemMaxImages.
func newProductItem(p *product.Product) *product.Item {
	item := &product.Item{
		ID:          p.ID,
		Available:   p.Available == nil || *p.Available,
		Categories:  p.Categories,
		Title:       p.Name,
		Description: p.Summary,
		Prices:      p.Prices,
	}
	if p.Cover != "" {
		item.Images = append(item.Images, "/images/"+p.ID+"/"+p.Cover)
	}
	// Skip video entries: their preview is an .mp4, which can't render in the grid's <img>.
	for _, img := range p.Images {
		if len(item.Images) == product.ItemMaxImages {
			break
		}
		if img.Type == "video" || img.Preview == "" || slices.Contains(item.Images, img.Preview) {
			continue
		}
		item.Images = append(item.Images, img.Preview)
	}
	if len(item.Images) > 0 {
		item.Image = &item.Images[0]
	}
	return item
}

func loadPages(dataDir string, c *Catalog) error {
	path := filepath.Join(dataDir, "pages", "pages.yaml")

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read pages.yaml: %w", err)
	}

	var f pagesFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse pages.yaml: %w", err)
	}
	c.Pages = f.Pages
	return nil
}

type shopFile struct {
	Shop *shop.Shop `yaml:"shop"`
}

func loadShop(dataDir string, c *Catalog) error {
	path := filepath.Join(dataDir, "shop.yaml")

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("shop.yaml is required but not found at %s", path)
	}
	if err != nil {
		return fmt.Errorf("read shop.yaml: %w", err)
	}

	var f shopFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse shop.yaml: %w", err)
	}
	if f.Shop == nil {
		f.Shop = &shop.Shop{}
	}
	if len(f.Shop.Countries) == 0 {
		return fmt.Errorf("shop.yaml must define a non-empty countries list")
	}
	c.Shop = f.Shop
	return nil
}

func loadProducts(dataDir string, c *Catalog) error {
	prodsDir := filepath.Join(dataDir, "products")
	entries, err := os.ReadDir(prodsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read products dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		prodDir := filepath.Join(prodsDir, id)
		p, err := loadProduct(prodDir, id)
		if err != nil {
			return fmt.Errorf("load product %s: %w", id, err)
		}
		if p == nil {
			continue
		}
		c.Products = append(c.Products, p)
	}
	return nil
}

func loadProduct(dir, id string) (*product.Product, error) {
	data, err := os.ReadFile(filepath.Join(dir, "product.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var p product.Product
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	p.ID = id

	if err := validate(&p, dir); err != nil {
		return nil, err
	}

	for i, img := range p.Images {
		if img.Preview != "" {
			p.Images[i].Preview = "/images/" + id + "/" + img.Preview
		}
		if img.Full != "" {
			p.Images[i].Full = "/images/" + id + "/" + img.Full
		}
	}

	return &p, nil
}

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func isVideoFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mov", ".webm", ".m4v":
		return true
	}
	return false
}

func validate(p *product.Product, dir string) error {
	if len(p.Name) == 0 {
		return fmt.Errorf("name is required")
	}
	if len(p.Description) == 0 {
		return fmt.Errorf("description is required")
	}

	// All languages in name must appear in description and vice versa.
	for lang := range p.Name {
		if _, ok := p.Description[lang]; !ok {
			return fmt.Errorf("description missing language %q (defined in name)", lang)
		}
	}
	for lang := range p.Description {
		if _, ok := p.Name[lang]; !ok {
			return fmt.Errorf("description has extra language %q not present in name", lang)
		}
	}

	// summary is optional; when present it must cover exactly name's languages.
	if len(p.Summary) > 0 {
		for lang := range p.Name {
			if _, ok := p.Summary[lang]; !ok {
				return fmt.Errorf("summary missing language %q (defined in name)", lang)
			}
		}
		for lang := range p.Summary {
			if _, ok := p.Name[lang]; !ok {
				return fmt.Errorf("summary has extra language %q not present in name", lang)
			}
		}
	}

	// Each spec must carry all languages defined in name.
	for specKey, spec := range p.Specs {
		for lang := range p.Name {
			if _, ok := spec[lang]; !ok {
				return fmt.Errorf("spec %q missing language %q", specKey, lang)
			}
		}
		for lang := range spec {
			if _, ok := p.Name[lang]; !ok {
				return fmt.Errorf("spec %q has extra language %q not present in name", specKey, lang)
			}
		}
	}

	// Prices must always define the "default" key.
	if _, ok := p.Prices["default"]; !ok {
		return fmt.Errorf("prices must define a \"default\" key")
	}

	// Each attr must carry all languages defined in name, and each language entry
	// must have at least one value.
	for attrKey, attr := range p.Attrs {
		for lang := range p.Name {
			attrLang, ok := attr[lang]
			if !ok {
				return fmt.Errorf("attr %q missing language %q", attrKey, lang)
			}
			if len(attrLang.Values) == 0 {
				return fmt.Errorf("attr %q language %q has no values", attrKey, lang)
			}
		}
		for lang := range attr {
			if _, ok := p.Name[lang]; !ok {
				return fmt.Errorf("attr %q has extra language %q not present in name", attrKey, lang)
			}
		}
	}

	// All image paths must exist on disk relative to the product's images directory.
	imagesDir := filepath.Join(dir, "images")
	for i, img := range p.Images {
		if img.Preview != "" {
			if _, statErr := os.Stat(filepath.Join(imagesDir, img.Preview)); errors.Is(statErr, fs.ErrNotExist) {
				return fmt.Errorf("image[%d] preview file not found: %s", i, img.Preview)
			} else if statErr != nil {
				return fmt.Errorf("image[%d] preview: %w", i, statErr)
			}
		}
		if img.Full != "" {
			if _, statErr := os.Stat(filepath.Join(imagesDir, img.Full)); errors.Is(statErr, fs.ErrNotExist) {
				return fmt.Errorf("image[%d] full file not found: %s", i, img.Full)
			} else if statErr != nil {
				return fmt.Errorf("image[%d] full: %w", i, statErr)
			}
		}
	}

	// All attr_images paths must exist on disk relative to the product's images directory.
	for attrKey, valueImages := range p.AttrImages {
		for valueKey, filename := range valueImages {
			if _, statErr := os.Stat(filepath.Join(imagesDir, filename)); errors.Is(statErr, fs.ErrNotExist) {
				return fmt.Errorf("attr_images[%s][%s] file not found: %s", attrKey, valueKey, filename)
			} else if statErr != nil {
				return fmt.Errorf("attr_images[%s][%s]: %w", attrKey, valueKey, statErr)
			}
		}
	}

	// cover must be an existing still image under images/ (it is shown in an <img>).
	if p.Cover != "" {
		if isVideoFile(p.Cover) {
			return fmt.Errorf("cover must be a still image, got video %q", p.Cover)
		}
		for _, img := range p.Images {
			if img.Type == "video" && (img.Preview == p.Cover || img.Full == p.Cover) {
				return fmt.Errorf("cover must be a still image, got video %q", p.Cover)
			}
		}
		if _, statErr := os.Stat(filepath.Join(imagesDir, p.Cover)); errors.Is(statErr, fs.ErrNotExist) {
			return fmt.Errorf("cover file not found: %s", p.Cover)
		} else if statErr != nil {
			return fmt.Errorf("cover: %w", statErr)
		}
	}

	// attr_colors values are swatch colors and must be #rrggbb.
	for attrKey, valueColors := range p.AttrColors {
		for valueKey, color := range valueColors {
			if !hexColorRe.MatchString(color) {
				return fmt.Errorf("attr_colors[%s][%s] must be a #rrggbb color, got %q", attrKey, valueKey, color)
			}
		}
	}

	// promo_codes: codes match case-insensitively, so two keys differing only by
	// case would silently shadow each other.
	codes := make([]string, 0, len(p.PromoCodes))
	for code := range p.PromoCodes {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	seenCodes := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if code == "" || strings.IndexFunc(code, unicode.IsSpace) >= 0 {
			return fmt.Errorf("promo_codes: code %q must be non-empty and contain no whitespace", code)
		}
		if pct := p.PromoCodes[code]; pct < 1 || pct > 99 {
			return fmt.Errorf("promo_codes[%s] must be 1..99, got %d", code, pct)
		}
		upper := strings.ToUpper(code)
		if _, dup := seenCodes[upper]; dup {
			return fmt.Errorf("duplicate promo code %q", code)
		}
		seenCodes[upper] = struct{}{}
	}

	return nil
}
