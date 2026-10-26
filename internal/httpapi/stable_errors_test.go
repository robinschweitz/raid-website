package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"raid-website/internal/validation"
)

func TestReqArch013_AC01(t *testing.T) {
	srv, db := setupServer(t)
	client := srv.Client()
	seedAccount(t, db, "admin-113", "admin-113-password", validation.RoleAdmin)
	id := seedAccount(t, db, "member-113", "member-113-password", validation.RoleMember)
	token := login(t, client, srv, "admin-113", "admin-113-password")
	if _, err := db.DB.ExecContext(context.Background(), "CREATE TRIGGER reject_account BEFORE UPDATE ON players BEGIN SELECT RAISE(ABORT, 'private_constraint_name'); END"); err != nil {
		t.Fatal(err)
	}
	status, body := do(t, client, http.MethodPut, fmt.Sprintf("%s/api/v1/players/%d/account", srv.URL, id), token, map[string]any{"username": "renamed-113", "role": "MEMBER"})
	if status != http.StatusInternalServerError || !strings.Contains(string(body), `"code":"internal"`) || strings.Contains(string(body), "private_constraint_name") {
		t.Fatalf("failure = %d %s", status, body)
	}
}

func TestReqArch013_AC02(t *testing.T) {
	srv, db := setupServer(t)
	client := srv.Client()
	seedAccount(t, db, "admin-113b", "admin-113-password", validation.RoleAdmin)
	id := seedAccount(t, db, "member-113b", "member-113-password", validation.RoleMember)
	token := login(t, client, srv, "admin-113b", "admin-113-password")
	status, body := do(t, client, http.MethodPut, fmt.Sprintf("%s/api/v1/players/%d/account", srv.URL, id), token, map[string]any{"username": "member-113b", "password": "short", "role": "MEMBER"})
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(body), `"code":"password_too_short"`) {
		t.Fatalf("known error = %d %s", status, body)
	}
}
