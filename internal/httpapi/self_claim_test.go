package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/httpapi"
	"raid-website/internal/player"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

type selfClaimOut struct {
	Data struct {
		Claimed   bool                  `json:"claimed"`
		Character *character.Character  `json:"character"`
		Request   *registration.Request `json:"request"`
	} `json:"data"`
}

func selfClaimFeed() fakeRegistrationFeed {
	return fakeRegistrationFeed{characters: map[string]registration.ExternalCharacter{
		"42": {ProfileID: "42", Name: "Claimme", Class: "Magier", MainSpec: "Arkan", SecondSpec: "Frost", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=42"},
	}}
}

func decodeSelfClaim(t *testing.T, body []byte) selfClaimOut {
	t.Helper()
	var out selfClaimOut
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode claim: %v body %s", err, body)
	}
	return out
}

// FR-4.6, ADR 116: a MEMBER gets a character nobody owns at once, audited.
func TestReqClaim001_MemberClaimsFreeCharacterAtOnce(t *testing.T) {
	srv, bdb := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: selfClaimFeed()})
	client := srv.Client()
	memberID := seedAccount(t, bdb, "member", "pw", validation.RoleMember)
	token := login(t, client, srv, "member", "pw")

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/characters/claim", token, map[string]any{"profile_id": "42"})
	if status != http.StatusOK {
		t.Fatalf("claim: status %d body %s", status, body)
	}
	out := decodeSelfClaim(t, body)
	if !out.Data.Claimed || out.Data.Character == nil || out.Data.Character.PlayerID != memberID || out.Data.Character.Name != "Claimme" || out.Data.Character.ExternalProfileID != "42" {
		t.Fatalf("unexpected claim result: %s", body)
	}
	count, err := bdb.NewSelect().Model((*registration.Request)(nil)).Count(t.Context())
	if err != nil || count != 0 {
		t.Fatalf("a free claim must not create a request: count %d err %v", count, err)
	}
	audits, err := bdb.NewSelect().Model((*audit.Log)(nil)).Where("action = ? AND entity_type = ?", "claim_character", "character").Count(t.Context())
	if err != nil || audits != 1 {
		t.Fatalf("claim audit entries: %d err %v", audits, err)
	}
}

// FR-4.6: a character of a player without a login moves to the claimant at once.
func TestReqClaim002_MemberTakesCharacterOfPlayerWithoutLogin(t *testing.T) {
	srv, bdb := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: selfClaimFeed()})
	client := srv.Client()
	memberID := seedAccount(t, bdb, "member", "pw", validation.RoleMember)
	now := time.Now().UTC()
	legacy := &player.Player{Name: "Legacy", Role: validation.RoleGuest, Active: true, CreatedAt: now, UpdatedAt: now}
	if _, err := bdb.NewInsert().Model(legacy).Exec(t.Context()); err != nil {
		t.Fatalf("create legacy player: %v", err)
	}
	imported := &character.Character{PlayerID: legacy.ID, Name: "Claimme", Class: "Magier", MainSpec: "Arkan", Active: true, CreatedAt: now, UpdatedAt: now}
	if _, err := bdb.NewInsert().Model(imported).Exec(t.Context()); err != nil {
		t.Fatalf("create imported character: %v", err)
	}
	token := login(t, client, srv, "member", "pw")

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/characters/claim", token, map[string]any{"profile_id": "42"})
	if status != http.StatusOK {
		t.Fatalf("claim: status %d body %s", status, body)
	}
	out := decodeSelfClaim(t, body)
	if !out.Data.Claimed || out.Data.Character == nil || out.Data.Character.ID != imported.ID || out.Data.Character.PlayerID != memberID {
		t.Fatalf("expected the imported character to move: %s", body)
	}
}

// FR-4.6, FR-4.12: a character of a player with a login needs an Admin; the
// request names the claimant in the Admin list.
func TestReqClaim003_CharacterOfLoginPlayerNeedsRequest(t *testing.T) {
	srv, bdb := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: selfClaimFeed()})
	client := srv.Client()
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	ownerID := seedAccount(t, bdb, "owner", "pw", validation.RoleMember)
	memberID := seedAccount(t, bdb, "member", "pw", validation.RoleMember)
	now := time.Now().UTC()
	owned := &character.Character{PlayerID: ownerID, Name: "Claimme", Class: "Magier", MainSpec: "Arkan", Active: true, CreatedAt: now, UpdatedAt: now}
	if _, err := bdb.NewInsert().Model(owned).Exec(t.Context()); err != nil {
		t.Fatalf("create owned character: %v", err)
	}
	token := login(t, client, srv, "member", "pw")

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/characters/claim", token, map[string]any{"profile_id": "42"})
	if status != http.StatusCreated {
		t.Fatalf("claim: status %d body %s", status, body)
	}
	out := decodeSelfClaim(t, body)
	if out.Data.Claimed || out.Data.Request == nil || out.Data.Request.Purpose != registration.PurposeClaim {
		t.Fatalf("expected a claim request: %s", body)
	}
	var stored character.Character
	if err := bdb.NewSelect().Model(&stored).Where("id = ?", owned.ID).Scan(t.Context()); err != nil || stored.PlayerID != ownerID {
		t.Fatalf("the owned character must stay: owner %d err %v", stored.PlayerID, err)
	}

	adminToken := login(t, client, srv, "admin", "pw")
	status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/registration-requests?status=PENDING", adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list requests: status %d body %s", status, body)
	}
	var list struct {
		Data []registration.Request `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Data) != 1 || list.Data[0].Requester == nil || list.Data[0].Requester.PlayerID != memberID || list.Data[0].Requester.PlayerName == "" {
		t.Fatalf("request list must name the claimant: %s", body)
	}

	// The open request now blocks another immediate claim (FR-4.2).
	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/characters/claim", token, map[string]any{"profile_id": "42"})
	if status != http.StatusConflict {
		t.Fatalf("second claim: status %d body %s", status, body)
	}
}

// FR-4.6: a GUEST still only creates a request; without a session the answer is 401.
func TestReqClaim004_GuestGetsRequestAndVisitorIsRejected(t *testing.T) {
	srv, bdb := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: selfClaimFeed()})
	client := srv.Client()
	seedAccount(t, bdb, "guest", "pw", validation.RoleGuest)

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/characters/claim", "", map[string]any{"profile_id": "42"})
	if status != http.StatusUnauthorized {
		t.Fatalf("visitor claim: status %d body %s", status, body)
	}

	token := login(t, client, srv, "guest", "pw")
	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/characters/claim", token, map[string]any{"profile_id": "42"})
	if status != http.StatusCreated {
		t.Fatalf("guest claim: status %d body %s", status, body)
	}
	out := decodeSelfClaim(t, body)
	if out.Data.Claimed || out.Data.Request == nil {
		t.Fatalf("guest must get a request: %s", body)
	}
	count, err := bdb.NewSelect().Model((*character.Character)(nil)).Where("name = ?", "Claimme").Count(t.Context())
	if err != nil || count != 0 {
		t.Fatalf("guest claim must not create a character: count %d err %v", count, err)
	}
}
