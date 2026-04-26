package app

type Server struct {
	Addr   string `yaml:"addr"`
	APIKey string `yaml:"api_key"` // bearer token required for protected admin routes; empty disables them
}

type NovaPoshtaConfig struct {
	APIKey     string `yaml:"api_key"`
	ServiceURL string `yaml:"service_url"` // overridden in tests; empty means use production URL
}

type MonobankConfig struct {
	APIKey      string `yaml:"api_key"`      // X-Token; empty → startup error
	ServiceURL  string `yaml:"service_url"`  // overridden in tests; empty means use production URL
	RedirectURL string `yaml:"redirect_url"` // post-payment customer landing page; empty → startup error
	WebhookURL  string `yaml:"webhook_url"`  // public https URL Monobank posts invoice-status webhooks to; empty → startup error
	TaxIDs      []int  `yaml:"tax_ids"`      // merchant tax registration IDs from the Monobank business cabinet; required when fiscalization is enabled
}

type DBConfig struct {
	DSN string `yaml:"dsn"`
}

type Config struct {
	Debug      bool             `yaml:"debug"`
	Server     Server           `yaml:"server"`
	DataDir    string           `yaml:"data_dir"`
	NovaPoshta NovaPoshtaConfig `yaml:"nova_poshta"`
	Monobank   MonobankConfig   `yaml:"monobank"`
	Database   DBConfig         `yaml:"database"`
	RateLimit  int              `yaml:"rate_limit"` // requests per minute for POST /orders; 0 = default (1); negative = disabled
}
