package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"raid-website/internal/auth"
	"raid-website/internal/httpapi"
	"raid-website/internal/validation"
)

func TestReqAuth015_HTTPLoginAfterMigration(t *testing.T) {
	_, db := setupServer(t)
	id := seedAccount(t, db, "member-015", "member-015-password", validation.RoleMember)
	enableTestTOTP(t, db, id)
	e, err := auth.NewTOTPEncryption([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := httpapi.New(db, httpapi.Options{TOTPEncryption: e})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	status, body := do(t, srv.Client(), http.MethodPost, srv.URL+"/api/v1/auth/login", "", map[string]any{"username": "member-015", "password": "member-015-password"})
	if status != http.StatusOK {
		t.Fatalf("password step=%d %s", status, body)
	}
	var login struct {
		Data struct {
			Challenge string `json:"challenge"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &login); err != nil {
		t.Fatal(err)
	}
	status, body = do(t, srv.Client(), http.MethodPost, srv.URL+"/api/v1/auth/mfa/verify", "", map[string]any{"challenge": login.Data.Challenge, "code": testTOTPCode(t, testTOTPSecret)})
	if status != http.StatusOK {
		t.Fatalf("TOTP step=%d %s", status, body)
	}
}
