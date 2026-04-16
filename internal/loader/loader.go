package loader

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ashep/simshop/internal/page"
	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/internal/shop"
	"gopkg.in/yaml.v3"
)

// Catalog holds all content loaded from the data directory at startup.
type Catalog struct {
	Products     []*product.Product
	ProductItems []*product.Item
	Pages        []*page.Page
	Shop         *shop.Shop
}

// Load reads data_dir, returning a populated Catalog.
// A missing data_dir is not an error — it results in an empty catalog.
// A malformed YAML file or a validation error is fatal.
func Load(dataDir string) (*Catalog, error) {
	c := &Catalog{}

	if err := loadProducts(dataDir, c); err != nil {
		return nil, err
	}
	if err := loadProductsList(dataDir, c); err != nil {
		return nil, err
	}
	joinProductImages(c)
	if err := loadPages(dataDir, c); err != nil {
		return nil, err
	}
	if err := loadShop(dataDir, c); err != nil {
		return nil, err
	}
	return c, nil
}

type pagesFile struct {
	Pages []*page.Page `yaml:"pages"`
}

// joinProductImages sets Item.Image to the first preview path from the
// corresponding full Product, if one exists.
func joinProductImages(c *Catalog) {
	if len(c.ProductItems) == 0 || len(c.Products) == 0 {
		return
	}
	byID := make(map[string]*product.Product, len(c.Products))
	for _, p := range c.Products {
		byID[p.ID] = p
	}
	for _, item := range c.ProductItems {
		p, ok := byID[item.ID]
		if !ok || len(p.Images) == 0 || p.Images[0].Preview == "" {
			continue
		}
		preview := p.Images[0].Preview
		item.Image = &preview
	}
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
		c.Shop = &shop.Shop{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read shop.yaml: %w", err)
	}

	var f shopFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse shop.yaml: %w", err)
	}
	if f.Shop == nil {
		c.Shop = &shop.Shop{}
	} else {
		c.Shop = f.Shop
	}
	return nil
}

type productsFile struct {
	Products []*product.Item `yaml:"products"`
}

func loadProductsList(dataDir string, c *Catalog) error {
	path := filepath.Join(dataDir, "products", "products.yaml")

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read products.yaml: %w", err)
	}

	var f productsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse products.yaml: %w", err)
	}
	c.ProductItems = f.Products
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

	return nil
}
