package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Shop is a single configured backend target.
type Shop struct {
	Name   string
	URL    string
	APIKey string
}

// Config is the parsed ~/.simshop.yaml, shops kept in file order.
type Config struct {
	Shops       []*Shop
	defaultName string
}

// DefaultName returns the name of the shop used when --shop is omitted.
func (c *Config) DefaultName() string { return c.defaultName }

// Select returns the shop by name, or the default shop when name is empty.
func (c *Config) Select(name string) (*Shop, error) {
	if name == "" {
		if c.defaultName == "" {
			return nil, fmt.Errorf("no default shop configured")
		}
		name = c.defaultName
	}
	for _, s := range c.Shops {
		if s.Name == name {
			return s, nil
		}
	}
	names := make([]string, 0, len(c.Shops))
	for _, s := range c.Shops {
		names = append(names, s.Name)
	}
	return nil, fmt.Errorf("unknown shop %q; configured shops: %s", name, strings.Join(names, ", "))
}

// LoadConfig reads and parses the config file at path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return parseConfig(data)
}

// DefaultConfigPath returns ~/.simshop.yaml (or .yml if only that exists).
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	yml := filepath.Join(home, ".simshop.yml")
	yamlPath := filepath.Join(home, ".simshop.yaml")
	if _, err := os.Stat(yamlPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat %s: %w", yamlPath, err)
		}
		if _, err := os.Stat(yml); err == nil {
			return yml, nil
		}
	}
	return yamlPath, nil
}

func parseConfig(data []byte) (*Config, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("empty config")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config root must be a mapping of shop names")
	}

	cfg := &Config{}
	defaults := 0
	for i := 0; i+1 < len(root.Content); i += 2 {
		name := root.Content[i].Value
		var entry struct {
			URL     string `yaml:"url"`
			APIKey  string `yaml:"api_key"`
			Default bool   `yaml:"default"`
		}
		if err := root.Content[i+1].Decode(&entry); err != nil {
			return nil, fmt.Errorf("parse shop %q: %w", name, err)
		}
		if entry.URL == "" {
			return nil, fmt.Errorf("shop %q: url is required", name)
		}
		if entry.APIKey == "" {
			return nil, fmt.Errorf("shop %q: api_key is required", name)
		}
		if entry.Default {
			defaults++
			if defaults > 1 {
				return nil, fmt.Errorf("at most one shop may be marked default")
			}
			cfg.defaultName = name
		}
		cfg.Shops = append(cfg.Shops, &Shop{Name: name, URL: entry.URL, APIKey: entry.APIKey})
	}

	if len(cfg.Shops) == 0 {
		return nil, fmt.Errorf("no shops configured")
	}
	if cfg.defaultName == "" {
		cfg.defaultName = cfg.Shops[0].Name
	}
	return cfg, nil
}
