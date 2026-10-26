package httpapi_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	v1 "raid-website/internal/httpapi/v1"
	"raid-website/internal/validation"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) events(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("journal line is not JSON: %q", line)
		}
		out = append(out, e)
	}
	return out
}

func captureSecurityLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prev := v1.SetSecurityLogger(slog.New(slog.NewJSONHandler(buf, nil)))
	t.Cleanup(func() { v1.SetSecurityLogger(prev) })
	return buf
}

func eventNames(events []map[string]any) []string {
	names := make([]string, 0, len(events))
	for _, e := range events {
		names = append(names, e["event"].(string))
	}
	return names
}

// TestReqAuth024_SecurityEventsWithoutSecrets covers SC-041: failed login,
// login block, CSRF rejection, password change and Admin account change each
// write one JSON line with time, client address and request ID, and no secret.
func TestReqAuth024_SecurityEventsWithoutSecrets(t *testing.T) {
	buf := captureSecurityLog(t)
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "member", "member-password-1", validation.RoleMember)
	otherID := seedAccount(t, bdb, "other", "other-password-1", validation.RoleMember)

	for i := 0; i < 5; i++ {
		resp := postJSON(t, client, srv.URL+"/api/v1/auth/login", map[string]any{"username": " Member ", "password": "wrong-secret-pw"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("failed login = %d, want 401", resp.StatusCode)
		}
	}
	resp := postJSON(t, client, srv.URL+"/api/v1/auth/login", map[string]any{"username": "member", "password": "member-password-1"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("blocked login = %d, want 429", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login", strings.NewReader(`{}`))
	csrf, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	csrf.Body.Close()
	if csrf.StatusCode != http.StatusForbidden {
		t.Fatalf("login without CSRF = %d, want 403", csrf.StatusCode)
	}

	// The member is blocked by the limit above, so the password change uses a second account.
	otherToken := login(t, client, srv, "other", "other-password-1")
	body, _ := json.Marshal(map[string]any{"current_password": "other-password-1", "new_password": "brand-new-secret-9"})
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "raid_session="+otherToken)
	addCSRF(req)
	changed, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	changed.Body.Close()
	if changed.StatusCode != http.StatusNoContent {
		t.Fatalf("password change = %d, want 204", changed.StatusCode)
	}

	events := buf.events(t)
	want := []string{"login_failed", "login_failed", "login_failed", "login_failed", "login_failed", "login_blocked", "csrf_rejected", "password_changed"}
	if got := eventNames(events); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", got, want)
	}
	first := events[0]
	if first["username"] != "member" || first["client_address"] == "" || first["time"] == nil {
		t.Fatalf("login_failed line misses username, address or time: %v", first)
	}
	if id, _ := first["request_id"].(string); id == "" {
		t.Fatalf("login_failed line has no request ID: %v", first)
	}
	if last := events[len(events)-1]; last["player_id"] != float64(otherID) {
		t.Fatalf("password_changed line has player_id %v, want %d", last["player_id"], otherID)
	}
	raw, _ := json.Marshal(events)
	for _, secret := range []string{"wrong-secret-pw", "member-password-1", "brand-new-secret-9", otherToken} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("journal lines contain a secret: %s", secret)
		}
	}
}

// TestReqAuth024_WrongMFACodeNamesThePlayer checks that a wrong code and an
// invalidated challenge carry the player ID of the challenge, and that the
// code itself never reaches the journal (SC-041, SC-030).
func TestReqAuth024_WrongMFACodeNamesThePlayer(t *testing.T) {
	buf := captureSecurityLog(t)
	srv, bdb := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, bdb, "admin", "admin-password-1", validation.RoleAdmin)

	resp := postJSON(t, client, srv.URL+"/api/v1/auth/login", map[string]any{"username": "admin", "password": "admin-password-1"})
	var started struct {
		Data struct {
			Challenge string `json:"challenge"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&started)
	resp.Body.Close()
	if started.Data.Challenge == "" {
		t.Fatal("login did not start an MFA challenge")
	}

	const wrongCode = "135790"
	for i := 0; i < 5; i++ {
		verified := postJSON(t, client, srv.URL+"/api/v1/auth/mfa/verify", map[string]any{"challenge": started.Data.Challenge, "code": wrongCode})
		verified.Body.Close()
	}
	// The challenge is gone now: no player is known for it.
	gone := postJSON(t, client, srv.URL+"/api/v1/auth/mfa/verify", map[string]any{"challenge": started.Data.Challenge, "code": wrongCode})
	gone.Body.Close()

	events := buf.events(t)
	want := []string{"mfa_failed", "mfa_failed", "mfa_failed", "mfa_failed", "mfa_challenge_invalid", "mfa_challenge_invalid"}
	if got := eventNames(events); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i, e := range events[:5] {
		if e["player_id"] != float64(adminID) {
			t.Fatalf("event %d (%v) has player_id %v, want %d", i, e["event"], e["player_id"], adminID)
		}
	}
	if _, has := events[5]["player_id"]; has {
		t.Fatalf("an unknown challenge must log no player: %v", events[5])
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), wrongCode) || strings.Contains(string(raw), started.Data.Challenge) {
		t.Fatal("journal lines contain the MFA code or the challenge")
	}
}
