package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "raid-website/internal/httpapi/v1"
)

// TestReqAuth011_LoginEndpointRejectsThirtyFirstFailure covers SC-043 through
// the login endpoint behind nginx: 30 failed logins for 30 usernames from one
// X-Real-IP block the 31st, even with correct credentials, while another
// address still logs in.
func TestReqAuth011_LoginEndpointRejectsThirtyFirstFailure(t *testing.T) {
	ctx := context.Background()
	_, svc := middlewareAuthDB(t)
	if err := svc.BootstrapAdmin(ctx, "owner", "valid-password"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	v1.NewAPI(v1.Deps{Auth: svc, TrustProxyHeaders: true}).Register(mux)
	handler := authenticate(svc)(mux)
	login := func(username, password, address string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		r.Header.Set("X-Real-IP", address)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	for i := 0; i < 30; i++ {
		w := login(fmt.Sprintf("guess-%02d", i), "wrong-password", "203.0.113.7")
		assertAuthError(t, w, http.StatusUnauthorized, "invalid_credentials")
	}
	assertAuthError(t, login("owner", "valid-password", "203.0.113.7"), http.StatusTooManyRequests, "login_rate_limited")

	if w := login("owner", "valid-password", "198.51.100.4"); w.Code != http.StatusOK {
		t.Fatalf("login from another address = %d %s, want 200", w.Code, w.Body.String())
	}
}
