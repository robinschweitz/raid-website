package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"raid-website/internal/db"
	"raid-website/internal/player"
)

const mfaLimitSecret = "JBSWY3DPEHPK3PXP"

// mfaLimitService returns a service whose bootstrap Admin "owner" has TOTP.
func mfaLimitService(t *testing.T) (*db.BunDB, *Service) {
	t.Helper()
	ctx := context.Background()
	bdb, svc := loginLimitService(t)
	owner, err := player.NewRepo(bdb.DB).FindByUsername(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := svc.sessions.UpsertMFASettings(ctx, &MFASettings{PlayerID: owner.ID, TOTPSecret: mfaLimitSecret, TOTPEnabledAt: &now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return bdb, svc
}

func startMFALogin(t *testing.T, svc *Service) string {
	t.Helper()
	result, err := svc.StartLogin(context.Background(), "owner", "valid-password", "")
	if err != nil || result.MFAChallenge == "" {
		t.Fatalf("StartLogin = %+v, %v; want an MFA challenge", result, err)
	}
	return result.MFAChallenge
}

func currentTOTPCode() string {
	return generateTOTP(mfaLimitSecret, time.Now().UTC().Unix()/totpPeriodSeconds)
}

// wrongTOTPCode returns a six-digit code that matches none of the time steps
// verifyTOTP accepts right now.
func wrongTOTPCode() string {
	step := time.Now().UTC().Unix() / totpPeriodSeconds
	for _, candidate := range []string{"000000", "111111", "222222", "333333"} {
		if candidate != generateTOTP(mfaLimitSecret, step-1) && candidate != generateTOTP(mfaLimitSecret, step) && candidate != generateTOTP(mfaLimitSecret, step+1) {
			return candidate
		}
	}
	return "999999"
}

func countSessions(t *testing.T, bdb *db.BunDB) int {
	t.Helper()
	var count int
	if err := bdb.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestReqAuth013_AC01(t *testing.T) {
	ctx := context.Background()
	bdb, svc := mfaLimitService(t)
	challenge := startMFALogin(t, svc)
	for i := 1; i < mfaFailureLimit; i++ {
		if _, _, err := svc.VerifyMFAChallenge(ctx, challenge, wrongTOTPCode()); !errors.Is(err, ErrInvalidMFA) {
			t.Fatalf("wrong code %d: err = %v, want invalid code", i, err)
		}
	}
	if _, _, err := svc.VerifyMFAChallenge(ctx, challenge, wrongTOTPCode()); !errors.Is(err, ErrMFAChallengeInvalid) {
		t.Fatalf("wrong code %d: err = %v, want challenge invalid", mfaFailureLimit, err)
	}
	var challenges int
	if err := bdb.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM mfa_login_challenges").Scan(&challenges); err != nil || challenges != 0 {
		t.Fatalf("challenges after the limit = %d, %v; want the challenge deleted", challenges, err)
	}
	if _, _, err := svc.VerifyMFAChallenge(ctx, challenge, currentTOTPCode()); !errors.Is(err, ErrMFAChallengeInvalid) {
		t.Fatalf("correct code after the limit: err = %v, want challenge invalid", err)
	}
	if n := countSessions(t, bdb); n != 0 {
		t.Fatalf("sessions = %d, want none", n)
	}
}

func TestReqAuth013_AC02(t *testing.T) {
	ctx := context.Background()
	bdb, svc := mfaLimitService(t)
	challenge := startMFALogin(t, svc)
	for i := 1; i < mfaFailureLimit; i++ {
		if _, _, err := svc.VerifyMFAChallenge(ctx, challenge, wrongTOTPCode()); !errors.Is(err, ErrInvalidMFA) {
			t.Fatalf("wrong code %d: err = %v", i, err)
		}
	}
	token, p, err := svc.VerifyMFAChallenge(ctx, challenge, currentTOTPCode())
	if err != nil || token == "" || p == nil {
		t.Fatalf("correct code after %d wrong ones = %q, %v, %v; want a session", mfaFailureLimit-1, token, p, err)
	}
	if n := countSessions(t, bdb); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
}

func TestReqAuth013_AC03(t *testing.T) {
	ctx := context.Background()
	_, svc := mfaLimitService(t)
	closed := startMFALogin(t, svc)
	for i := 0; i < mfaFailureLimit; i++ {
		_, _, _ = svc.VerifyMFAChallenge(ctx, closed, wrongTOTPCode())
	}
	fresh := startMFALogin(t, svc)
	if token, _, err := svc.VerifyMFAChallenge(ctx, fresh, currentTOTPCode()); err != nil || token == "" {
		t.Fatalf("new challenge after an invalidated one = %q, %v; want a session", token, err)
	}
}

func TestReqAuth013_AC04(t *testing.T) {
	ctx := context.Background()
	bdb, svc := mfaLimitService(t)
	challenge := startMFALogin(t, svc)
	if _, err := bdb.DB.ExecContext(ctx, "CREATE TRIGGER reject_mfa_failure BEFORE UPDATE OF failed_attempts ON mfa_login_challenges BEGIN SELECT RAISE(ABORT, 'injected failure'); END"); err != nil {
		t.Fatal(err)
	}
	_, _, err := svc.VerifyMFAChallenge(ctx, challenge, wrongTOTPCode())
	if err == nil || errors.Is(err, ErrInvalidMFA) || errors.Is(err, ErrMFAChallengeInvalid) {
		t.Fatalf("wrong code with a failing counter: err = %v, want an internal error", err)
	}
	if n := countSessions(t, bdb); n != 0 {
		t.Fatalf("sessions = %d, want none", n)
	}
}

// A challenge that reached the limit but could not be deleted stays closed.
func TestReqAuth013_ChallengeAtLimitStaysClosed(t *testing.T) {
	ctx := context.Background()
	bdb, svc := mfaLimitService(t)
	challenge := startMFALogin(t, svc)
	if _, err := bdb.DB.ExecContext(ctx, "UPDATE mfa_login_challenges SET failed_attempts = ?", mfaFailureLimit); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.VerifyMFAChallenge(ctx, challenge, currentTOTPCode()); !errors.Is(err, ErrMFAChallengeInvalid) {
		t.Fatalf("correct code on a challenge at the limit: err = %v, want challenge invalid", err)
	}
}
