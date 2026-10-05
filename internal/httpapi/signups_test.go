package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"raid-website/internal/character"
	"raid-website/internal/raid"
	"raid-website/internal/raidsignup"
	"raid-website/internal/validation"
	"testing"
	"time"
)

func TestSignupPermissionsAndRoster(t *testing.T) {
	srv, database := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	seedAccount(t, database, "guest", "pw", validation.RoleGuest)
	member := seedAccount(t, database, "member", "pw", validation.RoleMember)
	staff := seedAccount(t, database, "assistant", "pw", validation.RoleRaidassistent)
	guestToken := login(t, client, srv, "guest", "pw")
	memberToken := login(t, client, srv, "member", "pw")
	staffToken := login(t, client, srv, "assistant", "pw")
	now := time.Now().UTC()
	start := now.Add(time.Hour)
	rd := &raid.Raid{Name: "Signup", RaidLeaderID: staff, Status: validation.RaidStatusPlanned, ScheduledStart: &start, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(database).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	c := &character.Character{PlayerID: member, Name: "Mage", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", CreatedAt: now, UpdatedAt: now, Active: true}
	if err := character.NewRepo(database).Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("%s/api/v1/raids/%d/signups", srv.URL, rd.ID)
	availability := map[string]any{"is_enabled": true}
	choices := map[string]any{"choices": []map[string]any{{"character_id": c.ID, "spec": "Arkan"}}}
	check := func(method, url, token string, body any, want int) []byte {
		t.Helper()
		status, result := do(t, client, method, url, token, body)
		if status != want {
			t.Fatalf("%s %s: %d, want %d: %s", method, url, status, want, result)
		}
		return result
	}
	check(http.MethodGet, base, "", nil, 401)
	body := check(http.MethodGet, base, guestToken, nil, 200)
	var response struct {
		Data raidsignup.Overview `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Entries == nil {
		t.Fatal("nested entries must be []")
	}
	check(http.MethodPut, base+"/availability", memberToken, availability, 403)
	check(http.MethodPut, base, guestToken, choices, 403)
	check(http.MethodPost, base+"/1/withdraw", guestToken, nil, 403)
	check(http.MethodPost, base+"/1/accept", memberToken, map[string]any{}, 403)
	check(http.MethodPut, base+"/availability", staffToken, availability, 204)
	check(http.MethodPut, base, staffToken, choices, 403)
	check(http.MethodPut, base, memberToken, map[string]any{"choices": []any{}, "player_id": staff}, 400)
	body = check(http.MethodPut, base, memberToken, choices, 200)
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	entry := response.Data.Entries[0]
	url := fmt.Sprintf("%s/%d", base, entry.ID)
	check(http.MethodPost, url+"/accept", staffToken, map[string]any{}, 204)
	check(http.MethodPost, url+"/accept", staffToken, map[string]any{}, 204)
	check(http.MethodPost, url+"/withdraw", memberToken, nil, 409)
	body = check(http.MethodGet, base, memberToken, nil, 200)
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if !response.Data.Entries[0].IsAccepted {
		t.Fatal("accepted character missing")
	}
}
