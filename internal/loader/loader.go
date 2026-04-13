package loader

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ashep/simshop/internal/product"
	"gopkg.in/yaml.v3"
)

// Catalog holds all content loaded from the data directory at startup.
type Catalog struct {
	Products []*product.Product
}

// Load reads data_dir, returning a populated Catalog.
// A missing data_dir is not an error — it results in an empty catalog.
// A malformed YAML file is a fatal error.
func Load(dataDir string) (*Catalog, error) {
	c := &Catalog{}

	if err := loadProducts(dataDir, c); err != nil {
		return nil, err
	}
	return c, nil
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
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".yaml")
		p, err := loadProduct(filepath.Join(prodsDir, e.Name()), id)
		if err != nil {
			return fmt.Errorf("load product %s: %w", id, err)
		}
		c.Products = append(c.Products, p)
	}
	return nil
}

func loadProduct(path, id string) (*product.Product, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var p product.Product
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	p.ID = id

	return &p, nil
}
