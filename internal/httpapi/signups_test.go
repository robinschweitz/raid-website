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
	"slices"
	"strings"
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

func TestAbsenceDeclareRevokeAndPermissions(t *testing.T) {
	srv, database := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	guest := seedAccount(t, database, "guest", "pw", validation.RoleGuest)
	member := seedAccount(t, database, "member", "pw", validation.RoleMember)
	other := seedAccount(t, database, "other", "pw", validation.RoleMember)
	staff := seedAccount(t, database, "assistant", "pw", validation.RoleRaidassistent)
	guestToken := login(t, client, srv, "guest", "pw")
	memberToken := login(t, client, srv, "member", "pw")
	otherToken := login(t, client, srv, "other", "pw")
	staffToken := login(t, client, srv, "assistant", "pw")
	_ = guest
	now := time.Now().UTC()
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	newRaid := func(name string, start *time.Time) int64 {
		rd := &raid.Raid{Name: name, RaidLeaderID: staff, Status: validation.RaidStatusPlanned, ScheduledStart: start, CreatedAt: now, UpdatedAt: now}
		if err := raid.NewRepo(database).Create(ctx, rd); err != nil {
			t.Fatal(err)
		}
		return rd.ID
	}
	open, started := newRaid("Open", &future), newRaid("Started", &past)
	url := func(raidID, playerID int64) string {
		return fmt.Sprintf("%s/api/v1/raids/%d/absences/%d", srv.URL, raidID, playerID)
	}
	check := func(method, u, token string, body any, want int) []byte {
		t.Helper()
		status, result := do(t, client, method, u, token, body)
		if status != want {
			t.Fatalf("%s %s: %d, want %d: %s", method, u, status, want, result)
		}
		return result
	}
	absences := func(token string, raidID int64) []raidsignup.Absence {
		t.Helper()
		body := check(http.MethodGet, fmt.Sprintf("%s/api/v1/raids/%d/signups", srv.URL, raidID), token, nil, 200)
		var res struct {
			Data raidsignup.Overview `json:"data"`
		}
		if err := json.Unmarshal(body, &res); err != nil {
			t.Fatal(err)
		}
		if res.Data.Absences == nil {
			t.Fatal("absences must be []")
		}
		return res.Data.Absences
	}
	check(http.MethodPut, url(open, member), "", map[string]any{}, 401)
	check(http.MethodPut, url(open, member), guestToken, map[string]any{}, 403)
	// Others cannot decline for someone else; staff can.
	check(http.MethodPut, url(open, member), otherToken, map[string]any{"reason": "x"}, 403)
	check(http.MethodPut, url(open, member), memberToken, map[string]any{"reason": strings.Repeat("ä", 201)}, 400)
	check(http.MethodPut, url(open, member), memberToken, map[string]any{"reason": "  Krank  "}, 200)
	got := absences(otherToken, open)
	if len(got) != 1 || got[0].PlayerID != member || got[0].Reason != "Krank" {
		t.Fatalf("absences = %+v", got)
	}
	// Updating the reason keeps a single row.
	check(http.MethodPut, url(open, member), memberToken, map[string]any{"reason": "Urlaub"}, 200)
	if got := absences(memberToken, open); len(got) != 1 || got[0].Reason != "Urlaub" {
		t.Fatalf("absences = %+v", got)
	}
	check(http.MethodDelete, url(open, member), otherToken, nil, 403)
	check(http.MethodDelete, url(open, member), memberToken, nil, 200)
	check(http.MethodDelete, url(open, member), memberToken, nil, 200)
	if got := absences(memberToken, open); len(got) != 0 {
		t.Fatalf("absences = %+v", got)
	}
	// After the scheduled start only staff may change it.
	check(http.MethodPut, url(started, member), memberToken, map[string]any{}, 409)
	check(http.MethodPut, url(started, member), staffToken, map[string]any{"reason": "Telefonisch"}, 200)
	check(http.MethodDelete, url(started, member), memberToken, nil, 409)
	check(http.MethodDelete, url(started, member), staffToken, nil, 200)
	check(http.MethodPut, url(open, 99999), staffToken, map[string]any{}, 404)

	// ADR 082: "unsicher" is a second status; players and staff can set it and
	// switch between the statuses without creating a second row.
	check(http.MethodPut, url(open, member), memberToken, map[string]any{"status": "MAYBE"}, 400)
	check(http.MethodPut, url(open, member), memberToken, map[string]any{"reason": "Spätschicht", "status": "TENTATIVE"}, 200)
	if got := absences(memberToken, open); len(got) != 1 || got[0].Status != raidsignup.AbsenceTentative || got[0].Reason != "Spätschicht" {
		t.Fatalf("tentative = %+v", got)
	}
	check(http.MethodPut, url(open, member), memberToken, map[string]any{}, 200)
	if got := absences(memberToken, open); len(got) != 1 || got[0].Status != raidsignup.AbsenceDeclined {
		t.Fatalf("an absence without status is declined: %+v", got)
	}
	check(http.MethodPut, url(open, other), staffToken, map[string]any{"status": "TENTATIVE"}, 200)
	if got := absences(memberToken, open); len(got) != 2 || got[1].PlayerID != other || got[1].Status != raidsignup.AbsenceTentative {
		t.Fatalf("staff marks unsure: %+v", got)
	}
	var actions []string
	if err := database.NewSelect().TableExpr("audit_log").Column("action").Where("entity_type = 'raid_signup'").OrderExpr("id ASC").Scan(ctx, &actions); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(actions, "absence_tentative") {
		t.Fatalf("tentative audit missing: %v", actions)
	}
}
