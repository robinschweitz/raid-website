package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// recordPasswordChecks replaces the login password comparison with a wrapper
// that records the hash of every comparison.
func recordPasswordChecks(t *testing.T) *[]string {
	t.Helper()
	var hashes []string
	original := comparePassword
	comparePassword = func(hash, password []byte) error {
		hashes = append(hashes, string(hash))
		return verifyStoredPassword(string(hash), string(password))
	}
	t.Cleanup(func() { comparePassword = original })
	return &hashes
}

// The dummy hash must cost exactly what a real hash costs (SC-009).
func TestDummyPasswordHashUsesCurrentParameters(t *testing.T) {
	if !strings.HasPrefix(dummyPasswordHash, argonPrefix) {
		t.Fatalf("dummy hash %q does not use the current Argon2id parameters %q", dummyPasswordHash, argonPrefix)
	}
	if err := verifyStoredPassword(dummyPasswordHash, "any password"); !errors.Is(err, errPasswordMismatch) {
		t.Fatalf("dummy hash check = %v, want a plain mismatch", err)
	}
}

func TestReqAuth012_AC01(t *testing.T) {
	ctx := context.Background()
	bdb, svc := loginLimitService(t)
	if _, err := bdb.DB.ExecContext(ctx, "UPDATE players SET password_hash = '' WHERE username = 'owner'"); err != nil {
		t.Fatal(err)
	}
	for _, username := range []string{"nobody", "owner"} {
		hashes := recordPasswordChecks(t)
		_, err := svc.StartLogin(ctx, username, "valid-password", limitedAddress)
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("%s: err = %v, want invalid credentials", username, err)
		}
		if len(*hashes) != 1 || (*hashes)[0] != dummyPasswordHash {
			t.Fatalf("%s: compared against %v, want one comparison with the dummy hash", username, *hashes)
		}
	}
}

func TestReqAuth012_AC02(t *testing.T) {
	ctx := context.Background()
	_, svc := loginLimitService(t)
	hashes := recordPasswordChecks(t)
	_, unknownErr := svc.StartLogin(ctx, "nobody", "wrong-password", limitedAddress)
	_, wrongErr := svc.StartLogin(ctx, "owner", "wrong-password", limitedAddress)
	if unknownErr != wrongErr || !errors.Is(wrongErr, ErrInvalidCredentials) {
		t.Fatalf("unknown = %v, wrong password = %v; want the same invalid-credentials error", unknownErr, wrongErr)
	}
	if len(*hashes) != 2 {
		t.Fatalf("password comparisons = %d, want one per login", len(*hashes))
	}
}

func TestReqAuth012_AC03(t *testing.T) {
	ctx := context.Background()
	bdb, svc := loginLimitService(t)
	if _, err := bdb.DB.ExecContext(ctx, "UPDATE players SET active = 0 WHERE username = 'owner'"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		password string
		want     error
	}{
		{"wrong-password", ErrInvalidCredentials},
		{"valid-password", ErrInactive},
	} {
		hashes := recordPasswordChecks(t)
		if _, err := svc.StartLogin(ctx, "owner", tc.password, limitedAddress); !errors.Is(err, tc.want) {
			t.Fatalf("password %q: err = %v, want %v", tc.password, err, tc.want)
		}
		if len(*hashes) != 1 || (*hashes)[0] == dummyPasswordHash {
			t.Fatalf("password %q: comparisons %v, want one against the account hash", tc.password, *hashes)
		}
	}
}
