package auth

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"raid-website/internal/player"
)

func testTOTPEncryption(t *testing.T) *TOTPEncryption {
	t.Helper()
	e, err := NewTOTPEncryption([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestReqAuth015_AC01(t *testing.T) {
	ctx := context.Background()
	db, svc := mfaLimitService(t)
	e := testTOTPEncryption(t)
	svc.sessions = NewRepo(db.DB).WithTOTPEncryption(e)
	if err := svc.sessions.MigrateTOTPSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.DB.NewSelect().Model((*MFASettings)(nil)).Column("totp_secret").Scan(ctx, &stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "v1:"+e.keyID+":") || strings.Contains(stored, mfaLimitSecret) {
		t.Fatal("secret is not encrypted")
	}
	token, _, err := svc.VerifyMFAChallenge(ctx, startMFALogin(t, svc), currentTOTPCode())
	if err != nil || token == "" {
		t.Fatalf("login after migration = %q, %v", token, err)
	}
	settings, err := svc.sessions.GetMFASettings(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.sessions.UpsertMFASettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
}

type failedNonceReader struct{}

func (failedNonceReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestReqAuth015_AC02(t *testing.T) {
	ctx := context.Background()
	db, svc := mfaLimitService(t)
	e := testTOTPEncryption(t)
	// Encrypting the first row succeeds; a rejected update of a second row must
	// roll back the first one as well.
	p := &player.Player{Name: "Second", Active: true}
	if err := svc.players.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := svc.sessions.UpsertMFASettings(ctx, &MFASettings{PlayerID: p.ID, TOTPSecret: mfaLimitSecret}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, "CREATE TRIGGER reject_second_secret BEFORE UPDATE ON player_mfa_settings WHEN NEW.player_id = 2 BEGIN SELECT RAISE(ABORT, 'do-not-leak'); END"); err != nil {
		t.Fatal(err)
	}
	repo := svc.sessions.WithTOTPEncryption(e)
	err := repo.MigrateTOTPSecrets(ctx)
	if err == nil || err.Error() != "TOTP migration failed for 1 row(s)" {
		t.Fatalf("migration error = %v", err)
	}
	var count int
	if err := db.DB.NewSelect().Model((*MFASettings)(nil)).Where("totp_secret = ?", mfaLimitSecret).ColumnExpr("count(*)").Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("rollback kept %d plaintext rows, want two", count)
	}
	e.random = failedNonceReader{}
	if _, err := e.encrypt(1, mfaLimitSecret); err == nil {
		t.Fatal("nonce failure ignored")
	}
}

func TestReqAuth015_AC03(t *testing.T) {
	ctx := context.Background()
	db, _ := mfaLimitService(t)
	repo := NewRepo(db.DB).WithTOTPEncryption(testTOTPEncryption(t))
	if err := repo.MigrateTOTPSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	var first string
	if err := db.DB.NewSelect().Model((*MFASettings)(nil)).Column("totp_secret").Scan(ctx, &first); err != nil {
		t.Fatal(err)
	}
	if err := repo.MigrateTOTPSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	var second string
	if err := db.DB.NewSelect().Model((*MFASettings)(nil)).Column("totp_secret").Scan(ctx, &second); err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("rerun rewrote an already migrated value")
	}
}

func TestReqAuth015_AC04(t *testing.T) {
	ctx := context.Background()
	db, _ := mfaLimitService(t)
	empty, err := NewTOTPEncryption(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewRepo(db.DB).WithTOTPEncryption(empty).MigrateTOTPSecrets(ctx); err == nil {
		t.Fatal("TOTP without a key accepted")
	}
	if _, err := NewTOTPEncryption([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
	if err := NewRepo(db.DB).WithTOTPEncryption(empty).UpsertMFASettings(ctx, &MFASettings{PlayerID: 1, TOTPSecret: mfaLimitSecret}); err == nil {
		t.Fatal("new unencrypted setup accepted")
	}
}

func TestReqAuth015_AC05(t *testing.T) {
	e := testTOTPEncryption(t)
	value, err := e.encrypt(1, mfaLimitSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.decrypt(2, value); !errors.Is(err, errTOTPEncryption) {
		t.Fatalf("copied secret error = %v", err)
	}
	for _, value := range []string{"v1:unknown:abc", "v1:" + e.keyID + ":bad", "v1:" + e.keyID + ":AAAA", mfaLimitSecret} {
		if _, err := e.decrypt(1, value); err == nil {
			t.Fatal("malformed or unknown-key ciphertext accepted")
		}
	}
}
