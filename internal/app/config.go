package app

type Database struct {
	DSN string `yaml:"dsn"`
}

type Config struct {
	Debug    bool     `yaml:"debug"`
	Database Database `yaml:"database"`
}
