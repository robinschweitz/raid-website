package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	Port              string
	DatabaseURL       string
	Env               string
	CookieSecure      bool
	TrustProxyHeaders bool
	PublicOrigin      string
	AdminUsername     string
	AdminPassword     string
}

func Load() (*Config, error) {
	env := getEnv("ENV", "development")
	c := &Config{
		Port:              getEnv("PORT", "8080"),
		DatabaseURL:       getEnv("DATABASE_URL", "file:raid.db?_pragma=journal_mode(WAL)"),
		Env:               env,
		CookieSecure:      getEnv("COOKIE_SECURE", "") == "true" || env == "production",
		TrustProxyHeaders: getEnv("TRUST_PROXY_HEADERS", "") == "true",
		PublicOrigin:      strings.TrimSpace(getEnv("PUBLIC_ORIGIN", "")),
		AdminUsername:     getEnv("ADMIN_USERNAME", "Robin"),
		AdminPassword:     getEnv("ADMIN_PASSWORD", "development-password"),
	}
	if env == "production" {
		username := strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
		password := os.Getenv("ADMIN_PASSWORD")
		if username == "" || strings.EqualFold(username, "Robin") || strings.TrimSpace(password) == "" || password == "test" || password == "development-password" {
			return nil, errors.New("production requires explicit nondefault ADMIN_USERNAME and ADMIN_PASSWORD")
		}
	}
	return c, nil
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
