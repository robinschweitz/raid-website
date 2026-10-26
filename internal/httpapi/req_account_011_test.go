package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/character"
	"raid-website/internal/httpapi"
	"raid-website/internal/player"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

// req011Setup starts a server with an Admin and the database handle, so a test
// can seed existing players around a redeem.
func req011Setup(t *testing.T) (*httptest.Server, *bun.DB, string) {
	t.Helper()
	feed := fakeRegistrationFeed{characters: map[string]registration.ExternalCharacter{
		"1": {ProfileID: "1", Name: "Firsthero", Class: "Magier", MainSpec: "Arkan", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=1"},
		"2": {ProfileID: "2", Name: "Secondhero", Class: "Krieger", MainSpec: "Waffen", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=2"},
	}}
	srv, bdb := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: feed})
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	return srv, bdb, login(t, srv.Client(), srv, "admin", "pw")
}

func redeemNewAccount(t *testing.T, srv *httptest.Server, code, username string) (int, []byte) {
	t.Helper()
	return do(t, srv.Client(), http.MethodPost, srv.URL+"/api/v1/auth/register-with-code", "", map[string]any{"code": code, "username": username, "password": "valid-secret-1"})
}

// redeemedRole reads the role from the redeem response (the caller's own player).
func redeemedRole(t *testing.T, body []byte) string {
	t.Helper()
	var out struct {
		Data struct {
			Role string `json:"role"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode redeem response %s: %v", body, err)
	}
	return out.Data.Role
}

func storedRole(t *testing.T, bdb *bun.DB, playerID int64) validation.Role {
	t.Helper()
	p, err := player.NewRepo(bdb).Get(context.Background(), playerID)
	if err != nil {
		t.Fatalf("load player %d: %v", playerID, err)
	}
	return p.Role
}

// seedLoginlessGuest creates a GUEST player without login who owns one character.
// A non-empty profileID links that character to the Rising-Gods profile.
func seedLoginlessGuest(t *testing.T, bdb *bun.DB, characterName, profileID string) int64 {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	p := &player.Player{Name: characterName, Role: validation.RoleGuest, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := player.NewRepo(bdb).Create(ctx, p); err != nil {
		t.Fatalf("seed loginless player: %v", err)
	}
	c := &character.Character{PlayerID: p.ID, Name: characterName, Class: "Magier", MainSpec: "Arkan", Active: true, CreatedAt: now, UpdatedAt: now}
	if profileID != "" {
		c.ExternalSource = registration.ExternalSourceRisingGods
		c.ExternalProfileID = profileID
	}
	if err := character.NewRepo(bdb).Create(ctx, c); err != nil {
		t.Fatalf("seed character: %v", err)
	}
	return p.ID
}

// AC-01: a new account created from a registration code is a MEMBER.
func TestReqAccount011_AC01(t *testing.T) {
	srv, bdb, adminToken := req011Setup(t)
	code := registrationCode(t, srv, srv.URL, adminToken, "1")
	status, body := redeemNewAccount(t, srv, code, "freshname")
	if status != http.StatusOK {
		t.Fatalf("redeem: status %d body %s", status, body)
	}
	if role := redeemedRole(t, body); role != string(validation.RoleMember) {
		t.Fatalf("response role %q, want MEMBER", role)
	}
	created, err := player.NewRepo(bdb).FindByUsername(context.Background(), "freshname")
	if err != nil {
		t.Fatalf("load new account: %v", err)
	}
	if created.Role != validation.RoleMember {
		t.Fatalf("stored role %q, want MEMBER", created.Role)
	}
}

// AC-02: existing GUEST accounts keep their role; there is no backfill (FR-4.10).
func TestReqAccount011_AC02(t *testing.T) {
	srv, bdb, adminToken := req011Setup(t)
	guestID := seedAccount(t, bdb, "oldguest", "pw", validation.RoleGuest)
	code := registrationCode(t, srv, srv.URL, adminToken, "2")
	if status, body := redeemNewAccount(t, srv, code, "newmember"); status != http.StatusOK {
		t.Fatalf("redeem: status %d body %s", status, body)
	}
	if role := storedRole(t, bdb, guestID); role != validation.RoleGuest {
		t.Fatalf("existing GUEST became %q, want GUEST", role)
	}
}

// AC-03: a code that attaches to an existing GUEST player without login makes
// that player a MEMBER (FR-4.5, ADR 103).
func TestReqAccount011_AC03(t *testing.T) {
	cases := []struct {
		name      string
		profileID string
	}{
		{name: "matched by Rising-Gods profile", profileID: "1"},
		{name: "matched by character name", profileID: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, bdb, adminToken := req011Setup(t)
			playerID := seedLoginlessGuest(t, bdb, "Firsthero", tc.profileID)
			var choice map[string]any
			if tc.profileID == "" {
				// SC-049: a name match attaches only after the Admin confirmed it.
				choice = map[string]any{"match_choice": "ATTACH", "match_player_id": playerID}
			}
			code := registrationCodeWith(t, srv, srv.URL, adminToken, "1", choice)
			status, body := redeemNewAccount(t, srv, code, "attachedname")
			if status != http.StatusOK {
				t.Fatalf("redeem: status %d body %s", status, body)
			}
			if role := redeemedRole(t, body); role != string(validation.RoleMember) {
				t.Fatalf("response role %q, want MEMBER", role)
			}
			if role := storedRole(t, bdb, playerID); role != validation.RoleMember {
				t.Fatalf("attached player role %q, want MEMBER", role)
			}
		})
	}
}

// AC-03b: attaching a login never lowers a role above MEMBER.
func TestReqAccount011_AC03KeepsHigherRole(t *testing.T) {
	srv, bdb, adminToken := req011Setup(t)
	playerID := seedLoginlessGuest(t, bdb, "Firsthero", "1")
	if _, err := bdb.NewUpdate().Model((*player.Player)(nil)).Set("role = ?", validation.RoleRaidassistent).Where("id = ?", playerID).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	code := registrationCode(t, srv, srv.URL, adminToken, "1")
	if status, body := redeemNewAccount(t, srv, code, "attachedname"); status != http.StatusOK {
		t.Fatalf("redeem: status %d body %s", status, body)
	}
	if role := storedRole(t, bdb, playerID); role != validation.RoleRaidassistent {
		t.Fatalf("attached player role %q, want RAIDASSISTENT", role)
	}
}
