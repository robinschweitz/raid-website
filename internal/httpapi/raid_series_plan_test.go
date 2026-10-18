package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/validation"
)

// ADR 078: raids of a series can be planned ahead, by date or by a rolling
// window, and their signups open right away.
func TestPlanSeriesRaidsAhead(t *testing.T) {
	srv, db := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, db, "plan-admin", "password", validation.RoleAdmin)
	seedAccount(t, db, "plan-assistant", "password", validation.RoleRaidassistent)
	seedAccount(t, db, "plan-member", "password", validation.RoleMember)
	admin := login(t, client, srv, "plan-admin", "password")
	assistant := login(t, client, srv, "plan-assistant", "password")
	member := login(t, client, srv, "plan-member", "password")
	call := func(token, method, path string, body any, want int) []byte {
		t.Helper()
		status, out := do(t, client, method, srv.URL+path, token, body)
		if status != want {
			t.Fatalf("%s %s: want %d got %d: %s", method, path, want, status, out)
		}
		return out
	}
	type raidRow struct {
		ID             int64     `json:"id"`
		Name           string    `json:"name"`
		Status         string    `json:"status"`
		SeriesID       *int64    `json:"series_id"`
		ScheduledStart time.Time `json:"scheduled_start"`
	}
	seriesRaids := func(seriesID int64) []raidRow {
		t.Helper()
		var out struct{ Data []raidRow }
		if err := json.Unmarshal(call(admin, http.MethodGet, "/api/v1/raids?include_archived=true", nil, http.StatusOK), &out); err != nil {
			t.Fatal(err)
		}
		rows := []raidRow{}
		for _, row := range out.Data {
			if row.SeriesID != nil && *row.SeriesID == seriesID {
				rows = append(rows, row)
			}
		}
		return rows
	}
	var created struct{ Data struct{ ID int64 } }
	body := call(admin, http.MethodPost, "/api/v1/raid-series", map[string]any{"name": "Donnerstag", "name_template": "ICC-{number}", "next_raid_number": 1, "raid_leader_id": adminID, "weekdays": []int{4}, "interval_weeks": 1, "start_time": "20:00", "timezone": "Europe/Berlin"}, http.StatusCreated)
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	seriesID := created.Data.ID
	plan := fmt.Sprintf("/api/v1/raid-series/%d/plan", seriesID)
	until := time.Now().UTC().AddDate(0, 0, 21).Format("2006-01-02")

	call(member, http.MethodPost, plan, map[string]any{"until": until}, http.StatusForbidden)
	call(assistant, http.MethodPost, plan, map[string]any{"until": "2020-01-01"}, http.StatusBadRequest)
	call(assistant, http.MethodPost, plan, map[string]any{"until": "soon"}, http.StatusBadRequest)

	var planned struct{ Data []raidRow }
	if err := json.Unmarshal(call(assistant, http.MethodPost, plan, map[string]any{"until": until}, http.StatusOK), &planned); err != nil {
		t.Fatal(err)
	}
	if len(planned.Data) < 3 || len(planned.Data) > 4 {
		t.Fatalf("three weeks of thursdays: %+v", planned.Data)
	}
	for i, row := range planned.Data {
		if row.Status != "PLANNED" || row.Name != fmt.Sprintf("ICC-%d", i+1) || row.ScheduledStart.In(mustLocation(t, "Europe/Berlin")).Weekday() != time.Thursday {
			t.Fatalf("planned raid %d: %+v", i, row)
		}
		var signups struct {
			Data struct {
				IsOpen bool `json:"is_open"`
			}
		}
		if err := json.Unmarshal(call(member, http.MethodGet, fmt.Sprintf("/api/v1/raids/%d/signups", row.ID), nil, http.StatusOK), &signups); err != nil {
			t.Fatal(err)
		}
		if !signups.Data.IsOpen {
			t.Fatalf("signups of planned raid %d are not open", row.ID)
		}
	}
	// Planning again is harmless.
	if err := json.Unmarshal(call(assistant, http.MethodPost, plan, map[string]any{"until": until}, http.StatusOK), &planned); err != nil || len(planned.Data) != 0 {
		t.Fatalf("second plan created %+v %v", planned.Data, err)
	}
	// A rolling window keeps six weeks planned as soon as the series is saved.
	var saved struct {
		Data struct {
			NextRaidNumber int `json:"next_raid_number"`
		}
	}
	if err := json.Unmarshal(call(admin, http.MethodPatch, fmt.Sprintf("/api/v1/raid-series/%d", seriesID), map[string]any{"plan_weeks_ahead": 6}, http.StatusOK), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Data.NextRaidNumber != 7 {
		t.Fatalf("saved series must show the counter after planning: %d", saved.Data.NextRaidNumber)
	}
	if rows := seriesRaids(seriesID); len(rows) != 6 {
		t.Fatalf("rolling window: want 6 raids, got %d: %+v", len(rows), rows)
	}
	call(admin, http.MethodPatch, fmt.Sprintf("/api/v1/raid-series/%d", seriesID), map[string]any{"plan_weeks_ahead": -1}, http.StatusUnprocessableEntity)

	var count int
	if err := db.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("entity_type = 'raid' AND action = 'plan'").Scan(t.Context(), &count); err != nil || count != 6 {
		t.Fatalf("plan audit entries: %d %v", count, err)
	}
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}
