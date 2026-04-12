package app

type Database struct {
	DSN string `yaml:"dsn"`
}

type Server struct {
	Addr string `yaml:"addr"`
}

type Files struct {
	MaxSize       int      `yaml:"max_size"`
	MaxNumPerUser int      `yaml:"max_num_per_user"`
	AllowedTypes  []string `yaml:"allowed_types"`
}

type Config struct {
	Debug    bool     `yaml:"debug"`
	Database Database `yaml:"database"`
	Server   Server   `yaml:"server"`
	Files    Files    `yaml:"files"`
}
