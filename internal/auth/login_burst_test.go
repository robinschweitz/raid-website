package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// parallelLogins starts count logins at once from one address and returns how
// many reached the password check and how many were rate limited.
func parallelLogins(t *testing.T, svc *Service, count int, username func(int) string) (checked, limited int) {
	t.Helper()
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.StartLogin(context.Background(), username(i), "wrong-password", limitedAddress)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case errors.Is(err, ErrInvalidCredentials):
				checked++
			case errors.Is(err, ErrLoginRateLimited):
				limited++
			default:
				t.Errorf("attempt %d: unexpected error %v", i, err)
			}
		}()
	}
	wg.Wait()
	return checked, limited
}

// Parallel wrong passwords for one account from one address reach the
// password check at most loginFailureLimit times (SC-008): the attempt is
// counted before the request waits for a hashing slot.
func TestLoginLimitHoldsForParallelAttempts(t *testing.T) {
	_, svc := loginLimitService(t)
	checked, limited := parallelLogins(t, svc, 12, func(int) string { return "owner" })
	if checked != loginFailureLimit || limited != 12-loginFailureLimit {
		t.Fatalf("checked %d, limited %d; want %d checked and the rest limited", checked, limited, loginFailureLimit)
	}
}

// The per-address limit (SC-043) also holds for a parallel burst over many usernames.
func TestAddressLimitHoldsForParallelAttempts(t *testing.T) {
	_, svc := loginLimitService(t)
	const burst = addressFailureLimit + 6
	checked, limited := parallelLogins(t, svc, burst, func(i int) string { return fmt.Sprintf("guess-%02d", i) })
	if checked != addressFailureLimit || limited != burst-addressFailureLimit {
		t.Fatalf("checked %d, limited %d; want %d checked and the rest limited", checked, limited, addressFailureLimit)
	}
}

// occupyArgonSlots holds every hashing slot and shortens the wait, so the next
// hash gives up at once; both are restored when the test ends.
func occupyArgonSlots(t *testing.T) {
	t.Helper()
	for i := 0; i < cap(argonSlots); i++ {
		argonSlots <- struct{}{}
	}
	originalWait := argonSlotWait
	argonSlotWait = 10 * time.Millisecond
	t.Cleanup(func() {
		argonSlotWait = originalWait
		for i := 0; i < cap(argonSlots); i++ {
			<-argonSlots
		}
	})
}

// With every hashing slot busy, a login answers ErrPasswordHashBusy instead of
// queueing, and it does not count as a failed attempt (SC-005).
func TestLoginBusyWithoutCountingAsFailure(t *testing.T) {
	ctx := context.Background()
	_, svc := loginLimitService(t)
	occupyArgonSlots(t)

	for _, username := range []string{"owner", "nobody"} {
		if _, err := svc.StartLogin(ctx, username, "wrong-password", limitedAddress); !errors.Is(err, ErrPasswordHashBusy) {
			t.Fatalf("%s: err = %v, want ErrPasswordHashBusy", username, err)
		}
	}
	failures, err := svc.sessions.CountFailedLoginAttemptsFromAddress(ctx, limitedAddress, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if failures != 0 {
		t.Fatalf("busy logins stored %d failed attempts, want 0", failures)
	}
	if err := svc.VerifyPassword(ctx, 1, "valid-password"); !errors.Is(err, ErrPasswordHashBusy) {
		t.Fatalf("VerifyPassword err = %v, want ErrPasswordHashBusy", err)
	}
	if _, err := HashPassword("valid-password"); !errors.Is(err, ErrPasswordHashBusy) {
		t.Fatalf("HashPassword err = %v, want ErrPasswordHashBusy", err)
	}
}

// A successful login after failures marks its own attempt as success, so the
// failures before it still count but the login itself does not.
func TestSuccessfulLoginIsNotCountedAsFailure(t *testing.T) {
	ctx := context.Background()
	_, svc := loginLimitService(t)
	if _, err := svc.StartLogin(ctx, "owner", "valid-password", limitedAddress); err != nil {
		t.Fatal(err)
	}
	failures, err := svc.sessions.CountFailedLoginAttempts(ctx, "owner", limitedAddress, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if failures != 0 {
		t.Fatalf("successful login left %d failed attempts, want 0", failures)
	}
}
