package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/httpapi"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

// FR-6.15, FR-6.16: owners set their own specs and reload their profile once a week.
func TestOwnCharacterSpecsAndWeeklyReload(t *testing.T) {
	gearScore := 5800
	feed := fakeRegistrationFeed{characters: map[string]registration.ExternalCharacter{
		"201": {ProfileID: "201", Name: "Ownchar", Class: "Paladin", MainSpec: "Vergeltung", SecondSpec: "Heilig", GearScore: &gearScore, Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=201"},
	}}
	srv, bdb := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: feed})
	client := srv.Client()
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	adminToken := login(t, client, srv, "admin", "pw")
	ownerID := seedAccount(t, bdb, "owner", "pw", validation.RoleMember)
	seedAccount(t, bdb, "other", "pw", validation.RoleMember)
	seedAccount(t, bdb, "guest", "pw", validation.RoleGuest)

	status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/characters", srv.URL, ownerID), adminToken, map[string]any{"profile_id": "201"})
	if status != http.StatusCreated {
		t.Fatalf("create character: status %d body %s", status, body)
	}
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	specsURL := fmt.Sprintf("%s/api/v1/characters/%d/specs", srv.URL, created.Data.ID)
	reloadURL := fmt.Sprintf("%s/api/v1/characters/%d/reload-profile", srv.URL, created.Data.ID)

	ownerToken := login(t, client, srv, "owner", "pw")
	otherToken := login(t, client, srv, "other", "pw")
	guestToken := login(t, client, srv, "guest", "pw")
	if status, _ := do(t, client, http.MethodPut, specsURL, otherToken, map[string]any{"main_spec": "Heilig"}); status != http.StatusForbidden {
		t.Fatalf("other member sets specs: want 403 got %d", status)
	}
	if status, _ := do(t, client, http.MethodPut, specsURL, guestToken, map[string]any{"main_spec": "Heilig"}); status != http.StatusForbidden {
		t.Fatalf("guest sets specs: want 403 got %d", status)
	}
	for _, bad := range []map[string]any{{"main_spec": "Frost"}, {"main_spec": "Heilig", "second_spec": "Heilig"}, {"main_spec": "Heilig", "second_spec": "Feuer"}} {
		if status, _ := do(t, client, http.MethodPut, specsURL, ownerToken, bad); status != http.StatusBadRequest {
			t.Fatalf("invalid specs %v: want 400 got %d", bad, status)
		}
	}
	status, body = do(t, client, http.MethodPut, specsURL, ownerToken, map[string]any{"main_spec": "Heilig", "second_spec": "Schutz (Tank)"})
	if status != http.StatusOK {
		t.Fatalf("owner sets specs: status %d body %s", status, body)
	}

	if status, _ := do(t, client, http.MethodPost, reloadURL, otherToken, nil); status != http.StatusForbidden {
		t.Fatalf("other member reloads: want 403 got %d", status)
	}
	newScore := 5950
	feed.characters["201"] = registration.ExternalCharacter{ProfileID: "201", Name: "Ownchar", Class: "Paladin", MainSpec: "Vergeltung", SecondSpec: "Heilig", GearScore: &newScore, Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=201"}
	status, body = do(t, client, http.MethodPost, reloadURL, ownerToken, nil)
	if status != http.StatusOK {
		t.Fatalf("owner reload: status %d body %s", status, body)
	}
	var reloaded struct {
		Data struct {
			MainSpec          string     `json:"main_spec"`
			SecondSpec        string     `json:"second_spec"`
			GearScore         *int       `json:"gear_score"`
			ProfileReloadedAt *time.Time `json:"profile_reloaded_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded.Data.MainSpec != "Heilig" || reloaded.Data.SecondSpec != "Schutz (Tank)" || reloaded.Data.GearScore == nil || *reloaded.Data.GearScore != newScore || reloaded.Data.ProfileReloadedAt == nil {
		t.Fatalf("reload must keep own specs and update gear score: %+v", reloaded.Data)
	}
	status, body = do(t, client, http.MethodPost, reloadURL, ownerToken, nil)
	if status != http.StatusConflict || !json.Valid(body) || !containsCode(body, "profile_reload_too_soon") {
		t.Fatalf("second owner reload: want 409 profile_reload_too_soon got %d %s", status, body)
	}
	if status, _ := do(t, client, http.MethodPost, reloadURL, adminToken, nil); status != http.StatusOK {
		t.Fatalf("admin reload stays unlimited: got %d", status)
	}
	if _, err := bdb.ExecContext(context.Background(), "UPDATE characters SET profile_reloaded_at = ? WHERE id = ?", time.Now().UTC().Add(-8*24*time.Hour), created.Data.ID); err != nil {
		t.Fatal(err)
	}
	if status, _ := do(t, client, http.MethodPost, reloadURL, ownerToken, nil); status != http.StatusOK {
		t.Fatalf("owner reload after a week: got %d", status)
	}
}

func containsCode(body []byte, code string) bool {
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	return json.Unmarshal(body, &out) == nil && out.Error.Code == code
}
