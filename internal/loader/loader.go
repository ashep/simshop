package loader

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ashep/simshop/internal/file"
	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/internal/property"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

// Catalog holds all content loaded from the data directory at startup.
type Catalog struct {
	Products   []*product.Product
	Properties []property.Property
	Files      map[string][]file.FileInfo // product ID → files
}

// Load reads data_dir and public_dir, returning a populated Catalog.
// A missing data_dir is not an error — it results in an empty catalog.
// A malformed YAML file is a fatal error.
// A missing binary file logs a warning and is skipped.
func Load(dataDir, publicDir string, l zerolog.Logger) (*Catalog, error) {
	c := &Catalog{
		Properties: []property.Property{},
		Files:      make(map[string][]file.FileInfo),
	}

	if err := loadProperties(dataDir, c); err != nil {
		return nil, err
	}
	if err := loadProducts(dataDir, publicDir, c, l); err != nil {
		return nil, err
	}
	return c, nil
}

func loadProperties(dataDir string, c *Catalog) error {
	path := filepath.Join(dataDir, "properties.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read properties.yaml: %w", err)
	}
	if err := yaml.Unmarshal(data, &c.Properties); err != nil {
		return fmt.Errorf("parse properties.yaml: %w", err)
	}
	if c.Properties == nil {
		c.Properties = []property.Property{}
	}
	return nil
}

func loadProducts(dataDir, publicDir string, c *Catalog, l zerolog.Logger) error {
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
		p, files, err := loadProduct(filepath.Join(prodsDir, e.Name()), id, publicDir, l)
		if err != nil {
			return fmt.Errorf("load product %s: %w", id, err)
		}
		c.Products = append(c.Products, p)
		c.Files[id] = files
	}
	return nil
}

func loadProduct(path, id, publicDir string, l zerolog.Logger) (*product.Product, []file.FileInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read: %w", err)
	}

	var p product.Product
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, nil, fmt.Errorf("parse: %w", err)
	}
	p.ID = id

	fileInfos := make([]file.FileInfo, 0)
	for _, name := range p.Files {
		diskPath := filepath.Join(publicDir, id, name)
		info, statErr := os.Stat(diskPath)
		if statErr != nil {
			if errors.Is(statErr, fs.ErrNotExist) {
				l.Warn().Str("product_id", id).Str("file", name).Msg("file not found on disk, skipping")
				continue
			}
			return nil, nil, fmt.Errorf("stat file %s: %w", name, statErr)
		}

		f, err := os.Open(diskPath)
		if err != nil {
			return nil, nil, fmt.Errorf("open file %s: %w", name, err)
		}
		buf := make([]byte, 512)
		n, _ := f.Read(buf)
		_ = f.Close()

		fileInfos = append(fileInfos, file.FileInfo{
			Name:      name,
			MimeType:  http.DetectContentType(buf[:n]),
			SizeBytes: int(info.Size()),
			Path:      "/files/" + id + "/" + name,
		})
	}
	return &p, fileInfos, nil
}
