package app

type Server struct {
	Addr           string   `yaml:"addr"`
	CORSOrigins    []string `yaml:"cors_origins"`
}

type Config struct {
	Debug   bool   `yaml:"debug"`
	Server  Server `yaml:"server"`
	DataDir string `yaml:"data_dir"`
}
