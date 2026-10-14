package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Port                     string
	DatabaseURL              string
	Env                      string
	CookieSecure             bool
	TrustProxyHeaders        bool
	PublicOrigin             string
	AdminUsername            string
	AdminPassword            string
	RisingGodsProxyURL       string
	RisingGodsProxyToken     string
	GoogleServiceAccountFile string
}

func Load() (*Config, error) {
	env := getEnv("ENV", "development")
	c := &Config{
		Port:                     getEnv("PORT", "8080"),
		DatabaseURL:              getEnv("DATABASE_URL", "file:raid.db?_pragma=journal_mode(WAL)"),
		Env:                      env,
		CookieSecure:             getEnv("COOKIE_SECURE", "") == "true" || env == "production",
		TrustProxyHeaders:        getEnv("TRUST_PROXY_HEADERS", "") == "true",
		PublicOrigin:             strings.TrimSpace(getEnv("PUBLIC_ORIGIN", "")),
		AdminUsername:            getEnv("ADMIN_USERNAME", "Robin"),
		AdminPassword:            getEnv("ADMIN_PASSWORD", "development-password"),
		RisingGodsProxyURL:       strings.TrimRight(strings.TrimSpace(getEnv("RISING_GODS_PROXY_URL", "")), "/"),
		RisingGodsProxyToken:     strings.TrimSpace(getEnv("RISING_GODS_PROXY_TOKEN", "")),
		GoogleServiceAccountFile: strings.TrimSpace(getEnv("GOOGLE_SERVICE_ACCOUNT_FILE", "F:\\raid-website\\raid-app-sync-93fc5aa7926b.json")),
	}
	if (c.RisingGodsProxyURL == "") != (c.RisingGodsProxyToken == "") {
		return nil, errors.New("RISING_GODS_PROXY_URL and RISING_GODS_PROXY_TOKEN must be set together")
	}
	if c.RisingGodsProxyURL != "" {
		u, err := url.Parse(c.RisingGodsProxyURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" {
			return nil, errors.New("RISING_GODS_PROXY_URL must be an https origin without a path")
		}
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
