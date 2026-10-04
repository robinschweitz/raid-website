package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

func TestRaidSeriesAPIConfigurationAndPermissions(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, bdb, "seriesadmin", "password", validation.RoleAdmin)
	seedAccount(t, bdb, "seriesmember", "password", validation.RoleMember)
	seedAccount(t, bdb, "seriesassistant", "password", validation.RoleRaidassistent)
	admin := login(t, client, srv, "seriesadmin", "password")
	member := login(t, client, srv, "seriesmember", "password")
	assistant := login(t, client, srv, "seriesassistant", "password")
	url := srv.URL + "/api/v1/raid-series"
	status, body := do(t, client, http.MethodGet, url, member, nil)
	if status != http.StatusOK || string(body) != "{\"data\":[]}\n" {
		t.Fatalf("empty series: %d %s", status, body)
	}
	status, _ = do(t, client, http.MethodGet, url, "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous read: %d", status)
	}
	configured := map[string]any{"name": "Explicit series", "raid_leader_id": adminID, "weekdays": []int{1, 3}, "interval_weeks": 2, "start_time": "20:00", "timezone": "Europe/Berlin"}
	for _, token := range []string{member, assistant} {
		status, _ = do(t, client, http.MethodPost, url, token, configured)
		if status != http.StatusForbidden {
			t.Fatalf("nonadmin create: %d", status)
		}
		status, _ = do(t, client, http.MethodPatch, url+"/1", token, map[string]any{"name": "changed"})
		if status != http.StatusForbidden {
			t.Fatalf("nonadmin patch: %d", status)
		}
	}
	for _, field := range []string{"name", "raid_leader_id", "weekdays", "interval_weeks", "start_time", "timezone"} {
		request := map[string]any{}
		for key, value := range configured {
			if key != field {
				request[key] = value
			}
		}
		status, body = do(t, client, http.MethodPost, url, admin, request)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("missing %s: %d %s", field, status, body)
		}
	}
	status, body = do(t, client, http.MethodPost, url, admin, configured)
	if status != http.StatusCreated {
		t.Fatalf("create series: %d %s", status, body)
	}
	var created struct {
		Data raidseries.Series `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	seriesURL := fmt.Sprintf("%s/%d", url, created.Data.ID)
	for _, invalid := range []map[string]any{
		{"weekdays": []int{}}, {"weekdays": []int{7}}, {"weekdays": []int{1, 1}},
		{"interval_weeks": 0}, {"interval_weeks": -1}, {"start_time": "24:00"},
		{"timezone": "Local"}, {"timezone": ""}, {"timezone": "Not/AZone"},
		{"raid_leader_id": 999999}, {"name": " "},
	} {
		status, body = do(t, client, http.MethodPatch, seriesURL, admin, invalid)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("invalid patch %+v: %d %s", invalid, status, body)
		}
	}
	status, body = do(t, client, http.MethodPatch, seriesURL, admin, map[string]any{"name": "Renamed", "interval_weeks": 3})
	if status != http.StatusOK {
		t.Fatalf("patch series: %d %s", status, body)
	}
	var updated struct {
		Data raidseries.Series `json:"data"`
	}
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Data.Name != "Renamed" || updated.Data.IntervalWeeks != 3 || updated.Data.Timezone != created.Data.Timezone || len(updated.Data.Weekdays) != 2 || !updated.Data.CreatedAt.Equal(created.Data.CreatedAt.Truncate(time.Microsecond)) {
		t.Fatalf("patch discarded config: %+v", updated.Data)
	}
	status, _ = do(t, client, http.MethodPatch, seriesURL, admin, map[string]any{"unexpected": true})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", status)
	}
	status, _ = do(t, client, http.MethodPatch, url+"/999999", admin, map[string]any{"name": "missing"})
	if status != http.StatusNotFound {
		t.Fatalf("missing series: %d", status)
	}
}

func TestArchiveAPIPrerequisitesHistoryAndRetry(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	actorID := seedAccount(t, bdb, "archiveassistant", "password", validation.RoleRaidassistent)
	seedAccount(t, bdb, "archivemember", "password", validation.RoleMember)
	assistant := login(t, client, srv, "archiveassistant", "password")
	member := login(t, client, srv, "archivemember", "password")
	ctx := context.Background()
	now := time.Now().UTC()
	series := &raidseries.Series{Name: "Successor name", RaidLeaderID: actorID, Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin"}
	if err := raidseries.NewService(raidseries.NewRepo(bdb)).Save(ctx, series); err != nil {
		t.Fatal(err)
	}
	rd := &raid.Raid{Name: "Concrete legacy raid", RaidLeaderID: actorID, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	raids := raid.NewRepo(bdb)
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("%s/api/v1/raids/%d", srv.URL, rd.ID)
	status, body := do(t, client, http.MethodPost, url+"/archive", member, nil)
	if status != http.StatusForbidden {
		t.Fatalf("member archive: %d", status)
	}
	status, _ = do(t, client, http.MethodPatch, url, member, map[string]any{"series_id": series.ID})
	if status != http.StatusForbidden {
		t.Fatalf("member attach: %d", status)
	}
	status, body = do(t, client, http.MethodPost, url+"/archive", assistant, nil)
	assertArchiveCode(t, status, body, "series_required")
	status, body = do(t, client, http.MethodPatch, url, assistant, map[string]any{"series_id": series.ID})
	if status != http.StatusOK {
		t.Fatalf("attach: %d %s", status, body)
	}
	status, body = do(t, client, http.MethodPost, url+"/archive", assistant, nil)
	assertArchiveCode(t, status, body, "scheduled_start_required")
	stored, err := raids.Get(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CompletedAt != nil || stored.ArchivedAt != nil || stored.Status != validation.RaidStatusOpen || stored.Name != rd.Name {
		t.Fatalf("failed archive mutated legacy raid: %+v", stored)
	}
	start := now.AddDate(0, 0, -7)
	status, body = do(t, client, http.MethodPatch, url, assistant, map[string]any{"scheduled_start": start.Format(time.RFC3339)})
	if status != http.StatusOK {
		t.Fatalf("configure start: %d %s", status, body)
	}
	status, body = do(t, client, http.MethodPost, url+"/archive", assistant, nil)
	if status != http.StatusOK {
		t.Fatalf("archive: %d %s", status, body)
	}
	var result struct {
		Data     raid.Raid  `json:"data"`
		NextRaid *raid.Raid `json:"next_raid"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Status != validation.RaidStatusCompleted || result.Data.CompletedAt == nil || result.Data.ArchivedAt == nil || result.NextRaid == nil || result.NextRaid.Name != series.Name || result.NextRaid.PreviousRaidID == nil || *result.NextRaid.PreviousRaidID != rd.ID || result.NextRaid.ScheduledEnd != nil {
		t.Fatalf("archive envelope: %s", body)
	}
	nextID := result.NextRaid.ID
	status, body = do(t, client, http.MethodPost, url+"/archive", assistant, nil)
	if status != http.StatusOK {
		t.Fatalf("retry: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.NextRaid == nil || result.NextRaid.ID != nextID {
		t.Fatalf("retry duplicated successor: %s", body)
	}
	status, body = do(t, client, http.MethodGet, srv.URL+"/api/v1/raids", member, nil)
	var list struct {
		Data []raid.Raid `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || len(list.Data) != 2 {
		t.Fatalf("history excluded from list: %d %s", status, body)
	}
	status, _ = do(t, client, http.MethodPatch, url, assistant, map[string]any{"series_id": series.ID})
	if status != http.StatusConflict {
		t.Fatalf("finalized series overwritten: %d", status)
	}
	status, _ = do(t, client, http.MethodPost, url+"/complete", assistant, nil)
	if status == http.StatusOK {
		t.Fatal("separate complete action still exposed")
	}
	cancelled := &raid.Raid{Name: "Cancelled", RaidLeaderID: actorID, Status: validation.RaidStatusCancelled, CreatedAt: now, UpdatedAt: now}
	if err := raids.Create(ctx, cancelled); err != nil {
		t.Fatal(err)
	}
	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/archive", srv.URL, cancelled.ID), assistant, nil)
	assertArchiveCode(t, status, body, "raid_cancelled")
}

func assertArchiveCode(t *testing.T, status int, body []byte, want string) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusConflict || envelope.Error.Code != want {
		t.Fatalf("want conflict %s got %d %s", want, status, body)
	}
}
