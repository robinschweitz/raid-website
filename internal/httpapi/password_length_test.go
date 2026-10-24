package httpapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"raid-website/internal/validation"
)

// REQ-AUTH-023 / SC-005: every endpoint that takes a new password rejects more
// than 1024 bytes with 422 password_too_long and changes nothing.
func TestReqAuth023_AC01_PasswordEndpointsRejectOver1024Bytes(t *testing.T) {
	srv, db := setupServer(t)
	client := srv.Client()
	memberID := seedAccount(t, db, "pw-member", "member-password", validation.RoleMember)
	seedAccount(t, db, "pw-admin", "admin-password", validation.RoleAdmin)
	member := login(t, client, srv, "pw-member", "member-password")
	admin := login(t, client, srv, "pw-admin", "admin-password")
	tooLong := strings.Repeat("a", 1025)

	assertTooLong := func(name string, status int, body []byte) {
		t.Helper()
		if status != http.StatusUnprocessableEntity || !strings.Contains(string(body), `"password_too_long"`) {
			t.Fatalf("%s: status %d body %s, want 422 password_too_long", name, status, body)
		}
	}

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/password", member, map[string]any{"current_password": "member-password", "new_password": tooLong})
	assertTooLong("change own password", status, body)

	status, body = do(t, client, http.MethodPut, fmt.Sprintf("%s/api/v1/players/%d/account", srv.URL, memberID), admin, map[string]any{"username": "pw-member", "password": tooLong, "role": "MEMBER"})
	assertTooLong("admin account save", status, body)

	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/register-with-code", "", map[string]any{"code": "unbekannt", "username": "neuling", "password": tooLong})
	assertTooLong("register with code", status, body)

	// Nothing changed: the old password still logs in after every rejected request.
	login(t, client, srv, "pw-member", "member-password")
}

// REQ-AUTH-023 / SC-005: a 1024-byte password is accepted by the own-password endpoint.
func TestReqAuth023_AC02_1024BytesAccepted(t *testing.T) {
	srv, db := setupServer(t)
	client := srv.Client()
	seedAccount(t, db, "pw-edge", "member-password", validation.RoleMember)
	member := login(t, client, srv, "pw-edge", "member-password")
	// 512 umlauts are 1024 bytes, so the limit must count bytes rather than characters.
	exact := strings.Repeat("ä", 512)

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/password", member, map[string]any{"current_password": "member-password", "new_password": exact})
	if status != http.StatusNoContent {
		t.Fatalf("1024-byte password: status %d body %s, want 204", status, body)
	}
	login(t, client, srv, "pw-edge", exact)
}
