package app

type Server struct {
	Addr string `yaml:"addr"`
}

type NovaPoshtaConfig struct {
	APIKey     string `yaml:"api_key"`
	ServiceURL string `yaml:"service_url"` // overridden in tests; empty means use production URL
}

type GoogleSheetsConfig struct {
	CredentialsJSON string `yaml:"credentials_json"`
	SpreadsheetID   string `yaml:"spreadsheet_id"`
	SheetName       string `yaml:"sheet_name"`
	ServiceURL      string `yaml:"service_url"` // overridden in tests; empty means use production URL
}

type Config struct {
	Debug        bool               `yaml:"debug"`
	Server       Server             `yaml:"server"`
	DataDir      string             `yaml:"data_dir"`
	NovaPoshta   NovaPoshtaConfig   `yaml:"nova_poshta"`
	GoogleSheets GoogleSheetsConfig `yaml:"google_sheets"`
	RateLimit    int                `yaml:"rate_limit"` // requests per minute for POST /orders; 0 = default (1); negative = disabled
}
