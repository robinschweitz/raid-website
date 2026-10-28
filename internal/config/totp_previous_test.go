package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestPreviousTOTPKeys(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	t.Setenv("TOTP_ENCRYPTION_KEY", key)
	t.Setenv("TOTP_ENCRYPTION_KEYS_PREVIOUS", key+", "+key)
	cfg, err := Load()
	if err != nil || len(cfg.TOTPEncryptionKeysPrevious) != 2 {
		t.Fatalf("previous keys: %v", err)
	}
	for _, value := range []string{"invalid", key + ","} {
		t.Setenv("TOTP_ENCRYPTION_KEYS_PREVIOUS", value)
		if _, err := Load(); err == nil {
			t.Fatal("malformed previous keys accepted")
		}
	}
	t.Setenv("TOTP_ENCRYPTION_KEY", "")
	t.Setenv("TOTP_ENCRYPTION_KEYS_PREVIOUS", key)
	if _, err := Load(); err == nil {
		t.Fatal("previous keys without current accepted")
	}
}
