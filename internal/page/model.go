package page

// Page holds metadata for a single content page.
type Page struct {
	ID    string            `json:"id"    yaml:"id"`
	Title map[string]string `json:"title" yaml:"title"`
}
