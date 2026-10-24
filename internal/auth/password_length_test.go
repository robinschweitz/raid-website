package auth

import (
	"errors"
	"strings"
	"testing"
)

// REQ-AUTH-023 / SC-005: bcrypt takes at most 72 bytes, so the limit counts bytes, not characters.
func TestReqAuth023_AC01_RejectsMoreThan72Bytes(t *testing.T) {
	cases := map[string]string{
		"73 ASCII bytes":           strings.Repeat("a", 73),
		"37 umlauts (74 bytes)":    strings.Repeat("ä", 37),
		"25 euro signs (75 bytes)": strings.Repeat("€", 25),
	}
	for name, password := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePassword(password); !errors.Is(err, ErrPasswordTooLong) {
				t.Fatalf("ValidatePassword(%d bytes) = %v, want ErrPasswordTooLong", len(password), err)
			}
		})
	}
}

// REQ-AUTH-023 / SC-005: a 72-byte password is accepted and hashed without truncation.
func TestReqAuth023_AC02_Accepts72Bytes(t *testing.T) {
	cases := map[string]string{
		"72 ASCII bytes":           strings.Repeat("a", 72),
		"36 umlauts (72 bytes)":    strings.Repeat("ä", 36),
		"24 euro signs (72 bytes)": strings.Repeat("€", 24),
	}
	for name, password := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePassword(password); err != nil {
				t.Fatalf("ValidatePassword(%d bytes) = %v, want nil", len(password), err)
			}
			if _, err := HashPassword(password); err != nil {
				t.Fatalf("HashPassword(%d bytes) = %v, want nil", len(password), err)
			}
		})
	}
}
