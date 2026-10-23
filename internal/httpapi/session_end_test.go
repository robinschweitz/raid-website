package httpapi_test

import (
	"fmt"
	"net/http"
	"testing"

	"raid-website/internal/validation"
)

func TestReqAuth017_HTTPOwnPasswordKeepsCurrentDevice(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "member-017", "member-017-password", validation.RoleMember)
	device1 := login(t, client, srv, "member-017", "member-017-password")
	device2 := login(t, client, srv, "member-017", "member-017-password")

	status, _ := do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/password", device1, map[string]any{"current_password": "wrong-password-xx", "new_password": "member-017-replaced"})
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong current password status = %d", status)
	}
	if status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", device2, nil); status != http.StatusOK {
		t.Fatalf("rejected change ended device 2: %d", status)
	}
	status, _ = do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/password", device1, map[string]any{"current_password": "member-017-password", "new_password": "member-017-replaced"})
	if status != http.StatusNoContent {
		t.Fatalf("password change status = %d", status)
	}
	if status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", device1, nil); status != http.StatusOK {
		t.Fatalf("current device status = %d", status)
	}
	if status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", device2, nil); status != http.StatusUnauthorized {
		t.Fatalf("other device status = %d", status)
	}
}

func TestReqAuth017_HTTPAdminAccountChange(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin-017", "admin-017-password", validation.RoleAdmin)
	memberID := seedAccount(t, bdb, "member-017b", "member-017b-password", validation.RoleMember)
	adminToken := login(t, client, srv, "admin-017", "admin-017-password")
	memberToken := login(t, client, srv, "member-017b", "member-017b-password")
	url := srv.URL + "/api/v1/players/" + fmt.Sprint(memberID) + "/account"
	unchanged := map[string]any{"username": "member-017b", "password": "", "role": "MEMBER"}

	if status, _ := do(t, client, http.MethodPut, url, memberToken, unchanged); status != http.StatusForbidden {
		t.Fatalf("member account save status = %d", status)
	}
	if status, _ := do(t, client, http.MethodPut, url, "", unchanged); status != http.StatusUnauthorized {
		t.Fatalf("anonymous account save status = %d", status)
	}
	if status, _ := do(t, client, http.MethodPut, url, adminToken, unchanged); status != http.StatusOK {
		t.Fatalf("unchanged save status = %d", status)
	}
	if status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", memberToken, nil); status != http.StatusOK {
		t.Fatalf("unchanged save ended the session: %d", status)
	}
	changed := map[string]any{"username": "member-017b", "password": "", "role": "RAIDASSISTENT"}
	if status, _ := do(t, client, http.MethodPut, url, adminToken, changed); status != http.StatusOK {
		t.Fatalf("role change status = %d", status)
	}
	if status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", memberToken, nil); status != http.StatusUnauthorized {
		t.Fatalf("session survived role change: %d", status)
	}
	if status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", adminToken, nil); status != http.StatusOK {
		t.Fatalf("admin session ended: %d", status)
	}
}
