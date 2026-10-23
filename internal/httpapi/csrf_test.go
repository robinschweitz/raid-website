package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"raid-website/internal/validation"
)

// rawPost sends a POST without the helper's CSRF token and without Origin or Sec-Fetch-Site.
func rawPost(t *testing.T, url, session, csrfCookie, csrfHeader, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if session != "" {
		req.AddCookie(&http.Cookie{Name: "raid_session", Value: session})
	}
	if csrfCookie != "" {
		req.AddCookie(&http.Cookie{Name: "raid_csrf", Value: csrfCookie})
	}
	if csrfHeader != "" {
		req.Header.Set("X-CSRF-Token", csrfHeader)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct{ Error struct{ Code string } }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Error.Code
}

func TestReqAuth019_AC01(t *testing.T) {
	srv, db := setupServer(t)
	seedAccount(t, db, "csrf-admin", "password-12345", validation.RoleAdmin)
	session := login(t, srv.Client(), srv, "csrf-admin", "password-12345")
	url := srv.URL + "/api/v1/players"

	cases := []struct{ name, cookie, header string }{
		{"no token", "", ""},
		{"header only", "", "abc"},
		{"cookie only", "abc", ""},
		{"mismatch", "abc", "abd"},
	}
	for _, tc := range cases {
		status, code := rawPost(t, url, session, tc.cookie, tc.header, `{"name":"Intruder"}`)
		if status != http.StatusForbidden || code != "csrf_required" {
			t.Errorf("%s: %d %q, want 403 csrf_required", tc.name, status, code)
		}
	}
	if status, _ := rawPost(t, url, session, "abc", "abc", `{"name":"Allowed"}`); status != http.StatusCreated {
		t.Errorf("matching token: %d, want 201", status)
	}
	// Nothing changed by the rejected requests.
	status, body := do(t, srv.Client(), http.MethodGet, url, session, nil)
	if status != http.StatusOK || bytes.Contains(body, []byte("Intruder")) {
		t.Fatalf("rejected request changed data: %d", status)
	}
}

func TestReqAuth019_AC02(t *testing.T) {
	srv, db := setupServer(t)
	seedAccount(t, db, "csrf-staff", "password-12345", validation.RoleAdmin)
	session := login(t, srv.Client(), srv, "csrf-staff", "password-12345")
	// With a matching token, a write succeeds regardless of Origin/Sec-Fetch-Site.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/players", bytes.NewReader([]byte(`{"name":"Browser"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: "raid_session", Value: session})
	addCSRF(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201", resp.StatusCode)
	}
	// Cross-site metadata does not help without the token.
	if status, code := rawPost(t, srv.URL+"/api/v1/players", session, "", "", `{"name":"X"}`); status != http.StatusForbidden || code != "csrf_required" {
		t.Fatalf("%d %q", status, code)
	}
}

func TestReqAuth019_AC03(t *testing.T) {
	srv, db := setupServer(t)
	seedAccount(t, db, "csrf-login", "password-12345", validation.RoleMember)
	body := `{"username":"csrf-login","password":"password-12345"}`
	for _, path := range []string{"/api/v1/auth/login", "/api/v1/auth/mfa/verify", "/api/v1/auth/registration-requests"} {
		if status, code := rawPost(t, srv.URL+path, "", "", "", body); status != http.StatusForbidden || code != "csrf_required" {
			t.Errorf("%s: %d %q, want 403 csrf_required", path, status, code)
		}
	}
	if status, _ := rawPost(t, srv.URL+"/api/v1/auth/login", "", "t", "t", body); status != http.StatusOK {
		t.Errorf("login with token: %d, want 200", status)
	}
}
