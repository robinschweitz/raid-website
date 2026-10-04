package config

import "os"

type Config struct {
	Port          string
	DatabaseURL   string
	Env           string
	CookieSecure  bool
	AdminUsername string
	AdminPassword string
}

func Load() (*Config, error) {
	env := getEnv("ENV", "development")
	c := &Config{
		Port:          getEnv("PORT", "8080"),
		DatabaseURL:   getEnv("DATABASE_URL", "file:raid.db?_pragma=journal_mode(WAL)"),
		Env:           env,
		CookieSecure:  getEnv("COOKIE_SECURE", "") == "true" || env == "production",
		AdminUsername: getEnv("ADMIN_USERNAME", "Robin"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "test"),
	}
	return c, nil
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
