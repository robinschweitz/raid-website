package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/validation"
)

func TestReqAuth018_HTTPIdleSession(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "member-018", "member-018-password", validation.RoleMember)
	token := login(t, client, srv, "member-018", "member-018-password")
	if _, err := bdb.DB.ExecContext(context.Background(), "UPDATE sessions SET last_seen_at = ?", time.Now().UTC().Add(-7*24*time.Hour-time.Minute)); err != nil {
		t.Fatal(err)
	}
	status, body := do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", token, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("idle session status = %d, body = %s", status, body)
	}
}
