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

// TestReqAuth012_UnknownAndWrongPasswordAnswerAlike covers SC-009 at the
// endpoint: an unknown username and a wrong password get the same status and
// the same body.
func TestReqAuth012_UnknownAndWrongPasswordAnswerAlike(t *testing.T) {
	ctx := context.Background()
	_, svc := middlewareAuthDB(t)
	if err := svc.BootstrapAdmin(ctx, "owner", "valid-password"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	v1.NewAPI(v1.Deps{Auth: svc}).Register(mux)
	handler := authenticate(svc)(mux)
	login := func(username string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"username":%q,"password":"wrong-password"}`, username)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	unknown, wrong := login("nobody"), login("owner")
	assertAuthError(t, unknown, http.StatusUnauthorized, "invalid_credentials")
	if unknown.Code != wrong.Code || unknown.Body.String() != wrong.Body.String() {
		t.Fatalf("unknown = %d %s; wrong password = %d %s; want identical answers", unknown.Code, unknown.Body.String(), wrong.Code, wrong.Body.String())
	}
}
