package auth

import (
	"errors"
	"strings"
	"testing"
)

// REQ-AUTH-023 / SC-005: the limit counts bytes, not characters, and longer
// passwords are rejected instead of truncated.
func TestReqAuth023_AC01_RejectsMoreThan1024Bytes(t *testing.T) {
	cases := map[string]string{
		"1025 ASCII bytes":            strings.Repeat("a", 1025),
		"513 umlauts (1026 bytes)":    strings.Repeat("ä", 513),
		"342 euro signs (1026 bytes)": strings.Repeat("€", 342),
	}
	for name, password := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePassword(password); !errors.Is(err, ErrPasswordTooLong) {
				t.Fatalf("ValidatePassword(%d bytes) = %v, want ErrPasswordTooLong", len(password), err)
			}
		})
	}
}

// REQ-AUTH-023 / SC-005: passwords up to 1024 bytes, also above bcrypt's old
// 72-byte limit, are accepted and hashed in full.
func TestReqAuth023_AC02_Accepts1024Bytes(t *testing.T) {
	cases := map[string]string{
		"73 ASCII bytes":              strings.Repeat("a", 73),
		"1024 ASCII bytes":            strings.Repeat("a", 1024),
		"512 umlauts (1024 bytes)":    strings.Repeat("ä", 512),
		"341 euro signs (1023 bytes)": strings.Repeat("€", 341),
	}
	for name, password := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePassword(password); err != nil {
				t.Fatalf("ValidatePassword(%d bytes) = %v, want nil", len(password), err)
			}
			hash, err := HashPassword(password)
			if err != nil {
				t.Fatalf("HashPassword(%d bytes) = %v, want nil", len(password), err)
			}
			if err := verifyStoredPassword(hash, password); err != nil {
				t.Fatalf("verify %d bytes: %v", len(password), err)
			}
			if err := verifyStoredPassword(hash, password[:len(password)-1]); err == nil {
				t.Fatalf("a %d-byte password matched without its last byte", len(password))
			}
		})
	}
}
