package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/validation"
)

type auditRow struct {
	ActorID    *int64
	EntityType string
	EntityID   string
	Action     string
	BeforeJSON string
	AfterJSON  string
}

// ADR 077: every write a player or admin makes through the website shows up in
// the audit log with its actor, not only corrections.
func TestEverydayChangesAreAudited(t *testing.T) {
	srv, db := setupServer(t)
	ctx := context.Background()
	client := srv.Client()
	adminID := seedAccount(t, db, "audit-admin", "password", validation.RoleAdmin)
	memberID := seedAccount(t, db, "audit-member", "member-password", validation.RoleMember)
	admin := login(t, client, srv, "audit-admin", "password")
	member := login(t, client, srv, "audit-member", "member-password")
	now := time.Now().UTC()
	c := character.Character{PlayerID: memberID, Name: "Protokollant", Class: "Magier", MainSpec: "Feuer", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(db).Create(ctx, &c); err != nil {
		t.Fatal(err)
	}
	if err := enhancepoints.NewRepo(db).InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: memberID, Amount: 150, Type: "adjustment", Reason: "test balance", ActorID: adminID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	call := func(token, method, path string, body any, want int) []byte {
		t.Helper()
		status, out := do(t, client, method, srv.URL+path, token, body)
		if status != want {
			t.Fatalf("%s %s: want %d got %d: %s", method, path, want, status, out)
		}
		return out
	}
	id := func(body []byte) int64 {
		t.Helper()
		var out struct{ Data struct{ ID int64 } }
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		return out.Data.ID
	}

	raidID := id(call(admin, http.MethodPost, "/api/v1/raids", map[string]any{"name": "ICC", "raid_leader_id": adminID}, http.StatusCreated))
	call(admin, http.MethodPatch, fmt.Sprintf("/api/v1/raids/%d", raidID), map[string]any{"name": "ICC Donnerstag"}, http.StatusOK)
	call(admin, http.MethodPost, fmt.Sprintf("/api/v1/raids/%d/attendance", raidID), map[string]any{"player_id": memberID, "character_id": c.ID, "role": "DD"}, http.StatusCreated)

	wishlist := fmt.Sprintf("/api/v1/characters/%d/wishlist/items", c.ID)
	call(member, http.MethodPost, wishlist, map[string]any{"item_id": "50415", "item_name": "Bryntroll", "priority": 1}, http.StatusCreated)
	call(member, http.MethodPut, wishlist+"/50415", map[string]any{"priority": 2}, http.StatusOK)
	call(member, http.MethodDelete, wishlist+"/50415", nil, http.StatusNoContent)

	pollID := id(call(admin, http.MethodPost, "/api/v1/polls", map[string]any{"title": "Raidtag"}, http.StatusCreated))
	call(member, http.MethodPost, fmt.Sprintf("/api/v1/polls/%d/vote", pollID), map[string]any{"choice": "yes"}, http.StatusOK)
	call(admin, http.MethodPost, fmt.Sprintf("/api/v1/polls/%d/close", pollID), nil, http.StatusOK)

	lockID := id(call(member, http.MethodPost, fmt.Sprintf("/api/v1/players/%d/locks", memberID), map[string]any{"raid_id": raidID, "item_name": "Bryntroll"}, http.StatusCreated))
	call(member, http.MethodPost, fmt.Sprintf("/api/v1/players/%d/locks/%d/cancel", memberID, lockID), nil, http.StatusOK)

	rankID := id(call(admin, http.MethodPost, "/api/v1/guild-ranks", map[string]any{"name": "Raider"}, http.StatusCreated))
	call(admin, http.MethodPatch, fmt.Sprintf("/api/v1/guild-ranks/%d", rankID), map[string]any{"name": "Kernraider"}, http.StatusOK)
	call(admin, http.MethodPost, "/api/v1/raid-groups", map[string]any{"name": "Gruppe A"}, http.StatusCreated)

	seriesID := id(call(admin, http.MethodPost, "/api/v1/raid-series", map[string]any{"name": "Donnerstagsraid", "raid_leader_id": adminID, "weekdays": []int{4}, "interval_weeks": 1, "start_time": "20:00", "timezone": "Europe/Berlin"}, http.StatusCreated))
	call(admin, http.MethodPatch, fmt.Sprintf("/api/v1/raid-series/%d", seriesID), map[string]any{"start_time": "19:30"}, http.StatusOK)

	newPlayerID := id(call(admin, http.MethodPost, "/api/v1/players", map[string]any{"name": "Neuling"}, http.StatusCreated))
	call(admin, http.MethodPut, fmt.Sprintf("/api/v1/players/%d/account", newPlayerID), map[string]any{"username": "neuling", "password": "ein-langes-passwort", "role": "MEMBER"}, http.StatusOK)

	call(admin, http.MethodPost, fmt.Sprintf("/api/v1/raids/%d/cancel", raidID), nil, http.StatusOK)
	call(member, http.MethodPost, "/api/v1/auth/password", map[string]any{"current_password": "member-password", "new_password": "ein-neues-langes-passwort"}, http.StatusNoContent)

	var rows []auditRow
	if err := db.NewSelect().TableExpr("audit_log").Column("actor_id", "entity_type", "entity_id", "action", "before_json", "after_json").OrderExpr("id ASC").Scan(ctx, &rows); err != nil {
		t.Fatal(err)
	}
	find := func(entity, action string, actor int64) auditRow {
		t.Helper()
		for _, row := range rows {
			if row.EntityType == entity && row.Action == action {
				if row.ActorID == nil || *row.ActorID != actor {
					t.Fatalf("%s %s: actor %v, want %d", entity, action, row.ActorID, actor)
				}
				return row
			}
		}
		t.Fatalf("no audit entry for %s %s in %+v", entity, action, rows)
		return auditRow{}
	}
	find("raid", "create", adminID)
	find("raid", "update", adminID)
	find("raid", "cancel", adminID)
	if row := find("attendance", "correction", adminID); row.BeforeJSON != "null" || !strings.Contains(row.AfterJSON, "Protokollant") {
		t.Fatalf("roster addition: %+v", row)
	}
	find("wishlist_item", "create", memberID)
	if row := find("wishlist_item", "update", memberID); !strings.Contains(row.BeforeJSON, `"priority":1`) || !strings.Contains(row.AfterJSON, `"priority":2`) {
		t.Fatalf("wishlist update: %+v", row)
	}
	if row := find("wishlist_item", "delete", memberID); row.AfterJSON != "null" || !strings.Contains(row.BeforeJSON, "Bryntroll") {
		t.Fatalf("wishlist delete: %+v", row)
	}
	find("poll", "create", adminID)
	if row := find("poll", "vote", memberID); !strings.Contains(row.AfterJSON, "Raidtag") || !strings.Contains(row.AfterJSON, "yes") {
		t.Fatalf("poll vote: %+v", row)
	}
	find("poll", "close", adminID)
	find("item_lock", "create", memberID)
	find("item_lock", "cancel", memberID)
	find("guild_rank", "create", adminID)
	find("guild_rank", "update", adminID)
	find("raid_group", "create", adminID)
	find("raid_series", "create", adminID)
	find("raid_series", "update", adminID)
	find("player", "create", adminID)
	account := find("player", "set_account", adminID)
	if !strings.Contains(account.AfterJSON, `"password_changed":true`) || strings.Contains(account.AfterJSON, "password_hash") || strings.Contains(account.AfterJSON, "ein-langes-passwort") {
		t.Fatalf("account audit must not leak secrets: %s", account.AfterJSON)
	}
	if row := find("player", "password_change", memberID); strings.Contains(row.AfterJSON, "passwort") {
		t.Fatalf("password audit must not contain the password: %s", row.AfterJSON)
	}
}
