package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"raid-website/internal/player"
)

func storedTOTPStep(t *testing.T, svc *Service) *int64 {
	t.Helper()
	ctx := context.Background()
	owner, err := svc.players.FindByUsername(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := svc.sessions.GetMFASettings(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	return settings.TOTPLastStep
}

func TestReqAuth014_AC01(t *testing.T) {
	ctx := context.Background()
	_, svc := mfaLimitService(t)
	code := currentTOTPCode()
	if token, _, err := svc.VerifyMFAChallenge(ctx, startMFALogin(t, svc), code); err != nil || token == "" {
		t.Fatalf("first use = %q, %v; want a session", token, err)
	}
	if _, _, err := svc.VerifyMFAChallenge(ctx, startMFALogin(t, svc), code); !errors.Is(err, ErrInvalidMFA) {
		t.Fatalf("second use of the same code: err = %v, want invalid code", err)
	}
}

func TestReqAuth014_AC02(t *testing.T) {
	ctx := context.Background()
	bdb, svc := mfaLimitService(t)
	step := time.Now().UTC().Unix() / totpPeriodSeconds
	// Step N-1 was used; the code for the current step N is accepted.
	if _, err := bdb.DB.ExecContext(ctx, "UPDATE player_mfa_settings SET totp_last_step = ?", step-1); err != nil {
		t.Fatal(err)
	}
	if token, _, err := svc.VerifyMFAChallenge(ctx, startMFALogin(t, svc), generateTOTP(mfaLimitSecret, step)); err != nil || token == "" {
		t.Fatalf("code for the next step = %q, %v; want a session", token, err)
	}
	if got := storedTOTPStep(t, svc); got == nil || *got != step {
		t.Fatalf("stored step = %v, want %d", got, step)
	}
	// A still-valid code for the earlier step is now rejected.
	if _, _, err := svc.VerifyMFAChallenge(ctx, startMFALogin(t, svc), generateTOTP(mfaLimitSecret, step-1)); !errors.Is(err, ErrInvalidMFA) {
		t.Fatalf("code for an earlier step: err = %v, want invalid code", err)
	}
}

func TestReqAuth014_AC03(t *testing.T) {
	ctx := context.Background()
	bdb, svc := mfaLimitService(t)
	code := currentTOTPCode()
	challenges := []string{startMFALogin(t, svc), startMFALogin(t, svc)}
	errs := make([]error, len(challenges))
	var wg sync.WaitGroup
	for i, challenge := range challenges {
		wg.Add(1)
		go func(i int, challenge string) {
			defer wg.Done()
			_, _, errs[i] = svc.VerifyMFAChallenge(ctx, challenge, code)
		}(i, challenge)
	}
	wg.Wait()
	succeeded := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case !errors.Is(err, ErrInvalidMFA):
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || countSessions(t, bdb) != 1 {
		t.Fatalf("succeeded = %d, sessions = %d; want exactly one of each", succeeded, countSessions(t, bdb))
	}
}

// The code that enables TOTP cannot be replayed to log in.
func TestReqAuth014_EnableCodeIsUsedOnce(t *testing.T) {
	ctx := context.Background()
	bdb, svc := loginLimitService(t)
	owner, err := player.NewRepo(bdb.DB).FindByUsername(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	setup, err := svc.SetupTOTP(ctx, owner.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	code := generateTOTP(setup.Secret, time.Now().UTC().Unix()/totpPeriodSeconds)
	if _, err := svc.EnableTOTP(ctx, owner.ID, code); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.VerifyMFAChallenge(ctx, startMFALogin(t, svc), code); !errors.Is(err, ErrInvalidMFA) {
		t.Fatalf("login with the enable code: err = %v, want invalid code", err)
	}
}
