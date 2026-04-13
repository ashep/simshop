package app

type Server struct {
	Addr string `yaml:"addr"`
}

type Config struct {
	Debug   bool   `yaml:"debug"`
	Server  Server `yaml:"server"`
	DataDir string `yaml:"data_dir"`
}
