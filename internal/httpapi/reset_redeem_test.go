package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"
	"raid-website/internal/auth"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

func resetTestCode(t *testing.T, db *bun.DB, actor, target int64) string {
	t.Helper()
	service := registration.NewService(db, nil, nil, nil, nil, nil)
	code, err := service.CreateResetCode(context.Background(), actor, target)
	if err != nil {
		t.Fatal(err)
	}
	return code.Code
}

func TestReqAccount018_AC01(t *testing.T) {
	srv, db := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, db, "reset-admin-18", "reset-admin-password", validation.RoleAdmin)
	targetID := seedAccount(t, db, "reset-member-18", "reset-member-password", validation.RoleMember)
	oldToken := login(t, client, srv, "reset-member-18", "reset-member-password")
	enableTestTOTP(t, db, targetID)
	if err := auth.NewRepo(db).CreateRecoveryCode(context.Background(), &auth.MFARecoveryCode{PlayerID: targetID, CodeHash: "recovery-secret", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	code := resetTestCode(t, db, adminID, targetID)
	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/reset-password", "", map[string]any{"code": code, "password": "a-new-password"})
	if status != 204 {
		t.Fatalf("redeem=%d %s", status, body)
	}
	if status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", oldToken, nil); status != 401 {
		t.Fatalf("old session=%d", status)
	}
	login(t, client, srv, "reset-member-18", "a-new-password")
	for _, table := range []string{"player_mfa_settings", "mfa_recovery_codes", "webauthn_credentials", "mfa_login_challenges", "webauthn_sessions"} {
		if countRows(t, db, table, "player_id", targetID) != 0 {
			t.Errorf("MFA survived: %s", table)
		}
	}
	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/reset-password", "", map[string]any{"code": code, "password": "another-password"})
	if status != 400 || !strings.Contains(string(body), "reset_code_invalid") {
		t.Fatalf("AC02 replay=%d %s", status, body)
	}
	code = resetTestCode(t, db, adminID, targetID)
	if _, err := db.ExecContext(context.Background(), "UPDATE password_reset_codes SET expires_at = ? WHERE used_at IS NULL", time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/reset-password", "", map[string]any{"code": code, "password": "another-password"})
	if status != 400 || !strings.Contains(string(body), "reset_code_invalid") {
		t.Fatalf("expired=%d %s", status, body)
	}
}

func TestReqAccount018_AC03(t *testing.T) {
	srv, _ := setupServer(t)
	for i := 0; i < 30; i++ {
		path := srv.URL + "/api/v1/auth/reset-password"
		body := map[string]any{"code": "invalid", "password": "valid-password"}
		if i%2 == 0 {
			path = srv.URL + "/api/v1/auth/register-with-code"
			body["username"] = "unknown"
		}
		status, result := do(t, srv.Client(), http.MethodPost, path, "", body)
		if status != 400 {
			t.Fatalf("failure %d=%d %s", i, status, result)
		}
	}
	status, body := do(t, srv.Client(), http.MethodPost, srv.URL+"/api/v1/auth/reset-password", "", map[string]any{"code": "invalid", "password": "valid-password"})
	if status != 429 || !strings.Contains(string(body), "login_rate_limited") {
		t.Fatalf("31st=%d %s", status, body)
	}
}

func TestReqAccount018_AC04(t *testing.T) {
	srv, db := setupServer(t)
	id := seedAccount(t, db, "reset-only-admin", "reset-only-admin-password", validation.RoleAdmin)
	code := resetTestCode(t, db, id, id)
	status, body := do(t, srv.Client(), http.MethodPost, srv.URL+"/api/v1/auth/reset-password", "", map[string]any{"code": code, "password": "new-admin-password"})
	if status != 204 {
		t.Fatalf("reset=%d %s", status, body)
	}
	// Password-only login succeeds, but Admin functions stay gated by MFA setup.
	token := login(t, srv.Client(), srv, "reset-only-admin", "new-admin-password")
	status, body = do(t, srv.Client(), http.MethodGet, srv.URL+"/api/v1/audit-log", token, nil)
	if status != 403 || !strings.Contains(string(body), "mfa_setup_required") {
		t.Fatalf("Admin gate=%d %s", status, body)
	}
}
func TestReqAccount018_AC05(t *testing.T) {
	srv, db := setupServer(t)
	admin := seedAccount(t, db, "reset-fail-admin", "reset-admin-password", validation.RoleAdmin)
	id := seedAccount(t, db, "reset-fail-user", "reset-member-password", validation.RoleMember)
	enableTestTOTP(t, db, id)
	code := resetTestCode(t, db, admin, id)
	if _, err := db.ExecContext(context.Background(), "CREATE TRIGGER reject_reset_mfa BEFORE DELETE ON player_mfa_settings BEGIN SELECT RAISE(ABORT,'failure'); END"); err != nil {
		t.Fatal(err)
	}
	status, _ := do(t, srv.Client(), http.MethodPost, srv.URL+"/api/v1/auth/reset-password", "", map[string]any{"code": code, "password": "a-new-password"})
	if status != 500 {
		t.Fatalf("failed reset=%d", status)
	}
	var used int
	if err := db.NewSelect().Model((*registration.ResetCode)(nil)).ColumnExpr("count(*)").Where("used_at IS NOT NULL").Scan(context.Background(), &used); err != nil || used != 0 {
		t.Fatalf("code consumed=%d %v", used, err)
	}
	if countRows(t, db, "player_mfa_settings", "player_id", id) != 1 {
		t.Fatal("MFA was removed")
	}
	status, body := do(t, srv.Client(), http.MethodPost, srv.URL+"/api/v1/auth/login", "", map[string]any{"username": "reset-fail-user", "password": "reset-member-password"})
	var result map[string]any
	_ = json.Unmarshal(body, &result)
	if status != 200 {
		t.Fatalf("old password lost=%d %s (%s)", status, body, fmt.Sprint(result))
	}
}
