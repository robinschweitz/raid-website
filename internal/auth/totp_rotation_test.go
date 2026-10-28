package auth

import (
	"context"
	"strings"
	"testing"
)

func TestReqAuth016_AC01(t *testing.T) {
	ctx := context.Background()
	db, svc := mfaLimitService(t)
	oldKey := []byte(strings.Repeat("a", 32))
	currentKey := []byte(strings.Repeat("b", 32))
	old, err := NewTOTPEncryption(oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.sessions.WithTOTPEncryption(old).MigrateTOTPSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	rotating, err := NewTOTPEncryptionWithPrevious(currentKey, [][]byte{oldKey})
	if err != nil {
		t.Fatal(err)
	}
	svc.sessions = NewRepo(db.DB).WithTOTPEncryption(rotating)
	if err := svc.sessions.MigrateTOTPSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.DB.NewSelect().Model((*MFASettings)(nil)).Column("totp_secret").Scan(ctx, &stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "v1:"+rotating.keyID+":") {
		t.Fatal("row was not rotated")
	}
	// The documented final restart no longer needs the previous live key.
	current, err := NewTOTPEncryption(currentKey)
	if err != nil {
		t.Fatal(err)
	}
	svc.sessions = NewRepo(db.DB).WithTOTPEncryption(current)
	if err := svc.sessions.MigrateTOTPSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	token, _, err := svc.VerifyMFAChallenge(ctx, startMFALogin(t, svc), currentTOTPCode())
	if err != nil || token == "" {
		t.Fatalf("rotated login=%q %v", token, err)
	}
}

func TestReqAuth016_AC02(t *testing.T) {
	ctx := context.Background()
	db, svc := mfaLimitService(t)
	old := testTOTPEncryption(t)
	if err := svc.sessions.WithTOTPEncryption(old).MigrateTOTPSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := NewTOTPEncryption([]byte(strings.Repeat("b", 32)))
	if err != nil {
		t.Fatal(err)
	}
	if err := NewRepo(db.DB).WithTOTPEncryption(current).MigrateTOTPSecrets(ctx); err == nil {
		t.Fatal("unknown key allowed startup")
	}
	row, err := svc.sessions.WithTOTPEncryption(old).GetMFASettings(ctx, 1)
	if err != nil || row.TOTPSecret != mfaLimitSecret {
		t.Fatalf("failed rotation changed the original: %v", err)
	}
}
