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

// waitForArgonSlots lets every login in a burst wait for a hashing slot, so the
// limit tests below do not depend on how fast the machine hashes: a slow build
// host (2 CPUs, other test packages running) otherwise hits argonSlotWait and
// answers ErrPasswordHashBusy. The wait is restored when the test ends.
func waitForArgonSlots(t *testing.T) {
	t.Helper()
	originalWait := argonSlotWait
	argonSlotWait = 5 * time.Minute
	t.Cleanup(func() { argonSlotWait = originalWait })
}

// Parallel wrong passwords for one account from one address reach the
// password check at most loginFailureLimit times (SC-008): the attempt is
// counted before the request waits for a hashing slot.
func TestLoginLimitHoldsForParallelAttempts(t *testing.T) {
	_, svc := loginLimitService(t)
	waitForArgonSlots(t)
	checked, limited := parallelLogins(t, svc, 12, func(int) string { return "owner" })
	if checked != loginFailureLimit || limited != 12-loginFailureLimit {
		t.Fatalf("checked %d, limited %d; want %d checked and the rest limited", checked, limited, loginFailureLimit)
	}
}

// The per-address limit (SC-043) also holds for a parallel burst over many usernames.
func TestAddressLimitHoldsForParallelAttempts(t *testing.T) {
	_, svc := loginLimitService(t)
	waitForArgonSlots(t)
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
	if _, err := HashPassword(context.Background(), "valid-password"); !errors.Is(err, ErrPasswordHashBusy) {
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

// failedAttempts counts the stored failed attempts for one username from limitedAddress.
func failedAttempts(t *testing.T, svc *Service, username string) int {
	t.Helper()
	count, err := svc.sessions.CountFailedLoginAttempts(context.Background(), username, limitedAddress, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return count
}

// The credential decision is final: a correct password whose session cannot be
// created afterwards fails the login but is not counted as a guess (ADR 103).
func TestCorrectPasswordIsNotAGuessWhenSessionFails(t *testing.T) {
	bdb, svc := loginLimitService(t)
	if _, err := bdb.DB.NewRaw("ALTER TABLE sessions RENAME TO sessions_unavailable").Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartLogin(context.Background(), "owner", "valid-password", limitedAddress); err == nil {
		t.Fatal("login without a sessions table succeeded")
	}
	if got := failedAttempts(t, svc, "owner"); got != 0 {
		t.Fatalf("correct password left %d failed attempts, want 0", got)
	}
}

// A request that ends while it waits for a hashing slot checked no password:
// its reservation is released even though its context is already done.
func TestCancelledWaitReleasesTheReservation(t *testing.T) {
	_, svc := loginLimitService(t)
	occupyArgonSlots(t)
	argonSlotWait = time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := svc.StartLogin(ctx, "owner", "wrong-password", limitedAddress)
	if !errors.Is(err, ErrPasswordHashBusy) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrPasswordHashBusy caused by the deadline", err)
	}
	if got := failedAttempts(t, svc, "owner"); got != 0 {
		t.Fatalf("cancelled wait left %d failed attempts, want 0", got)
	}
}

// Parallel right and wrong passwords near the limit: after all of them finish,
// only wrong passwords that were really checked count as failures; right
// passwords that logged in do not.
func TestParallelSuccessesAndFailuresCountOnlyWrongPasswords(t *testing.T) {
	_, svc := loginLimitService(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	wrongChecked, rightOK := 0, 0
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			password := "wrong-password"
			if i%2 == 0 {
				password = "valid-password"
			}
			result, err := svc.StartLogin(context.Background(), "owner", password, limitedAddress)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case errors.Is(err, ErrLoginRateLimited):
			case errors.Is(err, ErrInvalidCredentials):
				wrongChecked++
			case err == nil && result.Token != "":
				rightOK++
			default:
				t.Errorf("attempt %d: %v", i, err)
			}
		}()
	}
	wg.Wait()
	if wrongChecked > loginFailureLimit {
		t.Fatalf("%d wrong passwords reached the check, limit %d", wrongChecked, loginFailureLimit)
	}
	if got := failedAttempts(t, svc, "owner"); got != wrongChecked {
		t.Fatalf("stored failures %d, want the %d checked wrong passwords", got, wrongChecked)
	}
	// Undecided attempts count while they wait, so at most the limit gets through at once.
	if wrongChecked+rightOK > loginFailureLimit {
		t.Fatalf("%d attempts got through at once, limit %d", wrongChecked+rightOK, loginFailureLimit)
	}
}
