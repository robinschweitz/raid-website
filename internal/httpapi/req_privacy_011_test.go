package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/auth"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

func anonymiseURL(base string, id int64) string {
	return fmt.Sprintf("%s/api/v1/players/%d/anonymise", base, id)
}

func countRows(t *testing.T, bdb *bun.DB, table, column string, id int64) int {
	t.Helper()
	var count int
	if err := bdb.NewSelect().TableExpr(table).ColumnExpr("COUNT(*)").Where(column+" = ?", id).Scan(context.Background(), &count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func seedPrivacyChar(t *testing.T, bdb *bun.DB, playerID int64, name string, isMain bool, createdAt time.Time) *character.Character {
	t.Helper()
	c := &character.Character{PlayerID: playerID, Name: name, Class: "Magier", MainSpec: "Feuer", IsMain: isMain, Active: true,
		ExternalSource: "rising-gods", ExternalProfileID: "profile-" + name, ExternalProfileURL: "https://example.invalid/" + name, ExternalProfileJSON: `{"raw":true}`,
		CreatedAt: createdAt, UpdatedAt: createdAt}
	if err := character.NewRepo(bdb).Create(context.Background(), c); err != nil {
		t.Fatalf("create character: %v", err)
	}
	return c
}

// AC-01: credentials and profile data are gone, the name is the oldest main, history stays.
func TestReqPrivacy011_AC01(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	adminID := seedAccount(t, bdb, "admin-priv", "admin-priv-password", validation.RoleAdmin)
	adminToken := login(t, client, srv, "admin-priv", "admin-priv-password")
	targetID := seedAccount(t, bdb, "former-member", "former-member-password", validation.RoleMember)
	targetToken := login(t, client, srv, "former-member", "former-member-password")
	now := time.Now().UTC()

	seedPrivacyChar(t, bdb, targetID, "Altmain", false, now.Add(-72*time.Hour))
	oldestMain := seedPrivacyChar(t, bdb, targetID, "Erstmain", true, now.Add(-48*time.Hour))
	seedPrivacyChar(t, bdb, targetID, "Zweitmain", true, now.Add(-24*time.Hour))
	raidID := mergeTestRaid(t, bdb, adminID, now)
	if _, err := bdb.ExecContext(ctx, "INSERT INTO raid_attendance (raid_id, player_id, character_id, role, created_at, updated_at) VALUES (?, ?, ?, 'DD', ?, ?)", raidID, targetID, oldestMain.ID, now, now); err != nil {
		t.Fatalf("seed attendance: %v", err)
	}
	if err := enhancepoints.NewRepo(bdb).InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: targetID, Amount: 20, Type: "ADJUSTMENT", Reason: "seed", ActorID: adminID, CreatedAt: now}); err != nil {
		t.Fatalf("seed points: %v", err)
	}
	authRepo := auth.NewRepo(bdb)
	if err := authRepo.UpsertMFASettings(ctx, &auth.MFASettings{PlayerID: targetID, TOTPSecret: "secret", TOTPEnabledAt: &now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed mfa: %v", err)
	}
	if err := authRepo.CreateRecoveryCode(ctx, &auth.MFARecoveryCode{PlayerID: targetID, CodeHash: "recovery-priv", CreatedAt: now}); err != nil {
		t.Fatalf("seed recovery: %v", err)
	}
	if _, err := bdb.ExecContext(ctx, "INSERT INTO webauthn_credentials (player_id, credential_id_hash, credential_json, created_at, updated_at) VALUES (?, 'cred-hash', '{}', ?, ?)", targetID, now, now); err != nil {
		t.Fatalf("seed webauthn credential: %v", err)
	}
	if _, err := bdb.ExecContext(ctx, "INSERT INTO login_attempts (username, client_ip, success, created_at) VALUES ('former-member', '1.2.3.4', 0, ?)", now); err != nil {
		t.Fatalf("seed login attempt: %v", err)
	}
	if _, err := bdb.ExecContext(ctx, "INSERT INTO registration_requests (requester_player_id, status, purpose, external_source, external_profile_id, external_profile_url, character_name, class, main_spec, raw_profile_json, created_at, updated_at) VALUES (?, 'CONSUMED', 'NEW_ACCOUNT', 'rising-gods', 'rg-1', 'https://example.invalid/rg-1', 'Erstmain', 'Magier', 'Feuer', '{\"raw\":1}', ?, ?)", targetID, now, now); err != nil {
		t.Fatalf("seed request: %v", err)
	}

	status, body := do(t, client, http.MethodPost, anonymiseURL(srv.URL, targetID), adminToken, map[string]any{"reason": "DSGVO-Anfrage"})
	if status != http.StatusNoContent {
		t.Fatalf("anonymise status %d: %s", status, body)
	}

	got, err := player.NewRepo(bdb).Get(ctx, targetID)
	if err != nil {
		t.Fatalf("player row must stay: %v", err)
	}
	if got.Name != "Erstmain" || got.Username != nil || got.PasswordHash != "" || got.Role != validation.RoleGuest {
		t.Fatalf("player after anonymise = %+v", got)
	}
	for _, table := range []string{"sessions", "player_mfa_settings", "mfa_recovery_codes", "webauthn_credentials", "push_subscriptions", "mfa_login_challenges", "webauthn_sessions"} {
		if n := countRows(t, bdb, table, "player_id", targetID); n != 0 {
			t.Fatalf("%s rows = %d", table, n)
		}
	}
	var attempts int
	if err := bdb.NewSelect().TableExpr("login_attempts").ColumnExpr("COUNT(*)").Where("username = 'former-member'").Scan(ctx, &attempts); err != nil || attempts != 0 {
		t.Fatalf("login attempts = %d, %v", attempts, err)
	}
	var profile string
	if err := bdb.NewSelect().TableExpr("registration_requests").ColumnExpr("raw_profile_json || external_profile_url || character_name").Where("requester_player_id = ?", targetID).Scan(ctx, &profile); err != nil || profile != "" {
		t.Fatalf("request profile data = %q, %v", profile, err)
	}
	chars, err := character.NewRepo(bdb).ListByPlayer(ctx, targetID)
	if err != nil || len(chars) != 3 {
		t.Fatalf("characters = %d, %v", len(chars), err)
	}
	for _, c := range chars {
		if c.ExternalSource != "" || c.ExternalProfileID != "" || c.ExternalProfileURL != "" || c.ExternalProfileJSON != "" {
			t.Fatalf("character %s keeps profile data: %+v", c.Name, c)
		}
	}
	if n := countRows(t, bdb, "raid_attendance", "player_id", targetID); n != 1 {
		t.Fatalf("attendance rows = %d", n)
	}
	if balance, err := enhancepoints.NewRepo(bdb).Balance(ctx, targetID); err != nil || balance != 20 {
		t.Fatalf("balance = %d, %v", balance, err)
	}
	if status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", targetToken, nil); status != http.StatusUnauthorized {
		t.Fatalf("old session status = %d", status)
	}
	resp := postJSON(t, client, srv.URL+"/api/v1/auth/login", map[string]any{"username": "former-member", "password": "former-member-password"})
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("anonymised player can still log in")
	}

	// The audit entry holds no secret and no name; the display hides the old name.
	var entry struct{ Before, After string }
	if err := bdb.NewSelect().TableExpr("audit_log").ColumnExpr("before_json AS before, after_json AS after").Where("action = 'player_anonymise'").Where("entity_id = ?", fmt.Sprint(targetID)).Scan(ctx, &entry); err != nil {
		t.Fatalf("audit entry: %v", err)
	}
	if text := strings.ToLower(entry.Before + entry.After); strings.Contains(text, "former-member") || strings.Contains(text, "password") || strings.Contains(text, "secret") {
		t.Fatalf("audit snapshots leak data: %s", text)
	}
	status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/audit-log", adminToken, nil)
	if status != http.StatusOK || strings.Contains(string(body), "former-member") {
		t.Fatalf("audit log status %d, shows login: %v", status, strings.Contains(string(body), "former-member"))
	}
}

// AC-02: a player without any character gets the placeholder name.
func TestReqPrivacy011_AC02(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin-priv2", "admin-priv-password", validation.RoleAdmin)
	adminToken := login(t, client, srv, "admin-priv2", "admin-priv-password")
	targetID := seedAccount(t, bdb, "nochars", "nochars-password-1", validation.RoleMember)
	status, body := do(t, client, http.MethodPost, anonymiseURL(srv.URL, targetID), adminToken, map[string]any{})
	if status != http.StatusNoContent {
		t.Fatalf("status %d: %s", status, body)
	}
	got, err := player.NewRepo(bdb).Get(context.Background(), targetID)
	if err != nil || got.Name != fmt.Sprintf("Ehemaliges Mitglied #%d", targetID) {
		t.Fatalf("name = %+v, %v", got, err)
	}
	if status, _ := do(t, client, http.MethodPost, anonymiseURL(srv.URL, targetID), adminToken, map[string]any{}); status != http.StatusConflict {
		t.Fatalf("second anonymise status = %d", status)
	}
}

// Without a main character the oldest character gives the name.
func TestReqPrivacy011_OldestCharacterFallback(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin-priv3", "admin-priv-password", validation.RoleAdmin)
	adminToken := login(t, client, srv, "admin-priv3", "admin-priv-password")
	targetID := seedAccount(t, bdb, "twinks", "twinks-password-12", validation.RoleMember)
	now := time.Now().UTC()
	seedPrivacyChar(t, bdb, targetID, "Neuer", false, now)
	seedPrivacyChar(t, bdb, targetID, "Aelter", false, now.Add(-time.Hour))
	if status, body := do(t, client, http.MethodPost, anonymiseURL(srv.URL, targetID), adminToken, map[string]any{}); status != http.StatusNoContent {
		t.Fatalf("status %d: %s", status, body)
	}
	if got, err := player.NewRepo(bdb).Get(context.Background(), targetID); err != nil || got.Name != "Aelter" {
		t.Fatalf("name = %+v, %v", got, err)
	}
}

// AC-03: the only active Admin cannot be anonymised.
func TestReqPrivacy011_AC03(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, bdb, "admin-priv4", "admin-priv-password", validation.RoleAdmin)
	adminToken := login(t, client, srv, "admin-priv4", "admin-priv-password")
	status, body := do(t, client, http.MethodPost, anonymiseURL(srv.URL, adminID), adminToken, map[string]any{})
	if status != http.StatusConflict || errorCode(t, body) != "last_admin" {
		t.Fatalf("status %d body %s", status, body)
	}
	got, err := player.NewRepo(bdb).Get(context.Background(), adminID)
	if err != nil || got.Username == nil || got.Role != validation.RoleAdmin {
		t.Fatalf("admin changed: %+v, %v", got, err)
	}
	if status, _ := do(t, client, http.MethodPost, anonymiseURL(srv.URL, 99999), adminToken, map[string]any{}); status != http.StatusNotFound {
		t.Fatalf("unknown player status = %d", status)
	}
}

// AC-04: roles below ADMIN get 403, no session gets 401, and nothing changes.
func TestReqPrivacy011_AC04(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin-priv5", "admin-priv-password", validation.RoleAdmin)
	seedAccount(t, bdb, "leader-priv", "leader-priv-password", validation.RoleRaidleader)
	leaderToken := login(t, client, srv, "leader-priv", "leader-priv-password")
	targetID := seedAccount(t, bdb, "victim", "victim-password-12", validation.RoleMember)
	if status, _ := do(t, client, http.MethodPost, anonymiseURL(srv.URL, targetID), leaderToken, map[string]any{}); status != http.StatusForbidden {
		t.Fatalf("raidleader status = %d", status)
	}
	if status, _ := do(t, client, http.MethodPost, anonymiseURL(srv.URL, targetID), "", map[string]any{}); status != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", status)
	}
	got, err := player.NewRepo(bdb).Get(context.Background(), targetID)
	if err != nil || got.Username == nil {
		t.Fatalf("player changed: %+v, %v", got, err)
	}
}
