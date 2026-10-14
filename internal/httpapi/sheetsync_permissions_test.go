package httpapi_test

import (
	"net/http"
	"raid-website/internal/validation"
	"testing"
)

func TestSyncPermissions(t *testing.T) {
	srv, b := setupServer(t)
	for _, role := range []validation.Role{validation.RoleGuest, validation.RoleMember, validation.RoleRaidassistent, validation.RoleRaidleader, validation.RoleAdmin} {
		user := "pr13-" + string(role)
		seedAccount(t, b, user, "test-password", role)
		token := login(t, srv.Client(), srv, user, "test-password")
		for _, path := range []string{"", "/config", "/run", "/preflight"} {
			method := http.MethodGet
			if path == "/run" || path == "/preflight" {
				method = http.MethodPost
			}
			url := srv.URL + "/api/v1/sheet-sync"
			url += path
			status, body := do(t, srv.Client(), method, url, token, map[string]any{})
			want := http.StatusForbidden
			if role == validation.RoleAdmin {
				want = http.StatusOK
				if method == http.MethodPost {
					want = http.StatusConflict
				}
			}
			if status != want {
				t.Fatalf("%s %s got %d want %d %s", role, method, status, want, body)
			}
			t.Logf("%s %s: HTTP %d", role, method, status)
		}
	}
	for _, path := range []string{"", "/run", "/preflight"} {
		method := http.MethodGet
		if path != "" {
			method = http.MethodPost
		}
		url := srv.URL + "/api/v1/sheet-sync" + path
		status, body := do(t, srv.Client(), method, url, "", nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("anonymous %s: %d %s", method, status, body)
		}
	}
}

func TestSyncConfigMutationAdminOnly(t *testing.T) {
	srv, b := setupServer(t)
	payload := map[string]any{
		"enabled":   false,
		"automatic": false,
		"sheet_id":  "10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8",
		"interval":  "10m",
		"reason":    "",
	}
	for _, role := range []validation.Role{validation.RoleMember, validation.RoleRaidassistent, validation.RoleAdmin} {
		user := "settings-" + string(role)
		seedAccount(t, b, user, "test-password", role)
		token := login(t, srv.Client(), srv, user, "test-password")
		status, body := do(t, srv.Client(), http.MethodPut, srv.URL + "/api/v1/sheet-sync/config", token, payload)
		want := http.StatusForbidden
		if role == validation.RoleAdmin {
			want = http.StatusOK
		}
		if status != want {
			t.Fatalf("settings %s got %d want %d: %s", role, status, want, body)
		}
	}
	if status, body := do(t, srv.Client(), http.MethodPut, srv.URL + "/api/v1/sheet-sync/config", "", payload); status != http.StatusUnauthorized {
		t.Fatalf("anonymous update got %d: %s", status, body)
	}
}
