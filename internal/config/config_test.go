package config

import "testing"

func TestProductionCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, username, password string
		valid                    bool
	}{
		{"missing", "", "", false},
		{"missing username", "", "unique-password", false},
		{"missing password", "owner", "", false},
		{"default username", "Robin", "unique-password", false},
		{"normalized default username", " ROBIN ", "unique-password", false},
		{"default password", "owner", "test", false},
		{"development default password", "owner", "development-password", false},
		{"blank username", " \t", "unique-password", false},
		{"blank password", "owner", " \t", false},
		{"explicit credentials", "owner", "unique-password", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENV", "production")
			t.Setenv("ADMIN_USERNAME", tc.username)
			t.Setenv("ADMIN_PASSWORD", tc.password)
			t.Setenv("COOKIE_SECURE", "false")
			cfg, err := Load()
			if (err == nil) != tc.valid {
				t.Fatalf("Load error = %v, want valid %v", err, tc.valid)
			}
			if tc.valid && !cfg.CookieSecure {
				t.Fatal("production cookies must be secure")
			}
		})
	}
}

func TestDevelopmentDefaults(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ADMIN_PASSWORD", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdminUsername != "Robin" || cfg.AdminPassword != "development-password" {
		t.Fatal("development defaults changed")
	}
}

func TestRisingGodsProxyConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, proxyURL, token string
		valid                 bool
	}{
		{"disabled", "", "", true},
		{"missing token", "https://proxy.example", "", false},
		{"missing URL", "", "token", false},
		{"non HTTPS URL", "http://proxy.example", "token", false},
		{"URL path", "https://proxy.example/path", "token", false},
		{"enabled", "https://proxy.example/", "token", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RISING_GODS_PROXY_URL", tc.proxyURL)
			t.Setenv("RISING_GODS_PROXY_TOKEN", tc.token)
			cfg, err := Load()
			if (err == nil) != tc.valid {
				t.Fatalf("Load error = %v, want valid %v", err, tc.valid)
			}
			if tc.valid && tc.proxyURL != "" && cfg.RisingGodsProxyURL != "https://proxy.example" {
				t.Fatalf("unexpected normalized proxy URL %q", cfg.RisingGodsProxyURL)
			}
		})
	}
}
