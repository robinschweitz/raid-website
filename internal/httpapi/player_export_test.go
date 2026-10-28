package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/validation"
)

func TestReqPrivacy012_AC01(t *testing.T) {
	srv, db := setupServer(t)
	ctx := context.Background()
	now := time.Now().UTC()
	adminID := seedAccount(t, db, "export-admin", "export-admin-password", validation.RoleAdmin)
	targetID := seedAccount(t, db, "export-member", "export-member-password", validation.RoleMember)
	otherID := seedAccount(t, db, "other-member", "other-member-password", validation.RoleMember)
	token := login(t, srv.Client(), srv, "export-admin", "export-admin-password")
	c := seedPrivacyChar(t, db, targetID, "Exportmage", true, now)
	raidID := mergeTestRaid(t, db, adminID, now)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO raid_signups (raid_id,player_id,character_id,spec,created_at,updated_at) VALUES (?,?,?,'Feuer',?,?)", []any{raidID, targetID, c.ID, now, now}},
		{"INSERT INTO raid_absences (raid_id,player_id,reason,created_at) VALUES (?,?,'absence-note',?)", []any{raidID, targetID, now}},
		{"INSERT INTO raid_attendance (raid_id,player_id,character_id,role,notes,created_at,updated_at) VALUES (?,?,?,'DD','roster-note',?,?)", []any{raidID, targetID, c.ID, now, now}},
		{"INSERT INTO loot_awards (raid_id,boss,item_id,item_name,awarded_to_player_id,awarded_by_player_id,source,notes,awarded_at,created_at,updated_at) VALUES (?,'Boss','item','Item',?,?,'MANUAL','loot-note',?,?,?)", []any{raidID, targetID, adminID, now, now, now}},
		{"INSERT INTO enhance_point_transactions (player_id,amount,type,reason,actor_id,created_at) VALUES (?,12,'ADJUSTMENT','ep-note',?,?)", []any{targetID, adminID, now}},
		{"INSERT INTO item_locks (player_id,raid_id,status,locked_at,actor_id,notes,created_at,updated_at) VALUES (?,?,'EXPIRED',?,?,'lock-note',?,?)", []any{targetID, raidID, now, adminID, now, now}},
		{"INSERT INTO character_wishlist (player_id,character_id,item_id,item_name,priority,notes,created_at,updated_at) VALUES (?,?,'item','Item',1,'wish-note',?,?)", []any{targetID, c.ID, now, now}},
		{"INSERT INTO polls (title,status,created_by,created_at,updated_at) VALUES ('Export poll','OPEN',?,?,?)", []any{adminID, now, now}},
		{"INSERT INTO poll_votes (poll_id,player_id,choice,voted_at) VALUES (1,?,'vote-note',?)", []any{targetID, now}},
		{"INSERT INTO enhance_point_transactions (player_id,amount,type,reason,actor_id,created_at) VALUES (?,99,'ADJUSTMENT','other-private-note',?,?)", []any{otherID, adminID, now}},
	} {
		if _, err := db.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := audit.NewRepo(db).Record(ctx, adminID, "character", c.ID, "edit", nil, map[string]any{"player_id": targetID, "password_hash": "hash-secret", "nested": map[string]any{"totp_secret": "totp-secret", "token_hash": "session-secret", "code_hash": "recovery-secret"}}, ""); err != nil {
		t.Fatal(err)
	}
	status, body := do(t, srv.Client(), http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/export", srv.URL, targetID), token, nil)
	if status != 200 {
		t.Fatalf("export=%d %s", status, body)
	}
	var result struct {
		Data struct {
			Sections map[string][]map[string]any `json:"sections"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{"characters", "signups", "absences", "roster", "loot", "ep_transactions", "item_locks", "wishlist", "votes", "audit"} {
		if len(result.Data.Sections[section]) == 0 {
			t.Errorf("missing populated section %s", section)
		}
	}
	for _, secret := range []string{"password_hash", "hash-secret", "totp-secret", "session-secret", "recovery-secret", "other-private-note", "external_profile_json"} {
		if strings.Contains(string(body), secret) {
			t.Errorf("export contains %q", secret)
		}
	}
	var count int
	if err := db.NewSelect().TableExpr("audit_log").ColumnExpr("count(*)").Where("entity_type = 'player' AND entity_id = ? AND action = 'export' AND actor_id = ?", fmt.Sprint(targetID), adminID).Scan(ctx, &count); err != nil || count != 1 {
		t.Fatalf("export audit=%d %v", count, err)
	}
}

func TestReqPrivacy012_AC02(t *testing.T) {
	srv, db := setupServer(t)
	id := seedAccount(t, db, "export-leader", "export-leader-password", validation.RoleRaidleader)
	token := login(t, srv.Client(), srv, "export-leader", "export-leader-password")
	status, _ := do(t, srv.Client(), http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/export", srv.URL, id), token, nil)
	if status != 403 {
		t.Fatalf("leader export=%d", status)
	}
}
func TestReqPrivacy012_AC03(t *testing.T) {
	srv, db := setupServer(t)
	seedAccount(t, db, "export-admin2", "export-admin-password", validation.RoleAdmin)
	token := login(t, srv.Client(), srv, "export-admin2", "export-admin-password")
	status, _ := do(t, srv.Client(), http.MethodGet, srv.URL+"/api/v1/players/999999/export", token, nil)
	if status != 404 {
		t.Fatalf("missing player=%d", status)
	}
}

func TestReqPrivacy012_AnonymisedIdentityStaysHidden(t *testing.T) {
	srv, db := setupServer(t)
	ctx := context.Background()
	seedAccount(t, db, "privacy-export-admin", "export-admin-password", validation.RoleAdmin)
	targetID := seedAccount(t, db, "erased-login-unique", "export-member-password", validation.RoleMember)
	otherID := seedAccount(t, db, "export-other", "export-other-password", validation.RoleMember)
	token := login(t, srv.Client(), srv, "privacy-export-admin", "export-admin-password")
	seedPrivacyChar(t, db, targetID, "Retainedmage", true, time.Now().UTC())
	if err := audit.NewRepo(db).Record(ctx, otherID, "player", targetID, "update", nil,
		map[string]any{"name": "Erased identity unique", "username": "erased-login-unique", "role": "MEMBER"}, ""); err != nil {
		t.Fatal(err)
	}
	status, body := do(t, srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/anonymise", srv.URL, targetID), token, map[string]any{})
	if status != http.StatusNoContent {
		t.Fatalf("anonymise=%d %s", status, body)
	}
	// Also export the actor: their history includes the erased player's account edit,
	// but need not include that player's anonymisation marker.
	for _, id := range []int64{targetID, otherID} {
		status, body = do(t, srv.Client(), http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/export", srv.URL, id), token, nil)
		if status != http.StatusOK {
			t.Fatalf("export=%d %s", status, body)
		}
		for _, identity := range []string{"Erased identity unique", "erased-login-unique"} {
			if strings.Contains(string(body), identity) {
				t.Errorf("export for %d reveals anonymised identity %q", id, identity)
			}
		}
	}
	var stored string
	if err := db.NewSelect().TableExpr("audit_log").Column("after_json").Where("entity_type = 'player' AND entity_id = ? AND action = 'update'", fmt.Sprint(targetID)).Scan(ctx, &stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored, "Erased identity unique") {
		t.Fatal("export changed the append-only audit snapshot")
	}
}
