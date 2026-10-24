package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Port                 string
	ListenAddr           string
	DatabaseURL          string
	Env                  string
	CookieSecure         bool
	TrustProxyHeaders    bool
	PublicOrigin         string
	AdminUsername        string
	AdminPassword        string
	RisingGodsProxyURL   string
	RisingGodsProxyToken string
	// VAPIDPrivateKey and VAPIDSubject switch Web Push on (FR-32, SC-054).
	VAPIDPrivateKey string
	VAPIDSubject    string
}

func Load() (*Config, error) {
	env := getEnv("ENV", "development")
	port := getEnv("PORT", "8080")
	listenAddr := getEnv("LISTEN_ADDR", "")
	if listenAddr == "" {
		if env == "production" {
			listenAddr = "127.0.0.1:" + port
		} else {
			listenAddr = ":" + port
		}
	}
	c := &Config{
		Port:                 port,
		ListenAddr:           listenAddr,
		DatabaseURL:          getEnv("DATABASE_URL", "file:raid.db?_pragma=journal_mode(WAL)"),
		Env:                  env,
		CookieSecure:         getEnv("COOKIE_SECURE", "") == "true" || env == "production",
		TrustProxyHeaders:    getEnv("TRUST_PROXY_HEADERS", "") == "true",
		PublicOrigin:         strings.TrimSpace(getEnv("PUBLIC_ORIGIN", "")),
		AdminUsername:        getEnv("ADMIN_USERNAME", "Robin"),
		AdminPassword:        getEnv("ADMIN_PASSWORD", "development-password"),
		RisingGodsProxyURL:   strings.TrimRight(strings.TrimSpace(getEnv("RISING_GODS_PROXY_URL", "")), "/"),
		RisingGodsProxyToken: strings.TrimSpace(getEnv("RISING_GODS_PROXY_TOKEN", "")),
		VAPIDPrivateKey:      strings.TrimSpace(getEnv("VAPID_PRIVATE_KEY", "")),
		VAPIDSubject:         strings.TrimSpace(getEnv("VAPID_SUBJECT", "")),
	}
	if (c.VAPIDPrivateKey == "") != (c.VAPIDSubject == "") {
		return nil, errors.New("VAPID_PRIVATE_KEY and VAPID_SUBJECT must be set together")
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
		host, _, err := net.SplitHostPort(c.ListenAddr)
		if err != nil || host == "" {
			return nil, errors.New("LISTEN_ADDR must be a host:port address")
		}
		ip := net.ParseIP(host)
		if (ip != nil && !ip.IsLoopback()) || (ip == nil && !strings.EqualFold(host, "localhost")) {
			return nil, errors.New("production LISTEN_ADDR must use a loopback host")
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
