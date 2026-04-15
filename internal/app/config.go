package app

type Server struct {
	Addr string `yaml:"addr"`
}

type NovaPoshtaConfig struct {
	APIKey     string `yaml:"api_key"`
	ServiceURL string `yaml:"service_url"` // overridden in tests; empty means use production URL
}

type Config struct {
	Debug      bool            `yaml:"debug"`
	Server     Server          `yaml:"server"`
	DataDir    string          `yaml:"data_dir"`
	NovaPoshta NovaPoshtaConfig `yaml:"nova_poshta"`
}
