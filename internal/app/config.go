package app

type Server struct {
	Addr      string `yaml:"addr"`
	PublicDir string `yaml:"public_dir"`
}

type Config struct {
	Debug   bool   `yaml:"debug"`
	Server  Server `yaml:"server"`
	DataDir string `yaml:"data_dir"`
}
