package app

type Database struct {
	DSN string `yaml:"dsn"`
}

type Server struct {
	Addr string `yaml:"addr"`
}

type Config struct {
	Debug    bool     `yaml:"debug"`
	Database Database `yaml:"database"`
	Server   Server   `yaml:"server"`
}
