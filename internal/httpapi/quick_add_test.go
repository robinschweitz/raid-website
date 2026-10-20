package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/httpapi"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

func TestRaidQuickAddAtomicRetriesAndPermissions(t *testing.T) {
	feed := fakeRegistrationFeed{characters: map[string]registration.ExternalCharacter{
		"201": {ProfileID: "201", Name: "Newchar", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=201"},
		"202": {ProfileID: "202", Name: "Changed", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=202"},
		"203": {ProfileID: "203", Name: "Archived visitor", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=203"},
		"204": {ProfileID: "204", Name: "Cancelled visitor", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=204"},
		"205": {ProfileID: "205", Name: "Rollbackchar", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=205"},
	}}
	srv, db := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: feed})
	ctx := context.Background()
	staff := seedAccount(t, db, "staff", "pw", validation.RoleRaidassistent)
	seedAccount(t, db, "member", "pw", validation.RoleMember)
	token := login(t, srv.Client(), srv, "staff", "pw")
	member := login(t, srv.Client(), srv, "member", "pw")
	rd := &raid.Raid{Name: "Quick add", Status: validation.RaidStatusOpen, RaidLeaderID: staff, CreatedAt: time.Now().UTC()}
	if err := raid.NewRepo(db).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("%s/api/v1/raids/%d/quick-add", srv.URL, rd.ID)
	body := map[string]any{"request_id": "first", "profile_id": "201", "role": "DD"}
	for _, auth := range []string{"", member} {
		status, response := do(t, srv.Client(), http.MethodPost, url, auth, body)
		if status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Fatalf("unauthorized: %d %s", status, response)
		}
	}
	var first attendance.Attendance
	for i := 0; i < 2; i++ {
		status, response := do(t, srv.Client(), http.MethodPost, url, token, body)
		if status != http.StatusCreated {
			t.Fatalf("create/retry: %d %s", status, response)
		}
		var result struct{ Data attendance.Attendance }
		if err := json.Unmarshal(response, &result); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = result.Data
		} else if first.ID != result.Data.ID {
			t.Fatal("retry duplicated attendance")
		}
	}
	owner, err := player.NewRepo(db).Get(ctx, first.PlayerID)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Name != "Newchar" || owner.Role != validation.RoleGuest || owner.Username != nil || owner.PasswordHash != "" {
		t.Fatalf("unexpected owner: %+v", owner)
	}
	body["profile_id"] = "202"
	if status, _ := do(t, srv.Client(), http.MethodPost, url, token, body); status != http.StatusConflict {
		t.Fatalf("changed retry: %d", status)
	}
	body["request_id"] = "second"
	body["player_id"] = owner.ID
	// The owner already raids with Newchar: a second character is refused and
	// nothing is created (ADR 092).
	if status, response := do(t, srv.Client(), http.MethodPost, url, token, body); status != http.StatusConflict || !strings.Contains(string(response), "player_already_in_roster") {
		t.Fatalf("second character of a rostered owner: %d %s", status, response)
	}
	otherOwner := seedAccount(t, db, "other", "pw", validation.RoleMember)
	body["request_id"] = "second-other"
	body["player_id"] = otherOwner
	if status, response := do(t, srv.Client(), http.MethodPost, url, token, body); status != http.StatusCreated {
		t.Fatalf("existing owner: %d %s", status, response)
	}
	body["request_id"] = "rollback"
	body["player_id"] = 0
	body["player_name"] = "Rollback owner"
	body["profile_id"] = "205"
	body["role"] = "INVALID"
	if status, _ := do(t, srv.Client(), http.MethodPost, url, token, body); status != http.StatusBadRequest {
		t.Fatalf("invalid role: %d", status)
	}
	owners, err := player.NewRepo(db).List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(owners) != 4 {
		t.Fatalf("orphan owner: %d", len(owners))
	}
	chars, err := character.NewRepo(db).List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(chars) != 2 {
		t.Fatalf("orphan character: %d", len(chars))
	}
	body["role"] = "DD"
	body["profile_id"] = "201"
	body["request_id"] = "duplicate-profile"
	if status, _ := do(t, srv.Client(), http.MethodPost, url, token, body); status != http.StatusConflict {
		t.Fatalf("duplicate profile: %d", status)
	}
	body["player_name"] = owner.Name
	if status, response := do(t, srv.Client(), http.MethodPost, url, token, body); status != http.StatusConflict {
		t.Fatalf("existing name must require explicit owner: %d %s", status, response)
	}
	body["profile_id"] = "203"
	body["request_id"] = "archived"
	body["player_name"] = "Archived visitor"
	now := time.Now().UTC()
	rd.Status = validation.RaidStatusCompleted
	rd.ArchivedAt = &now
	rd.CompletedAt = &now
	if err := raid.NewRepo(db).Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	if status, _ := do(t, srv.Client(), http.MethodPost, url, token, body); status != http.StatusBadRequest {
		t.Fatalf("missing archive reason: %d", status)
	}
	body["reason"] = "Missing participant"
	status, response := do(t, srv.Client(), http.MethodPost, url, token, body)
	if status != http.StatusCreated {
		t.Fatalf("archived addition: %d %s", status, response)
	}
	var archived struct{ Data attendance.Attendance }
	if err := json.Unmarshal(response, &archived); err != nil {
		t.Fatal(err)
	}
	balance, err := enhancepoints.NewRepo(db).Balance(ctx, archived.Data.PlayerID)
	if err != nil || balance != 10 {
		t.Fatalf("archive correction EP: %d %v", balance, err)
	}
	if status, _ := do(t, srv.Client(), http.MethodPost, url, token, body); status != http.StatusCreated {
		t.Fatalf("archived retry: %d", status)
	}
	balance, err = enhancepoints.NewRepo(db).Balance(ctx, archived.Data.PlayerID)
	if err != nil || balance != 10 {
		t.Fatalf("retry awarded EP twice: %d %v", balance, err)
	}
	var audits int
	if err := db.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("entity_type = ? AND entity_id = ?", "raid_quick_add", fmt.Sprint(archived.Data.ID)).Scan(ctx, &audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("audit count: %d", audits)
	}
	rd.Status = validation.RaidStatusCancelled
	if err := raid.NewRepo(db).Update(ctx, rd); err != nil {
		t.Fatal(err)
	}
	body["request_id"] = "cancelled"
	body["profile_id"] = "204"
	body["player_name"] = "Cancelled visitor"
	if status, _ := do(t, srv.Client(), http.MethodPost, url, token, body); status != http.StatusConflict {
		t.Fatalf("cancelled: %d", status)
	}
	if status, _ := do(t, srv.Client(), http.MethodPost, srv.URL+"/api/v1/players", token, map[string]any{"name": "Forbidden"}); status != http.StatusForbidden {
		t.Fatalf("general creation must remain admin: %d", status)
	}
	for _, path := range []string{"/api/v1/characters", "/api/v1/character-options"} {
		if status, _ := do(t, srv.Client(), http.MethodGet, srv.URL+path, member, nil); status != http.StatusOK {
			t.Fatalf("directory: %d", status)
		}
	}
}
