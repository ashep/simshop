package shop

// Shop holds the store metadata loaded from shop.yaml.
type Shop struct {
	Name        map[string]string `json:"name,omitempty"        yaml:"name"`
	Title       map[string]string `json:"title,omitempty"       yaml:"title"`
	Description map[string]string `json:"description,omitempty" yaml:"description"`
}
