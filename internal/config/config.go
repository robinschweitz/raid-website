package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
	Env         string
}

func Load() (*Config, error) {
	c := &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "file:raid.db?_pragma=journal_mode(WAL)"),
		Env:         getEnv("ENV", "development"),
	}
	return c, nil
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
