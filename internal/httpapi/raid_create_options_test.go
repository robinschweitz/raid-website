package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

// FR-10.1, FR-10.17, FR-16.11 (ADR 114): the new-raid form sets all raid leaders,
// the EP pool, and the EP award; a member may not create raids.
func TestReqRaid_CreateWithLeadersAndEPPool(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	adminID := seedAccount(t, bdb, "createadmin", "password", validation.RoleAdmin)
	otherID := seedAccount(t, bdb, "createother", "password", validation.RoleMember)
	admin := login(t, client, srv, "createadmin", "password")
	member := login(t, client, srv, "createother", "password")
	if status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/ep-pools", admin, map[string]any{"name": "Ulduar"}); status != http.StatusCreated {
		t.Fatalf("create pool: %d %s", status, body)
	}
	start := time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339)
	base := func() map[string]any {
		return map[string]any{"name": "Optionen", "leader_ids": []int64{otherID, adminID}, "ep_pool_id": 2, "ep_award": 25, "scheduled_start": start}
	}

	if status, _ := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", member, base()); status != http.StatusForbidden {
		t.Fatalf("member create: %d", status)
	}
	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", admin, base())
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	var created struct {
		Data struct {
			ID           int64  `json:"id"`
			RaidLeaderID int64  `json:"raid_leader_id"`
			EPPoolID     *int64 `json:"ep_pool_id"`
			EPAward      int    `json:"ep_award"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.RaidLeaderID != otherID || created.Data.EPPoolID == nil || *created.Data.EPPoolID != 2 || created.Data.EPAward != 25 {
		t.Fatalf("unexpected raid: %s", body)
	}
	status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/raids/"+strconv.FormatInt(created.Data.ID, 10)+"/leaders", "", nil)
	var leaders struct{ Data []int64 }
	if err := json.Unmarshal(body, &leaders); err != nil || status != http.StatusOK || len(leaders.Data) != 2 || leaders.Data[0] != otherID || leaders.Data[1] != adminID {
		t.Fatalf("leaders: %d %s", status, body)
	}

	noPool := base()
	noPool["ep_pool_id"] = 0
	noPool["scheduled_start"] = time.Now().UTC().Add(96 * time.Hour).Format(time.RFC3339)
	status, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", admin, noPool)
	if status != http.StatusCreated || json.Unmarshal(body, &created) != nil || created.Data.EPPoolID != nil {
		t.Fatalf("raid without pool: %d %s", status, body)
	}

	for name, change := range map[string]map[string]any{
		"award too high":   {"ep_award": 101},
		"unknown pool":     {"ep_pool_id": 99},
		"repeated leader":  {"leader_ids": []int64{adminID, adminID}},
		"unknown leader":   {"leader_ids": []int64{9999}},
		"empty leader set": {"leader_ids": []int64{}, "raid_leader_id": 0},
	} {
		fields := base()
		for key, value := range change {
			fields[key] = value
		}
		if status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raids", admin, fields); status < 400 || status >= 500 {
			t.Fatalf("%s: %d %s", name, status, body)
		}
	}
	var raidCount int
	if err := bdb.NewRaw("SELECT COUNT(*) FROM raids").Scan(ctx, &raidCount); err != nil || raidCount != 2 {
		t.Fatalf("rejected creates stored raids: %d %v", raidCount, err)
	}
}

// FR-9.1 (ADR 114): every series uses Europe/Berlin; another timezone is rejected.
func TestReqRaid_SeriesTimezoneIsFixed(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, bdb, "tzadmin", "password", validation.RoleAdmin)
	admin := login(t, client, srv, "tzadmin", "password")
	config := map[string]any{"name": "Ohne Zone", "leader_ids": []int64{adminID}, "weekdays": []int{3}, "interval_weeks": 1, "start_time": "20:00", "catalog_instance_id": "631", "raid_size": 10}
	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raid-series", admin, config)
	var created struct{ Data raidseries.Series }
	if status != http.StatusCreated || json.Unmarshal(body, &created) != nil || created.Data.Timezone != raidseries.Timezone {
		t.Fatalf("series without timezone: %d %s", status, body)
	}
	config["timezone"] = "UTC"
	if status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/raid-series", admin, config); status != http.StatusUnprocessableEntity || errorCode(t, body) != "invalid_series" {
		t.Fatalf("series with UTC: %d %s", status, body)
	}
}
