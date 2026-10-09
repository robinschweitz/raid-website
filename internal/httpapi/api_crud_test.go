package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/character"
	"raid-website/internal/httpapi"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

func TestCharacterCRUDAndRename(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	token := login(t, client, srv, "admin", "pw")

	playerID := createPlayer(t, client, srv, token, "Owner")

	status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/players/%d/characters", srv.URL, playerID), token, map[string]any{
		"name":      "Altchar",
		"class":     "krieger",
		"main_spec": "Waffen",
		"is_main":   true,
	})
	if status != http.StatusCreated {
		t.Fatalf("create character: status %d body %s", status, body)
	}
	var charOut struct {
		Data struct {
			ID    int64  `json:"id"`
			Class string `json:"class"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &charOut); err != nil {
		t.Fatalf("decode character: %v", err)
	}
	if charOut.Data.Class != "Krieger" {
		t.Fatalf("class normalization: want Krieger got %q", charOut.Data.Class)
	}
	charID := charOut.Data.ID

	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d/characters", srv.URL, playerID), token, nil)
	if status != http.StatusOK {
		t.Fatalf("list characters: status %d body %s", status, body)
	}

	status, body = do(t, client, http.MethodPatch, fmt.Sprintf("%s/api/v1/characters/%d", srv.URL, charID), token, map[string]any{
		"name":   "Newname",
		"reason": "rename token",
	})
	if status != http.StatusOK {
		t.Fatalf("rename character: status %d body %s", status, body)
	}

	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/characters/%d/name-history", srv.URL, charID), token, nil)
	if status != http.StatusOK {
		t.Fatalf("name history: status %d body %s", status, body)
	}
	var histOut struct {
		Data []struct {
			OldName string `json:"old_name"`
			NewName string `json:"new_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &histOut); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(histOut.Data) != 1 || histOut.Data[0].OldName != "Altchar" || histOut.Data[0].NewName != "Newname" {
		t.Fatalf("unexpected history: %s", body)
	}

	status, _ = do(t, client, http.MethodPatch, fmt.Sprintf("%s/api/v1/characters/%d", srv.URL, charID), token, map[string]any{"class": "Barbar"})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown class: want 400 got %d", status)
	}
}

func TestRosterCRUD(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	token := login(t, client, srv, "admin", "pw")

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/guild-ranks", token, map[string]any{"name": "Raidleader", "sort_order": 1})
	if status != http.StatusCreated {
		t.Fatalf("create guild rank: status %d body %s", status, body)
	}
	var rankOut struct {
		Data struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &rankOut); err != nil {
		t.Fatalf("decode rank: %v", err)
	}
	status, body = do(t, client, http.MethodPatch, fmt.Sprintf("%s/api/v1/guild-ranks/%d", srv.URL, rankOut.Data.ID), token, map[string]any{"name": "Raid Lead"})
	if status != http.StatusOK {
		t.Fatalf("update guild rank: status %d body %s", status, body)
	}

	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/raid-groups", token, map[string]any{"name": "Gruppe 1", "color": "#ff0000"})
	if status != http.StatusCreated {
		t.Fatalf("create raid group: status %d body %s", status, body)
	}
	var groupOut struct {
		Data struct {
			ID    int64  `json:"id"`
			Color string `json:"color"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &groupOut); err != nil {
		t.Fatalf("decode group: %v", err)
	}
	status, body = do(t, client, http.MethodPatch, fmt.Sprintf("%s/api/v1/raid-groups/%d", srv.URL, groupOut.Data.ID), token, map[string]any{"color": "#00ff00"})
	if status != http.StatusOK {
		t.Fatalf("update raid group: status %d body %s", status, body)
	}

	status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/guild-ranks", token, nil)
	if status != http.StatusOK {
		t.Fatalf("list guild ranks: status %d body %s", status, body)
	}
	status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/raid-groups", token, nil)
	if status != http.StatusOK {
		t.Fatalf("list raid groups: status %d body %s", status, body)
	}
}

func TestPollLifecycle(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	token := login(t, client, srv, "admin", "pw")

	closes := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", token, map[string]any{
		"title":     "Raid am Sonntag?",
		"closes_at": closes,
	})
	if status != http.StatusCreated {
		t.Fatalf("create poll: status %d body %s", status, body)
	}
	var pollOut struct {
		Data struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &pollOut); err != nil {
		t.Fatalf("decode poll: %v", err)
	}
	if pollOut.Data.Status != "OPEN" {
		t.Fatalf("poll status: want OPEN got %q", pollOut.Data.Status)
	}
	pollID := pollOut.Data.ID

	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/polls/%d/vote", srv.URL, pollID), token, map[string]any{"choice": "yes"})
	if status != http.StatusOK {
		t.Fatalf("vote: status %d body %s", status, body)
	}
	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/polls/%d/vote", srv.URL, pollID), token, map[string]any{"choice": "no"})
	if status != http.StatusOK {
		t.Fatalf("revote: status %d body %s", status, body)
	}
	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/polls/%d/votes", srv.URL, pollID), token, nil)
	if status != http.StatusOK {
		t.Fatalf("list votes: status %d body %s", status, body)
	}
	var votesOut struct {
		Data []struct {
			Choice string `json:"choice"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &votesOut); err != nil {
		t.Fatalf("decode votes: %v", err)
	}
	if len(votesOut.Data) != 1 || votesOut.Data[0].Choice != "no" {
		t.Fatalf("expected single updated vote 'no', body %s", body)
	}

	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", token, map[string]any{
		"title":          "Welche Bosse?",
		"poll_type":      "CHOICE",
		"allow_multiple": true,
		"options":        []string{"Marrowgar", "Saurfang", "Sindragosa"},
	})
	if status != http.StatusCreated {
		t.Fatalf("create choice poll: status %d body %s", status, body)
	}
	var choicePollOut struct {
		Data struct {
			ID            int64  `json:"id"`
			PollType      string `json:"poll_type"`
			AllowMultiple bool   `json:"allow_multiple"`
			Options       []struct {
				Label string `json:"label"`
			} `json:"options"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &choicePollOut); err != nil {
		t.Fatalf("decode choice poll: %v", err)
	}
	if choicePollOut.Data.PollType != "CHOICE" || !choicePollOut.Data.AllowMultiple || len(choicePollOut.Data.Options) != 3 {
		t.Fatalf("unexpected choice poll body %s", body)
	}
	choicePollID := choicePollOut.Data.ID

	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/polls/%d/vote", srv.URL, choicePollID), token, map[string]any{"choices": []string{"Marrowgar", "Sindragosa"}})
	if status != http.StatusOK {
		t.Fatalf("choice vote: status %d body %s", status, body)
	}
	var choiceVoteOut struct {
		Data struct {
			Choice  string   `json:"choice"`
			Choices []string `json:"choices"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &choiceVoteOut); err != nil {
		t.Fatalf("decode choice vote: %v", err)
	}
	if choiceVoteOut.Data.Choice != "Marrowgar, Sindragosa" || len(choiceVoteOut.Data.Choices) != 2 {
		t.Fatalf("unexpected choice vote body %s", body)
	}

	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/polls/%d/vote", srv.URL, choicePollID), token, map[string]any{"choices": []string{"Putricide"}})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid choice vote: want 400 got %d body %s", status, body)
	}

	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/polls/%d/close", srv.URL, pollID), token, nil)
	if status != http.StatusOK {
		t.Fatalf("close: status %d body %s", status, body)
	}
	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/polls/%d/vote", srv.URL, pollID), token, map[string]any{"choice": "maybe"})
	if status != http.StatusConflict {
		t.Fatalf("vote after close: want 409 got %d body %s", status, body)
	}
}

func TestAccountProvisioning(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	adminToken := login(t, client, srv, "admin", "pw")

	memberID := createPlayer(t, client, srv, adminToken, "Newbie")

	status, body := do(t, client, http.MethodPut, fmt.Sprintf("%s/api/v1/players/%d/account", srv.URL, memberID), adminToken, map[string]any{
		"username": "newbie",
		"password": "secret",
		"role":     "MEMBER",
	})
	if status != http.StatusOK {
		t.Fatalf("set account: status %d body %s", status, body)
	}

	memberToken := login(t, client, srv, "newbie", "secret")
	status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", memberToken, nil)
	if status != http.StatusOK {
		t.Fatalf("me: status %d body %s", status, body)
	}
	var meOut struct {
		Data struct {
			ID   int64  `json:"id"`
			Role string `json:"role"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &meOut); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if meOut.Data.ID != memberID || meOut.Data.Role != "MEMBER" {
		t.Fatalf("unexpected me: %s", body)
	}

	status, _ = do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/logout", memberToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("logout: want 204 got %d", status)
	}
}

func TestPlayerListSummaryAndAccountVisibility(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	adminToken := login(t, client, srv, "admin", "pw")

	firstID := createPlayer(t, client, srv, adminToken, "First")
	secondID := createPlayer(t, client, srv, adminToken, "Second")
	for id, username := range map[int64]string{firstID: "first-login", secondID: "second-login"} {
		status, body := do(t, client, http.MethodPut, fmt.Sprintf("%s/api/v1/players/%d/account", srv.URL, id), adminToken, map[string]any{
			"username": username,
			"password": "secret",
			"role":     "MEMBER",
		})
		if status != http.StatusOK {
			t.Fatalf("set account %s: status %d body %s", username, status, body)
		}
	}

	status, body := do(t, client, http.MethodGet, srv.URL+"/api/v1/players", adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("admin list players: status %d body %s", status, body)
	}
	var adminList struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &adminList); err != nil {
		t.Fatalf("decode admin players: %v", err)
	}
	for _, row := range adminList.Data {
		if _, ok := row["username"]; ok {
			t.Fatalf("admin player list includes username: %s", body)
		}
		if _, ok := row["role"]; ok {
			t.Fatalf("admin player list includes role: %s", body)
		}
		if _, ok := row["created_at"]; ok {
			t.Fatalf("admin player list includes created_at: %s", body)
		}
		if _, ok := row["updated_at"]; ok {
			t.Fatalf("admin player list includes updated_at: %s", body)
		}
	}

	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d", srv.URL, firstID), adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("admin get player: status %d body %s", status, body)
	}
	var adminDetail struct {
		Data struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &adminDetail); err != nil {
		t.Fatalf("decode admin player detail: %v", err)
	}
	if adminDetail.Data.Username != "first-login" || adminDetail.Data.Role != "MEMBER" {
		t.Fatalf("admin account fields not visible on detail: %s", body)
	}

	firstToken := login(t, client, srv, "first-login", "secret")
	status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/players", firstToken, nil)
	if status != http.StatusOK {
		t.Fatalf("member list players: status %d body %s", status, body)
	}
	var memberList struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &memberList); err != nil {
		t.Fatalf("decode member players: %v", err)
	}
	for _, row := range memberList.Data {
		if _, ok := row["username"]; ok {
			t.Fatalf("member player list includes username: %s", body)
		}
		if _, ok := row["role"]; ok {
			t.Fatalf("member player list includes role: %s", body)
		}
	}

	status, body = do(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/players/%d", srv.URL, secondID), firstToken, nil)
	if status != http.StatusOK {
		t.Fatalf("member get other player: status %d body %s", status, body)
	}
	var other struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &other); err != nil {
		t.Fatalf("decode other player: %v", err)
	}
	if _, ok := other.Data["username"]; ok {
		t.Fatalf("other detail username visible to member: %s", body)
	}
	if _, ok := other.Data["role"]; ok {
		t.Fatalf("other detail role visible to member: %s", body)
	}
}

func TestRegistrationWithRisingGodsCode(t *testing.T) {
	feed := fakeRegistrationFeed{characters: map[string]registration.ExternalCharacter{
		"9390923": {ProfileID: "9390923", Name: "Bigdevil", Class: "Krieger", MainSpec: "Schutz (Tank)", SecondSpec: "Furor", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=9390923"},
	}}
	srv, bdb := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: feed})
	client := srv.Client()
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	adminToken := login(t, client, srv, "admin", "pw")

	status, body := do(t, client, http.MethodGet, srv.URL+"/api/v1/external/rising-gods/characters?q=Big", "", nil)
	if status != http.StatusOK {
		t.Fatalf("search: status %d body %s", status, body)
	}

	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/registration-requests", "", map[string]any{"profile_id": "9390923"})
	if status != http.StatusCreated {
		t.Fatalf("create request: status %d body %s", status, body)
	}
	var requestOut struct {
		Data registration.Request `json:"data"`
	}
	if err := json.Unmarshal(body, &requestOut); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if requestOut.Data.CharacterName != "Bigdevil" || requestOut.Data.MainSpec != "Schutz (Tank)" || requestOut.Data.SecondSpec != "Furor" {
		t.Fatalf("unexpected request: %s", body)
	}

	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/registration-requests/%d/code", srv.URL, requestOut.Data.ID), adminToken, nil)
	if status != http.StatusCreated {
		t.Fatalf("create code: status %d body %s", status, body)
	}
	var codeOut struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &codeOut); err != nil {
		t.Fatalf("decode code: %v", err)
	}
	if codeOut.Data.Code == "" {
		t.Fatalf("empty code: %s", body)
	}

	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/register-with-code", "", map[string]any{"code": codeOut.Data.Code, "username": "bigdevil", "password": "secret"})
	if status != http.StatusOK {
		t.Fatalf("redeem: status %d body %s", status, body)
	}
	if token := login(t, client, srv, "bigdevil", "secret"); token == "" {
		t.Fatal("new account cannot log in")
	}

	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/register-with-code", "", map[string]any{"code": codeOut.Data.Code, "username": "again", "password": "secret"})
	if status != http.StatusBadRequest {
		t.Fatalf("reuse code: want 400 got %d body %s", status, body)
	}
}

func TestCharacterClaimWithRisingGodsCode(t *testing.T) {
	feed := fakeRegistrationFeed{characters: map[string]registration.ExternalCharacter{
		"42": {ProfileID: "42", Name: "Claimme", Class: "Magier", MainSpec: "Arkan", SecondSpec: "Frost", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=42"},
	}}
	srv, bdb := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: feed})
	client := srv.Client()
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	seedAccount(t, bdb, "member", "pw", validation.RoleGuest)
	adminToken := login(t, client, srv, "admin", "pw")
	memberToken := login(t, client, srv, "member", "pw")

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/characters/claim-requests", memberToken, map[string]any{"profile_id": "42"})
	if status != http.StatusCreated {
		t.Fatalf("claim request: status %d body %s", status, body)
	}
	var reqOut struct {
		Data registration.Request `json:"data"`
	}
	if err := json.Unmarshal(body, &reqOut); err != nil {
		t.Fatalf("decode claim request: %v", err)
	}

	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/registration-requests/%d/code", srv.URL, reqOut.Data.ID), adminToken, nil)
	if status != http.StatusCreated {
		t.Fatalf("claim code: status %d body %s", status, body)
	}
	var codeOut struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &codeOut); err != nil {
		t.Fatalf("decode code: %v", err)
	}

	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/characters/claim-with-code", memberToken, map[string]any{"code": codeOut.Data.Code})
	if status != http.StatusOK {
		t.Fatalf("claim redeem: status %d body %s", status, body)
	}
	var charOut struct {
		Data character.Character `json:"data"`
	}
	if err := json.Unmarshal(body, &charOut); err != nil {
		t.Fatalf("decode character: %v", err)
	}
	if charOut.Data.Name != "Claimme" || charOut.Data.Class != "Magier" || charOut.Data.MainSpec != "Arkan" || charOut.Data.SecondSpec != "Frost" {
		t.Fatalf("unexpected claimed character: %s", body)
	}
}
