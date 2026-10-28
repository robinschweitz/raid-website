package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/character"
	"raid-website/internal/httpapi"
	"raid-website/internal/player"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

// req012Env is a server with an Admin, a request for "ImportedHero" and an
// imported player without a login who already owns a character of that name.
type req012Env struct {
	srv        *httptest.Server
	client     *http.Client
	bdb        *bun.DB
	adminToken string
	ownerID    int64
	codePath   string
}

func newReq012Env(t *testing.T) req012Env {
	t.Helper()
	feed := fakeRegistrationFeed{characters: map[string]registration.ExternalCharacter{
		"42": {ProfileID: "42", Name: "ImportedHero", Class: "Magier", MainSpec: "Arkan", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=42"},
	}}
	srv, bdb := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: feed})
	client := srv.Client()
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	now := time.Now().UTC()
	owner := &player.Player{Name: "Legacy owner", Role: validation.RoleGuest, Active: true, CreatedAt: now, UpdatedAt: now}
	if _, err := bdb.NewInsert().Model(owner).Exec(t.Context()); err != nil {
		t.Fatalf("create imported owner: %v", err)
	}
	imported := &character.Character{PlayerID: owner.ID, Name: "ImportedHero", Class: "Magier", MainSpec: "Arkan", Active: true, CreatedAt: now, UpdatedAt: now}
	if _, err := bdb.NewInsert().Model(imported).Exec(t.Context()); err != nil {
		t.Fatalf("create imported character: %v", err)
	}
	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/registration-requests", "", map[string]any{"profile_id": "42"})
	if status != http.StatusCreated {
		t.Fatalf("create request: status %d body %s", status, body)
	}
	var req struct {
		Data registration.Request `json:"data"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return req012Env{
		srv:        srv,
		client:     client,
		bdb:        bdb,
		adminToken: login(t, client, srv, "admin", "pw"),
		ownerID:    owner.ID,
		codePath:   fmt.Sprintf("%s/api/v1/registration-requests/%d/code", srv.URL, req.Data.ID),
	}
}

func (e req012Env) redeem(t *testing.T, code, username string) (int, []byte) {
	return do(t, e.client, http.MethodPost, e.srv.URL+"/api/v1/auth/register-with-code", "", map[string]any{"code": code, "username": username, "password": "valid-secret-1"})
}

func (e req012Env) createCode(t *testing.T, payload any) string {
	t.Helper()
	status, body := do(t, e.client, http.MethodPost, e.codePath, e.adminToken, payload)
	if status != http.StatusCreated {
		t.Fatalf("create code: status %d body %s", status, body)
	}
	var out struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode code: %v", err)
	}
	return out.Data.Code
}

func (e req012Env) loadPlayer(t *testing.T, id int64) *player.Player {
	t.Helper()
	p := new(player.Player)
	if err := e.bdb.NewSelect().Model(p).Where("id = ?", id).Scan(t.Context()); err != nil {
		t.Fatalf("load player %d: %v", id, err)
	}
	return p
}

func TestReqAccount012_AC01(t *testing.T) {
	e := newReq012Env(t)

	// The Admin sees the matching player before choosing.
	status, body := do(t, e.client, http.MethodGet, e.srv.URL+"/api/v1/registration-requests?status=PENDING", e.adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d body %s", status, body)
	}
	var list struct {
		Data []registration.Request `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Data) != 1 || list.Data[0].NameMatch == nil || list.Data[0].NameMatch.PlayerID != e.ownerID {
		t.Fatalf("request does not show the name match: %s", body)
	}

	for _, payload := range []any{nil, map[string]any{}} {
		status, body = do(t, e.client, http.MethodPost, e.codePath, e.adminToken, payload)
		if status != http.StatusUnprocessableEntity || !strings.Contains(string(body), "match_choice_required") {
			t.Fatalf("code without choice: status %d body %s", status, body)
		}
	}
	var codes int
	if err := e.bdb.NewSelect().Table("registration_codes").ColumnExpr("COUNT(*)").Scan(t.Context(), &codes); err != nil || codes != 0 {
		t.Fatalf("a refused code must not be stored: %d %v", codes, err)
	}

	// A choice naming another player is refused too.
	status, body = do(t, e.client, http.MethodPost, e.codePath, e.adminToken, map[string]any{"match_choice": "ATTACH", "match_player_id": e.ownerID + 100})
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(body), "match_choice_invalid") {
		t.Fatalf("wrong player: status %d body %s", status, body)
	}
}

func TestReqAccount012_AC02(t *testing.T) {
	e := newReq012Env(t)
	code := e.createCode(t, map[string]any{"match_choice": "NEW"})
	status, body := e.redeem(t, code, "newcomer")
	if status != http.StatusOK {
		t.Fatalf("redeem: status %d body %s", status, body)
	}
	var out struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode player: %v", err)
	}
	if out.Data.ID == e.ownerID {
		t.Fatal("new player choice attached the login to the matching player")
	}
	if owner := e.loadPlayer(t, e.ownerID); owner.Username != nil || owner.PasswordHash != "" {
		t.Fatalf("matching player got a login: %#v", owner)
	}
	var owned int
	if err := e.bdb.NewSelect().Table("characters").ColumnExpr("COUNT(*)").Where("player_id = ? AND name = ?", e.ownerID, "ImportedHero").Scan(t.Context(), &owned); err != nil || owned != 1 {
		t.Fatalf("the imported character must stay with its owner: %d %v", owned, err)
	}
	var fresh int
	if err := e.bdb.NewSelect().Table("characters").ColumnExpr("COUNT(*)").Where("player_id = ? AND name = ?", out.Data.ID, "ImportedHero").Scan(t.Context(), &fresh); err != nil || fresh != 1 {
		t.Fatalf("the new player must get its own character: %d %v", fresh, err)
	}
}

func TestReqAccount012_AC03(t *testing.T) {
	e := newReq012Env(t)
	code := e.createCode(t, map[string]any{"match_choice": "ATTACH", "match_player_id": e.ownerID})
	status, body := e.redeem(t, code, "importedhero")
	if status != http.StatusOK {
		t.Fatalf("redeem: status %d body %s", status, body)
	}
	owner := e.loadPlayer(t, e.ownerID)
	if owner.Username == nil || *owner.Username != "importedhero" || owner.PasswordHash == "" {
		t.Fatalf("login was not attached to the chosen player: %#v", owner)
	}
	if owner.Role != validation.RoleMember {
		t.Fatalf("attached GUEST must become MEMBER, got %s", owner.Role)
	}
	var snapshot string
	if err := e.bdb.NewSelect().Table("audit_log").Column("after_json").Where("action = ?", "create_code").Scan(t.Context(), &snapshot); err != nil || !strings.Contains(snapshot, `"match_choice":"ATTACH"`) {
		t.Fatalf("code creation audit must record the choice: %q %v", snapshot, err)
	}
}

func TestReqAccount012_AC04(t *testing.T) {
	e := newReq012Env(t)
	seedAccount(t, e.bdb, "guest", "pw", validation.RoleGuest)
	seedAccount(t, e.bdb, "member", "pw", validation.RoleMember)
	seedAccount(t, e.bdb, "leader", "pw", validation.RoleRaidleader)
	payload := map[string]any{"match_choice": "ATTACH", "match_player_id": e.ownerID}
	for _, name := range []string{"guest", "member", "leader"} {
		token := login(t, e.client, e.srv, name, "pw")
		status, body := do(t, e.client, http.MethodPost, e.codePath, token, payload)
		if status != http.StatusForbidden {
			t.Fatalf("%s: status %d body %s", name, status, body)
		}
	}
	status, _ := do(t, e.client, http.MethodPost, e.codePath, "", payload)
	if status != http.StatusUnauthorized {
		t.Fatalf("no session: status %d", status)
	}
}

// A name match never attaches a login on its own (SC-049): a code without a
// stored choice, such as one made before this rule, redeems as a new player.
func TestReqAccount012_NoMatchByNameAtRedeem(t *testing.T) {
	e := newReq012Env(t)
	code := e.createCode(t, map[string]any{"match_choice": "NEW"})
	if _, err := e.bdb.NewUpdate().Table("registration_codes").Set("match_choice = ''").Where("1 = 1").Exec(t.Context()); err != nil {
		t.Fatalf("clear choice: %v", err)
	}
	if status, body := e.redeem(t, code, "newcomer"); status != http.StatusOK {
		t.Fatalf("redeem: status %d body %s", status, body)
	}
	if owner := e.loadPlayer(t, e.ownerID); owner.Username != nil {
		t.Fatal("redeem matched by name without a stored choice")
	}
}
