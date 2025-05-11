package config

type Config struct {
	UploadDir          string      `json:"upload_dir"`
	JWTSecret          string      `json:"jwt_secret"`
	Redis              RedisConfig `json:"redis"`
	Port               string      `json:"port"`
	MaxUploadSize      int64       `json:"max_upload_size"`
	TopRefreshInterval int         `json:"top_refresh_interval"`
	RateLimit          struct {
		Requests int `json:"requests"`
		Duration int `json:"duration"`
	} `json:"rate_limit"`
}

type RedisConfig struct {
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`
	PoolSize int    `json:"pool_size"`
}
