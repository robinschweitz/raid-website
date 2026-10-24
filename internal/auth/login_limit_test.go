package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"raid-website/internal/db"
	"raid-website/internal/player"
)

const (
	limitedAddress = "203.0.113.7"
	otherAddress   = "198.51.100.4"
)

// countPasswordChecks replaces the login password comparison with a counting
// wrapper for the duration of the test.
func countPasswordChecks(t *testing.T) *int {
	t.Helper()
	calls := 0
	original := comparePassword
	comparePassword = func(hash, password []byte) error {
		calls++
		return verifyStoredPassword(string(hash), string(password))
	}
	t.Cleanup(func() { comparePassword = original })
	return &calls
}

func loginLimitService(t *testing.T) (*db.BunDB, *Service) {
	t.Helper()
	bdb := reliabilityDB(t)
	svc := NewService(player.NewRepo(bdb.DB), NewRepo(bdb.DB))
	if err := svc.BootstrapAdmin(context.Background(), "owner", "valid-password"); err != nil {
		t.Fatal(err)
	}
	return bdb, svc
}

// failFromAddress makes count failed logins from one address, each for a
// different username, so the per-username limit (SC-008) never applies.
func failFromAddress(t *testing.T, svc *Service, address string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		_, err := svc.StartLogin(context.Background(), fmt.Sprintf("guess-%02d", i), "wrong-password", address)
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: err = %v, want invalid credentials", i+1, err)
		}
	}
}

func TestReqAuth011_AC01(t *testing.T) {
	ctx := context.Background()
	_, svc := loginLimitService(t)
	failFromAddress(t, svc, limitedAddress, addressFailureLimit-1)

	// The 30th attempt is still evaluated normally.
	if _, err := svc.StartLogin(ctx, "guess-last", "wrong-password", limitedAddress); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("30th attempt err = %v, want invalid credentials", err)
	}

	calls := countPasswordChecks(t)
	result, err := svc.StartLogin(ctx, "owner", "valid-password", limitedAddress)
	if !errors.Is(err, ErrLoginRateLimited) || result != nil {
		t.Fatalf("31st attempt = %v, %v; want rate limited", result, err)
	}
	if *calls != 0 {
		t.Fatalf("rate-limited attempt ran %d password comparisons, want 0", *calls)
	}
}

func TestReqAuth011_AC02(t *testing.T) {
	ctx := context.Background()
	_, svc := loginLimitService(t)
	old := time.Now().UTC().Add(-loginFailureWindow - time.Minute)
	for i := 0; i < addressFailureLimit; i++ {
		if err := svc.sessions.CreateLoginAttempt(ctx, &LoginAttempt{Username: fmt.Sprintf("guess-%02d", i), ClientIP: limitedAddress, CreatedAt: old}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := svc.StartLogin(ctx, "owner", "valid-password", limitedAddress)
	if err != nil || result.Token == "" {
		t.Fatalf("login after the window = %v, %v; want a session", result, err)
	}
}

func TestReqAuth011_AC03(t *testing.T) {
	ctx := context.Background()
	_, svc := loginLimitService(t)
	failFromAddress(t, svc, limitedAddress, addressFailureLimit)

	result, err := svc.StartLogin(ctx, "owner", "valid-password", otherAddress)
	if err != nil || result.Token == "" {
		t.Fatalf("login from another address = %v, %v; want a session", result, err)
	}
	if _, err := svc.StartLogin(ctx, "owner", "valid-password", limitedAddress); !errors.Is(err, ErrLoginRateLimited) {
		t.Fatalf("limited address err = %v, want rate limited", err)
	}
}

// Successful logins do not count toward the per-address limit
// (owner 2026-10-23, SC-043).
func TestReqAuth011_SuccessfulLoginsDoNotCount(t *testing.T) {
	ctx := context.Background()
	_, svc := loginLimitService(t)
	for i := 0; i <= addressFailureLimit; i++ {
		if _, err := svc.StartLogin(ctx, "owner", "valid-password", limitedAddress); err != nil {
			t.Fatalf("successful login %d: %v", i+1, err)
		}
	}
}

func TestReqAuth011_AC04(t *testing.T) {
	ctx := context.Background()
	bdb, svc := loginLimitService(t)
	if _, err := bdb.DB.ExecContext(ctx, "ALTER TABLE login_attempts RENAME TO login_attempts_broken"); err != nil {
		t.Fatal(err)
	}
	result, err := svc.StartLogin(ctx, "owner", "valid-password", limitedAddress)
	if err == nil || result != nil {
		t.Fatalf("login with a failing count = %v, %v; want an error", result, err)
	}
	if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrLoginRateLimited) {
		t.Fatalf("err = %v, want an internal error, not a login outcome", err)
	}
	var sessions int
	if err := bdb.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("sessions = %d, error %v; want none", sessions, err)
	}
}
