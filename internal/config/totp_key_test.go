package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestTOTPEncryptionKey(t *testing.T) {
	for _, value := range []string{"invalid", base64.StdEncoding.EncodeToString([]byte("short"))} {
		t.Setenv("TOTP_ENCRYPTION_KEY", value)
		if _, err := Load(); err == nil {
			t.Fatal("malformed key accepted")
		}
	}
	raw := strings.Repeat("k", 32)
	t.Setenv("TOTP_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(raw)))
	cfg, err := Load()
	if err != nil || string(cfg.TOTPEncryptionKey) != raw {
		t.Fatalf("key load: %v", err)
	}
}
