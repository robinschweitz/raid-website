package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/validation"
)

// ADR 112: a series lists several raid leaders and passes them to every raid made from it.
func TestSeriesRaidLeaders(t *testing.T) {
	srv, db := setupServer(t)
	client := srv.Client()
	adminID := seedAccount(t, db, "series-admin", "password", validation.RoleAdmin)
	coID := seedAccount(t, db, "series-co", "password", validation.RoleRaidleader)
	thirdID := seedAccount(t, db, "series-third", "password", validation.RoleRaidassistent)
	admin := login(t, client, srv, "series-admin", "password")
	assistant := login(t, client, srv, "series-third", "password")
	call := func(token, method, path string, body any, want int) []byte {
		t.Helper()
		status, out := do(t, client, method, srv.URL+path, token, body)
		if status != want {
			t.Fatalf("%s %s: want %d got %d: %s", method, path, want, status, out)
		}
		return out
	}
	leadersOf := func(path string) []int64 {
		t.Helper()
		var out struct{ Data []int64 }
		if err := json.Unmarshal(call(admin, http.MethodGet, path, nil, http.StatusOK), &out); err != nil {
			t.Fatal(err)
		}
		return out.Data
	}
	base := map[string]any{"name": "Donnerstag", "weekdays": []int{4}, "interval_weeks": 1, "start_time": "20:00", "timezone": "Europe/Berlin"}
	with := func(extra map[string]any) map[string]any {
		body := map[string]any{}
		for k, v := range base {
			body[k] = v
		}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}

	call(assistant, http.MethodPost, "/api/v1/raid-series", with(map[string]any{"leader_ids": []int64{thirdID}}), http.StatusForbidden)
	for _, bad := range [][]int64{{}, {adminID, adminID}} {
		call(admin, http.MethodPost, "/api/v1/raid-series", with(map[string]any{"leader_ids": bad}), http.StatusBadRequest)
	}
	call(admin, http.MethodPost, "/api/v1/raid-series", with(map[string]any{"leader_ids": []int64{adminID, 99999}}), http.StatusUnprocessableEntity)

	var created struct {
		Data struct {
			ID           int64   `json:"id"`
			RaidLeaderID int64   `json:"raid_leader_id"`
			LeaderIDs    []int64 `json:"leader_ids"`
		}
	}
	if err := json.Unmarshal(call(admin, http.MethodPost, "/api/v1/raid-series", with(map[string]any{"leader_ids": []int64{adminID, coID}}), http.StatusCreated), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.RaidLeaderID != adminID || len(created.Data.LeaderIDs) != 2 || created.Data.LeaderIDs[1] != coID {
		t.Fatalf("created series leaders: %+v", created.Data)
	}
	seriesID := created.Data.ID

	// A raid created from the series gets both leaders.
	start := time.Now().UTC().AddDate(0, 0, 7).Format(time.RFC3339)
	var raid struct{ Data struct{ ID int64 } }
	if err := json.Unmarshal(call(admin, http.MethodPost, "/api/v1/raids", map[string]any{"series_id": seriesID, "raid_leader_id": adminID, "scheduled_start": start}, http.StatusCreated), &raid); err != nil {
		t.Fatal(err)
	}
	if got := leadersOf(fmt.Sprintf("/api/v1/raids/%d/leaders", raid.Data.ID)); len(got) != 2 || got[0] != adminID || got[1] != coID {
		t.Fatalf("raid from series: %v", got)
	}
	// A hand-picked first leader stays first; the series' other leaders follow.
	if err := json.Unmarshal(call(admin, http.MethodPost, "/api/v1/raids", map[string]any{"series_id": seriesID, "raid_leader_id": thirdID, "scheduled_start": time.Now().UTC().AddDate(0, 0, 8).Format(time.RFC3339)}, http.StatusCreated), &raid); err != nil {
		t.Fatal(err)
	}
	if got := leadersOf(fmt.Sprintf("/api/v1/raids/%d/leaders", raid.Data.ID)); len(got) != 3 || got[0] != thirdID || got[1] != adminID || got[2] != coID {
		t.Fatalf("raid with own first leader: %v", got)
	}
	// Planned raids get the leaders too.
	call(admin, http.MethodPost, fmt.Sprintf("/api/v1/raid-series/%d/plan", seriesID), map[string]any{"until": time.Now().UTC().AddDate(0, 0, 21).Format("2006-01-02")}, http.StatusOK)
	var raids struct {
		Data []struct {
			ID       int64  `json:"id"`
			SeriesID *int64 `json:"series_id"`
		}
	}
	if err := json.Unmarshal(call(admin, http.MethodGet, "/api/v1/raids", nil, http.StatusOK), &raids); err != nil {
		t.Fatal(err)
	}
	planned := 0
	for _, row := range raids.Data {
		if row.SeriesID != nil && *row.SeriesID == seriesID {
			if got := leadersOf(fmt.Sprintf("/api/v1/raids/%d/leaders", row.ID)); len(got) < 2 {
				t.Fatalf("raid %d of the series lacks the co-leader: %v", row.ID, got)
			}
			planned++
		}
	}
	if planned < 3 {
		t.Fatalf("expected planned raids of the series, got %d", planned)
	}

	// Changing only the first leader keeps the others; existing raids stay as they are.
	before := leadersOf(fmt.Sprintf("/api/v1/raids/%d/leaders", raid.Data.ID))
	var updated struct {
		Data struct {
			LeaderIDs []int64 `json:"leader_ids"`
		}
	}
	if err := json.Unmarshal(call(admin, http.MethodPatch, fmt.Sprintf("/api/v1/raid-series/%d", seriesID), map[string]any{"raid_leader_id": thirdID}, http.StatusOK), &updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Data.LeaderIDs; len(got) != 2 || got[0] != thirdID || got[1] != coID {
		t.Fatalf("first leader change: %v", got)
	}
	if got := leadersOf(fmt.Sprintf("/api/v1/raids/%d/leaders", raid.Data.ID)); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Fatalf("series change altered an existing raid: %v -> %v", before, got)
	}
}
