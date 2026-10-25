package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"raid-website/internal/player"
	"raid-website/internal/validation"
)

func TestReqAccount015_AC01(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, bdb, "admin-015a", "admin-015a-password", validation.RoleAdmin)
	memberID := seedAccount(t, bdb, "member-015a", "member-015a-password", validation.RoleMember)
	adminToken := login(t, client, srv, "admin-015a", "admin-015a-password")
	memberToken := login(t, client, srv, "member-015a", "member-015a-password")
	url := fmt.Sprintf("%s/api/v1/players/%d/account", srv.URL, adminID)
	demote := map[string]any{"username": "admin-015a", "password": "", "role": "MEMBER"}

	if status, _ := do(t, client, http.MethodPut, url, memberToken, demote); status != http.StatusForbidden {
		t.Fatalf("member status = %d", status)
	}
	if status, _ := do(t, client, http.MethodPut, url, "", demote); status != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", status)
	}
	status, body := do(t, client, http.MethodPut, url, adminToken, demote)
	if status != http.StatusConflict || errorCode(t, body) != "last_admin" {
		t.Fatalf("demote status = %d: %s", status, body)
	}
	got, err := player.NewRepo(bdb).Get(context.Background(), adminID)
	if err != nil || got.Role != validation.RoleAdmin {
		t.Fatalf("role changed: %+v, %v", got, err)
	}
	if status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", adminToken, nil); status != http.StatusOK {
		t.Fatalf("rejected change ended the session: %d", status)
	}
	_ = memberID
}

func TestReqAccount015_AC02(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin-015b", "admin-015b-password", validation.RoleAdmin)
	otherID := seedAccount(t, bdb, "admin-015c", "admin-015c-password", validation.RoleAdmin)
	adminToken := login(t, client, srv, "admin-015b", "admin-015b-password")
	url := fmt.Sprintf("%s/api/v1/players/%d/account", srv.URL, otherID)
	status, body := do(t, client, http.MethodPut, url, adminToken, map[string]any{"username": "admin-015c", "password": "", "role": "MEMBER"})
	if status != http.StatusOK {
		t.Fatalf("demote with two admins status = %d: %s", status, body)
	}
}

func TestReqAccount015_AC03(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	sourceID := seedAccount(t, bdb, "admin-015d", "admin-015d-password", validation.RoleAdmin)
	targetID := seedAccount(t, bdb, "member-015d", "member-015d-password", validation.RoleMember)
	adminToken := login(t, client, srv, "admin-015d", "admin-015d-password")
	memberToken := login(t, client, srv, "member-015d", "member-015d-password")
	url := fmt.Sprintf("%s/api/v1/players/%d/merge", srv.URL, targetID)
	req := map[string]any{"source_player_id": sourceID, "reason": "Test"}

	if status, _ := do(t, client, http.MethodPost, url, memberToken, req); status != http.StatusForbidden {
		t.Fatalf("member merge status = %d", status)
	}
	status, body := do(t, client, http.MethodPost, url, adminToken, req)
	if status != http.StatusConflict || errorCode(t, body) != "last_admin" {
		t.Fatalf("merge status = %d: %s", status, body)
	}
	if _, err := player.NewRepo(bdb).Get(context.Background(), sourceID); err != nil {
		t.Fatalf("source was deleted: %v", err)
	}

	// An active Admin target keeps administration alive, so the merge passes.
	secondID := seedAccount(t, bdb, "admin-015e", "admin-015e-password", validation.RoleAdmin)
	url = fmt.Sprintf("%s/api/v1/players/%d/merge", srv.URL, secondID)
	if status, body := do(t, client, http.MethodPost, url, adminToken, req); status != http.StatusNoContent {
		t.Fatalf("merge into admin status = %d: %s", status, body)
	}
}
