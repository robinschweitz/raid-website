package auth

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// SC-005: new hashes are Argon2id in PHC format with a fresh salt each time.
func TestArgon2idRoundTrip(t *testing.T) {
	password := "a-secure-long-password"
	first, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two hashes of the same password share a salt")
	}
	if !strings.HasPrefix(first, argonPrefix) {
		t.Fatalf("hash %q does not start with %q", first, argonPrefix)
	}
	if err := verifyStoredPassword(first, password); err != nil {
		t.Fatalf("correct password rejected: %v", err)
	}
	if err := verifyStoredPassword(first, "wrong-password"); err == nil {
		t.Fatal("wrong password accepted")
	}
	if err := verifyStoredPassword(first+"x", password); err == nil {
		t.Fatal("corrupted hash accepted")
	}
}

// A stored hash with out-of-bounds parameters is rejected before hashing, so a
// corrupted row cannot make one login allocate unbounded memory.
func TestArgon2idRejectsUnsafeParameters(t *testing.T) {
	valid, err := HashPassword("a-secure-long-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, params := range []string{"m=4194304,t=3,p=1", "m=65536,t=0,p=1", "m=65536,t=100,p=1", "m=65536,t=3,p=0", "m=65536,t=03,p=1"} {
		hash := strings.Replace(valid, "m=65536,t=3,p=1", params, 1)
		if err := verifyStoredPassword(hash, "a-secure-long-password"); err == nil {
			t.Errorf("parameters %s accepted", params)
		}
	}
}

func TestNeedsRehash(t *testing.T) {
	current, err := HashPassword("a-secure-long-password")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := bcrypt.GenerateFromPassword([]byte("a-secure-long-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	older := strings.Replace(current, "t=3", "t=2", 1)
	for hash, want := range map[string]bool{current: false, string(legacy): true, older: true} {
		if got := needsRehash(hash); got != want {
			t.Errorf("needsRehash(%.30s...) = %v, want %v", hash, got, want)
		}
	}
}

// A legacy bcrypt hash keeps working and is replaced by Argon2id on the first
// successful login; a failed login leaves it unchanged.
func TestBcryptMigrationOnLogin(t *testing.T) {
	ctx := context.Background()
	database, svc := loginLimitService(t)
	legacy, err := bcrypt.GenerateFromPassword([]byte("valid-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, "UPDATE players SET password_hash = ? WHERE username = 'owner'", string(legacy)); err != nil {
		t.Fatal(err)
	}
	storedHash := func() string {
		t.Helper()
		var hash string
		if err := database.DB.QueryRowContext(ctx, "SELECT password_hash FROM players WHERE username = 'owner'").Scan(&hash); err != nil {
			t.Fatal(err)
		}
		return hash
	}

	if result, err := svc.StartLogin(ctx, "owner", "wrong-password", limitedAddress); result != nil || err == nil {
		t.Fatal("wrong password accepted")
	}
	if storedHash() != string(legacy) {
		t.Fatal("a failed login replaced the stored hash")
	}

	result, err := svc.StartLogin(ctx, "owner", "valid-password", limitedAddress)
	if err != nil || result == nil || result.Token == "" {
		t.Fatalf("login with a bcrypt hash: %v", err)
	}
	migrated := storedHash()
	if !strings.HasPrefix(migrated, argonPrefix) {
		t.Fatalf("hash after login = %.20s..., want Argon2id", migrated)
	}
	if result, err := svc.StartLogin(ctx, "owner", "valid-password", otherAddress); err != nil || result == nil {
		t.Fatalf("login with the migrated hash: %v", err)
	}
	if storedHash() != migrated {
		t.Fatal("a login with a current hash rewrote it")
	}
}

// The upgrade only writes while the verified hash is still stored, so a
// password change that ran meanwhile wins.
func TestUpgradePasswordHashKeepsConcurrentChange(t *testing.T) {
	ctx := context.Background()
	database, svc := loginLimitService(t)
	var playerID int64
	if err := database.DB.QueryRowContext(ctx, "SELECT id FROM players WHERE username = 'owner'").Scan(&playerID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, "UPDATE players SET password_hash = 'changed-meanwhile' WHERE id = ?", playerID); err != nil {
		t.Fatal(err)
	}
	if err := svc.sessions.UpgradePasswordHash(ctx, playerID, "verified-earlier", "upgraded"); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := database.DB.QueryRowContext(ctx, "SELECT password_hash FROM players WHERE id = ?", playerID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if hash != "changed-meanwhile" {
		t.Fatalf("hash = %q, want the concurrent change to survive", hash)
	}
}
