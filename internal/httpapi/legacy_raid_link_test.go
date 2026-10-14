package httpapi_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestLegacyRaidLinkAPIIsAdminOnlyAndImmutable(t *testing.T) {
	srv, database := setupServer(t)
	adminID := seedAccount(t, database, "legacy-link-admin", "password", validation.RoleAdmin)
	seedAccount(t, database, "legacy-link-ra", "password", validation.RoleRaidassistent)
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Legacy ICC", RaidLeaderID: adminID, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(database).Create(context.Background(), rd); err != nil {
		t.Fatal(err)
	}
	client := srv.Client()
	adminToken := login(t, client, srv, "legacy-link-admin", "password")
	raToken := login(t, client, srv, "legacy-link-ra", "password")
	url := srv.URL + "/api/v1/raids/" + strconv.FormatInt(rd.ID, 10) + "/legacy-link"
	body := map[string]any{"source": "google_sheets", "sheet_id": "sheet-id", "raid_key": "ICC25-01", "reason": "Historischen Raid verknuepfen"}

	if status, _ := do(t, client, http.MethodPost, url, raToken, body); status != http.StatusForbidden {
		t.Fatalf("raid assistant POST status = %d, want %d", status, http.StatusForbidden)
	}
	if status, response := do(t, client, http.MethodPost, url, adminToken, body); status != http.StatusCreated {
		t.Fatalf("admin POST status = %d, body = %s", status, response)
	}
	if status, response := do(t, client, http.MethodGet, url, adminToken, nil); status != http.StatusOK {
		t.Fatalf("admin GET status = %d, body = %s", status, response)
	}
	if status, _ := do(t, client, http.MethodPost, url, adminToken, body); status != http.StatusConflict {
		t.Fatalf("duplicate POST status = %d, want %d", status, http.StatusConflict)
	}
	if status, _ := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids/999999/legacy-link", adminToken, body); status != http.StatusNotFound {
		t.Fatalf("unknown raid POST status = %d, want %d", status, http.StatusNotFound)
	}
	if status, _ := do(t, client, http.MethodPost, url, adminToken, map[string]any{"source": "google_sheets", "sheet_id": "other", "raid_key": "ICC25-02", "reason": "x", "unexpected": true}); status != http.StatusBadRequest {
		t.Fatalf("unknown field POST status = %d, want %d", status, http.StatusBadRequest)
	}
}
