package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	Port                    string
	DatabaseURL             string
	Env                     string
	CookieSecure            bool
	TrustProxyHeaders       bool
	PublicOrigin            string
	AdminUsername           string
	AdminPassword           string
	RisingGodsProxyURL      string
	RisingGodsProxyToken    string
	SheetSyncEnabled        bool
	SheetSyncSheetID        string
	SheetSyncBackupSheetID  string
	SheetSyncReason         string
	SheetSyncInterval       time.Duration
	SheetSyncServiceAccount string
}

func Load() (*Config, error) {
	env := getEnv("ENV", "development")
	c := &Config{
		Port:                    getEnv("PORT", "8080"),
		DatabaseURL:             getEnv("DATABASE_URL", "file:raid.db?_pragma=journal_mode(WAL)"),
		Env:                     env,
		CookieSecure:            getEnv("COOKIE_SECURE", "") == "true" || env == "production",
		TrustProxyHeaders:       getEnv("TRUST_PROXY_HEADERS", "") == "true",
		PublicOrigin:            strings.TrimSpace(getEnv("PUBLIC_ORIGIN", "")),
		AdminUsername:           getEnv("ADMIN_USERNAME", "Robin"),
		AdminPassword:           getEnv("ADMIN_PASSWORD", "development-password"),
		RisingGodsProxyURL:      strings.TrimRight(strings.TrimSpace(getEnv("RISING_GODS_PROXY_URL", "")), "/"),
		RisingGodsProxyToken:    strings.TrimSpace(getEnv("RISING_GODS_PROXY_TOKEN", "")),
		SheetSyncEnabled:        getEnv("SHEET_SYNC_ENABLED", "") == "true",
		SheetSyncSheetID:        strings.TrimSpace(getEnv("SHEET_SYNC_SHEET_ID", "")),
		SheetSyncBackupSheetID:  strings.TrimSpace(getEnv("SHEET_SYNC_BACKUP_SHEET_ID", "")),
		SheetSyncReason:         strings.TrimSpace(getEnv("SHEET_SYNC_REASON", "")),
		SheetSyncServiceAccount: strings.TrimSpace(getEnv("SHEET_SYNC_SERVICE_ACCOUNT_JSON", "")),
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
	if c.SheetSyncEnabled {
		if c.SheetSyncSheetID != "10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8" || c.SheetSyncReason == "" || c.SheetSyncServiceAccount == "" {
			return nil, errors.New("SHEET_SYNC_SHEET_ID set to the explicitly authorized copy, SHEET_SYNC_REASON and SHEET_SYNC_SERVICE_ACCOUNT_JSON required when SHEET_SYNC_ENABLED=true")
		}
		interval, err := time.ParseDuration(getEnv("SHEET_SYNC_INTERVAL", "10m"))
		if err != nil || interval < time.Minute {
			return nil, errors.New("SHEET_SYNC_INTERVAL must be a duration of at least 1m")
		}
		c.SheetSyncInterval = interval
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
