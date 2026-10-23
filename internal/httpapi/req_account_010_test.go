package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"raid-website/internal/httpapi"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

// registrationCode creates a request for the profile and returns its code.
func registrationCode(t *testing.T, srv interface{ Client() *http.Client }, baseURL, adminToken, profileID string) string {
	t.Helper()
	client := srv.Client()
	status, body := do(t, client, http.MethodPost, baseURL+"/api/v1/auth/registration-requests", "", map[string]any{"profile_id": profileID})
	if status != http.StatusCreated {
		t.Fatalf("create request: status %d body %s", status, body)
	}
	var req struct {
		Data registration.Request `json:"data"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/registration-requests/%d/code", baseURL, req.Data.ID), adminToken, nil)
	if status != http.StatusCreated {
		t.Fatalf("create code: status %d body %s", status, body)
	}
	var code struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &code); err != nil {
		t.Fatalf("decode code: %v", err)
	}
	return code.Data.Code
}

func req010Server(t *testing.T) (string, *http.Client, string, func(code, username string) (int, []byte), func(profileID string) string) {
	t.Helper()
	feed := fakeRegistrationFeed{characters: map[string]registration.ExternalCharacter{
		"1": {ProfileID: "1", Name: "Firsthero", Class: "Magier", MainSpec: "Arkan", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=1"},
		"2": {ProfileID: "2", Name: "Secondhero", Class: "Krieger", MainSpec: "Waffen", Level: 80, ProfileURL: "https://db.rising-gods.de/?profile=load&id=2"},
	}}
	srv, bdb := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: feed})
	client := srv.Client()
	seedAccount(t, bdb, "admin", "pw", validation.RoleAdmin)
	adminToken := login(t, client, srv, "admin", "pw")
	redeem := func(code, username string) (int, []byte) {
		return do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/register-with-code", "", map[string]any{"code": code, "username": username, "password": "valid-secret-1"})
	}
	newCode := func(profileID string) string {
		return registrationCode(t, srv, srv.URL, adminToken, profileID)
	}
	return srv.URL, client, adminToken, redeem, newCode
}

func TestReqAccount010_AC01(t *testing.T) {
	_, _, _, redeem, _ := req010Server(t)
	statusTaken, bodyTaken := redeem("AAAA-BBBB-CCCC", "admin")
	statusFree, bodyFree := redeem("AAAA-BBBB-CCCC", "nobodyuses")
	if statusTaken != http.StatusBadRequest || statusFree != statusTaken || string(bodyTaken) != string(bodyFree) {
		t.Fatalf("taken: %d %s, free: %d %s", statusTaken, bodyTaken, statusFree, bodyFree)
	}
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(bodyTaken, &out); err != nil || out.Error.Code != "registration_code_invalid" {
		t.Fatalf("unexpected body %s", bodyTaken)
	}
}

func TestReqAccount010_AC02(t *testing.T) {
	_, _, _, redeem, newCode := req010Server(t)
	code := newCode("1")
	status, body := redeem(code, "admin")
	if status != http.StatusConflict || !strings.Contains(string(body), "username_taken") {
		t.Fatalf("taken username: %d %s", status, body)
	}
	status, body = redeem(code, "freshname")
	if status != http.StatusOK {
		t.Fatalf("code must stay unused after username_taken: %d %s", status, body)
	}
}

func TestReqAccount010_AC03(t *testing.T) {
	_, _, _, redeem, newCode := req010Server(t)
	codes := []string{newCode("1"), newCode("2")}
	statuses := make([]int, len(codes))
	var wg sync.WaitGroup
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i], _ = redeem(codes[i], "samename")
		}()
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, s := range statuses {
		switch s {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("want one success and one 409, got %v", statuses)
	}
}
