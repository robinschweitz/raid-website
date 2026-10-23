package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"raid-website/internal/auth"
	v1 "raid-website/internal/httpapi/v1"
	"raid-website/internal/player"
)

// TestReqAuth013_EndpointInvalidatesChallengeAtLimit covers SC-011 at the
// endpoint: four wrong codes answer invalid_mfa_code, the fifth answers
// mfa_challenge_invalid, and the challenge stays closed afterwards.
func TestReqAuth013_EndpointInvalidatesChallengeAtLimit(t *testing.T) {
	ctx := context.Background()
	bdb, svc := middlewareAuthDB(t)
	if err := svc.BootstrapAdmin(ctx, "owner", "valid-password"); err != nil {
		t.Fatal(err)
	}
	owner, err := player.NewRepo(bdb.DB).FindByUsername(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := auth.NewRepo(bdb.DB).UpsertMFASettings(ctx, &auth.MFASettings{PlayerID: owner.ID, TOTPSecret: "JBSWY3DPEHPK3PXP", TOTPEnabledAt: &now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	v1.NewAPI(v1.Deps{Auth: svc}).Register(mux)
	handler := authenticate(svc)(mux)
	post := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	w := post("/api/v1/auth/login", `{"username":"owner","password":"valid-password"}`)
	var login struct {
		Data struct {
			Challenge string `json:"challenge"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil || login.Data.Challenge == "" {
		t.Fatalf("login = %d %s; want an MFA challenge", w.Code, w.Body.String())
	}
	// Not a code: never matches the TOTP of any time step.
	verify := fmt.Sprintf(`{"challenge":%q,"code":"not-a-code"}`, login.Data.Challenge)
	for i := 0; i < 4; i++ {
		assertAuthError(t, post("/api/v1/auth/mfa/verify", verify), http.StatusUnauthorized, "invalid_mfa_code")
	}
	assertAuthError(t, post("/api/v1/auth/mfa/verify", verify), http.StatusUnauthorized, "mfa_challenge_invalid")
	assertAuthError(t, post("/api/v1/auth/mfa/verify", verify), http.StatusUnauthorized, "mfa_challenge_invalid")
}
