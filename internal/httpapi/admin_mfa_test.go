package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/auth"
	"raid-website/internal/httpapi"
	"raid-website/internal/validation"
)

// Admin-only routes need a second factor (REQ-AUTH-020, FR-3.10, SC-040).

func removeMFA(t *testing.T, bdb *bun.DB, playerID int64) {
	t.Helper()
	if _, err := bdb.NewDelete().Table("player_mfa_settings").Where("player_id = ?", playerID).Exec(context.Background()); err != nil {
		t.Fatalf("remove MFA: %v", err)
	}
}

func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode error body %s: %v", body, err)
	}
	return payload.Error.Code
}

func TestReqAuth020_AC01_AdminWithoutMFAGetsMFASetupRequired(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, bdb, "admin", "admin-password", validation.RoleAdmin)
	removeMFA(t, bdb, adminID)
	token := login(t, client, srv, "admin", "admin-password")
	// Recovery codes alone are not a second factor (FR-3.12).
	if err := auth.NewRepo(bdb).CreateRecoveryCode(context.Background(), &auth.MFARecoveryCode{PlayerID: adminID, CodeHash: "unused", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("seed recovery code: %v", err)
	}

	status, body := do(t, client, http.MethodGet, srv.URL+"/api/v1/audit-log", token, nil)
	if status != http.StatusForbidden || errorCode(t, body) != "mfa_setup_required" {
		t.Fatalf("audit log without MFA: status %d body %s", status, body)
	}
	// A write behind the Admin guard is rejected the same way and changes nothing.
	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/players", token, map[string]any{"name": "Neu"})
	if status != http.StatusForbidden || errorCode(t, body) != "mfa_setup_required" {
		t.Fatalf("create player without MFA: status %d body %s", status, body)
	}
	var count int
	if err := bdb.NewSelect().Table("players").ColumnExpr("count(*)").Where("name = ?", "Neu").Scan(context.Background(), &count); err != nil || count != 0 {
		t.Fatalf("player created without MFA: count %d err %v", count, err)
	}
}

func TestReqAuth020_AC02_AdminWithMFAPasses(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, bdb, "admin", "admin-password", validation.RoleAdmin)
	removeMFA(t, bdb, adminID)
	token := login(t, client, srv, "admin", "admin-password")

	// MFA setup is reachable without MFA (AC-05), then the Admin enables TOTP.
	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/mfa/totp/setup", token, map[string]any{"current_password": "admin-password"})
	if status != http.StatusOK {
		t.Fatalf("TOTP setup: status %d body %s", status, body)
	}
	var setup struct {
		Data struct {
			Secret string `json:"secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &setup); err != nil || setup.Data.Secret == "" {
		t.Fatalf("TOTP setup body %s: %v", body, err)
	}
	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/mfa/totp/enable", token, map[string]any{"code": testTOTPCode(t, setup.Data.Secret)})
	if status != http.StatusOK {
		t.Fatalf("TOTP enable: status %d body %s", status, body)
	}

	status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/audit-log", token, nil)
	if status != http.StatusOK {
		t.Fatalf("audit log with MFA: status %d body %s", status, body)
	}
}

func TestReqAuth020_AC03_PromotedAdminNeedsMFA(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin", "admin-password", validation.RoleAdmin)
	memberID := seedAccount(t, bdb, "member", "member-password", validation.RoleMember)
	adminToken := login(t, client, srv, "admin", "admin-password")

	status, body := do(t, client, http.MethodPut, srv.URL+"/api/v1/players/"+strconv.FormatInt(memberID, 10)+"/account", adminToken, map[string]any{"username": "member", "role": "ADMIN"})
	if status != http.StatusOK {
		t.Fatalf("promote: status %d body %s", status, body)
	}
	promotedToken := login(t, client, srv, "member", "member-password")
	status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/audit-log", promotedToken, nil)
	if status != http.StatusForbidden || errorCode(t, body) != "mfa_setup_required" {
		t.Fatalf("promoted Admin without MFA: status %d body %s", status, body)
	}
}

func TestReqAuth020_AC04_BootstrapAdminNeedsMFA(t *testing.T) {
	srv, _ := setupServerWithOptions(t, httpapi.Options{AdminUsername: "owner", AdminPassword: "owner-password-123"})
	client := srv.Client()
	token := login(t, client, srv, "owner", "owner-password-123")
	status, body := do(t, client, http.MethodGet, srv.URL+"/api/v1/audit-log", token, nil)
	if status != http.StatusForbidden || errorCode(t, body) != "mfa_setup_required" {
		t.Fatalf("bootstrap Admin without MFA: status %d body %s", status, body)
	}
}

func TestReqAuth020_AC05_AdminWithoutMFAKeepsLowerRoleAccess(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, bdb, "admin", "admin-password", validation.RoleAdmin)
	removeMFA(t, bdb, adminID)
	token := login(t, client, srv, "admin", "admin-password")

	for _, path := range []string{"/api/v1/auth/mfa", "/api/v1/auth/me"} {
		status, body := do(t, client, http.MethodGet, srv.URL+path, token, nil)
		if status != http.StatusOK {
			t.Fatalf("%s without MFA: status %d body %s", path, status, body)
		}
	}
	// Only Admin-only routes are gated (SC-040); staff routes still work.
	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", token, map[string]any{"name": "Probe", "scheduled_at": "2030-01-01T19:00:00Z"})
	if status == http.StatusForbidden && strings.Contains(string(body), "mfa_setup_required") {
		t.Fatalf("staff route gated by Admin MFA: %s", body)
	}
}

// TestReqAuth020_AC07_AdminWithoutMFASeesNoAccountFields: an Admin without a
// second factor cannot read other players' login names or roles through the
// shared player read; with MFA the fields are back (SC-040).
func TestReqAuth020_AC07_AdminWithoutMFASeesNoAccountFields(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, bdb, "admin", "admin-password", validation.RoleAdmin)
	memberID := seedAccount(t, bdb, "member", "member-password", validation.RoleMember)
	token := login(t, client, srv, "admin", "admin-password")
	memberURL := srv.URL + "/api/v1/players/" + strconv.FormatInt(memberID, 10)

	readAccount := func() (username, role any) {
		t.Helper()
		status, body := do(t, client, http.MethodGet, memberURL, token, nil)
		if status != http.StatusOK {
			t.Fatalf("get player: status %d body %s", status, body)
		}
		var payload struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Data["username"], payload.Data["role"]
	}
	if username, role := readAccount(); username != "member" || role != string(validation.RoleMember) {
		t.Fatalf("with MFA: username %v role %v", username, role)
	}
	removeMFA(t, bdb, adminID)
	if username, role := readAccount(); username != nil || role != nil {
		t.Fatalf("without MFA: username %v role %v, want both hidden", username, role)
	}
	status, body := do(t, client, http.MethodGet, srv.URL+"/api/v1/players/"+strconv.FormatInt(adminID, 10), token, nil)
	if status != http.StatusOK || !strings.Contains(string(body), `"username":"admin"`) {
		t.Fatalf("own account without MFA: status %d body %s", status, body)
	}
}
