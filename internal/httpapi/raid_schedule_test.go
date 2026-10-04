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

func TestRaidSchedulePatchPartialFieldsAndPermissions(t *testing.T) {
	srv, database := setupServer(t)
	actor := seedAccount(t, database, "schedule-assistant", "password", validation.RoleRaidassistent)
	seedAccount(t, database, "schedule-guest", "password", validation.RoleGuest)
	seedAccount(t, database, "schedule-member", "password", validation.RoleMember)
	assistant := login(t, srv.Client(), srv, "schedule-assistant", "password")
	guest := login(t, srv.Client(), srv, "schedule-guest", "password")
	member := login(t, srv.Client(), srv, "schedule-member", "password")
	ctx := context.Background()
	series := &raidseries.Series{Name: "Series", RaidLeaderID: actor, Weekdays: []int{1, 3}, IntervalWeeks: 2, StartTime: "20:00", Timezone: "Europe/Berlin"}
	if err := raidseries.NewService(raidseries.NewRepo(database)).Save(ctx, series); err != nil {
		t.Fatal(err)
	}
	raids := raid.NewRepo(database)
	for _, state := range []validation.RaidStatus{validation.RaidStatusPlanned, validation.RaidStatusOpen} {
		t.Run(string(state), func(t *testing.T) {
			now := time.Now().UTC()
			rd := &raid.Raid{Name: "Preserved name", RaidLeaderID: actor, Status: state, Notes: "Preserved notes", ScheduledEnd: &now, CreatedAt: now, UpdatedAt: now}
			if err := raids.Create(ctx, rd); err != nil {
				t.Fatal(err)
			}
			url := fmt.Sprintf("%s/api/v1/raids/%d", srv.URL, rd.ID)
			for _, token := range []string{guest, member, ""} {
				status, body := do(t, srv.Client(), http.MethodPatch, url, token, map[string]any{"scheduled_start": "2026-10-05T20:00:00+02:00"})
				want := http.StatusForbidden
				if token == "" {
					want = http.StatusUnauthorized
				}
				if status != want {
					t.Fatalf("permission: %d %s", status, body)
				}
			}
			patch := func(fields map[string]any) raid.Raid {
				t.Helper()
				status, body := do(t, srv.Client(), http.MethodPatch, url, assistant, fields)
				if status != http.StatusOK {
					t.Fatalf("patch: %d %s", status, body)
				}
				var result struct {
					Data raid.Raid `json:"data"`
				}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Fatal(err)
				}
				return result.Data
			}
			startOnly := patch(map[string]any{"scheduled_start": "2026-10-05T20:00:00+02:00"})
			if startOnly.SeriesID != nil || startOnly.ScheduledStart == nil || startOnly.ScheduledStart.Format(time.RFC3339) != "2026-10-05T18:00:00Z" {
				t.Fatalf("start-only: %+v", startOnly)
			}
			seriesOnly := patch(map[string]any{"series_id": series.ID})
			if seriesOnly.SeriesID == nil || *seriesOnly.SeriesID != series.ID || !seriesOnly.ScheduledStart.Equal(*startOnly.ScheduledStart) {
				t.Fatalf("series-only: %+v", seriesOnly)
			}
			// Distinct occurrences avoid conflicts when the same series is used by both lifecycle states.
			rawStart := "2026-10-07T20:00:00+02:00"
			if state == validation.RaidStatusOpen {
				rawStart = "2026-10-08T20:00:00+02:00"
			}
			both := patch(map[string]any{"series_id": series.ID, "scheduled_start": rawStart})
			if both.SeriesID == nil || *both.SeriesID != series.ID || both.ScheduledStart.Equal(*seriesOnly.ScheduledStart) || both.Name != rd.Name || both.RaidLeaderID != rd.RaidLeaderID || both.Notes != rd.Notes || !both.ScheduledEnd.Equal(now.Truncate(time.Microsecond)) {
				t.Fatalf("both fields/unrelated metadata: %+v", both)
			}
			for _, invalid := range []any{nil, "", " ", "not-a-date", "2026-10-05", 123, map[string]any{}} {
				status, body := do(t, srv.Client(), http.MethodPatch, url, assistant, map[string]any{"series_id": series.ID, "scheduled_start": invalid})
				var response struct{ Error struct{ Code string } }
				if err := json.Unmarshal(body, &response); err != nil {
					t.Fatal(err)
				}
				if status != http.StatusBadRequest || response.Error.Code != "invalid_scheduled_start" {
					t.Fatalf("invalid timestamp %+v: %d %s", invalid, status, body)
				}
			}
			status, _ := do(t, srv.Client(), http.MethodPatch, url, assistant, map[string]any{"scheduled_end": "2026-10-05T20:00:00Z"})
			if status != http.StatusBadRequest {
				t.Fatalf("unknown field: %d", status)
			}
			status, _ = do(t, srv.Client(), http.MethodPatch, url, assistant, map[string]any{"series_id": 999999, "scheduled_start": "2027-01-01T20:00:00Z"})
			if status != http.StatusNotFound {
				t.Fatalf("missing series: %d", status)
			}
			occupiedStart := time.Date(2027, 1, 1, 20, 0, 0, 0, time.UTC)
			if state == validation.RaidStatusOpen {
				occupiedStart = occupiedStart.AddDate(0, 0, 1)
			}
			occupied := &raid.Raid{Name: "Occupied occurrence", RaidLeaderID: actor, Status: validation.RaidStatusPlanned, SeriesID: &series.ID, ScheduledStart: &occupiedStart, CreatedAt: now, UpdatedAt: now}
			if err := raids.Create(ctx, occupied); err != nil {
				t.Fatal(err)
			}
			status, body := do(t, srv.Client(), http.MethodPatch, url, assistant, map[string]any{"scheduled_start": occupiedStart.Format(time.RFC3339)})
			assertArchiveCode(t, status, body, "occurrence_conflict")
			stored, err := raids.Get(ctx, rd.ID)
			if err != nil || !stored.ScheduledStart.Equal(*both.ScheduledStart) || !stored.UpdatedAt.Equal(both.UpdatedAt.Truncate(time.Microsecond)) || *stored.SeriesID != series.ID {
				t.Fatalf("rejected patch changed schedule: %+v %v", stored, err)
			}
		})
	}
}

func TestRaidSchedulePatchRejectsHistory(t *testing.T) {
	srv, database := setupServer(t)
	actor := seedAccount(t, database, "schedule-history", "password", validation.RoleRaidassistent)
	token := login(t, srv.Client(), srv, "schedule-history", "password")
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, state := range []validation.RaidStatus{validation.RaidStatusCancelled, validation.RaidStatusCompleted, validation.RaidStatusOpen, validation.RaidStatusPlanned} {
		rd := &raid.Raid{Name: "History", RaidLeaderID: actor, Status: state, ScheduledStart: &now, CreatedAt: now, UpdatedAt: now}
		if state == validation.RaidStatusOpen || state == validation.RaidStatusPlanned {
			rd.ArchivedAt = &now
		}
		if err := raid.NewRepo(database).Create(ctx, rd); err != nil {
			t.Fatal(err)
		}
		status, body := do(t, srv.Client(), http.MethodPatch, fmt.Sprintf("%s/api/v1/raids/%d", srv.URL, rd.ID), token, map[string]any{"scheduled_start": "2027-01-01T20:00:00Z"})
		assertArchiveCode(t, status, body, "schedule_not_editable")
		stored, err := raid.NewRepo(database).Get(ctx, rd.ID)
		if err != nil || !stored.ScheduledStart.Equal(now) || !stored.UpdatedAt.Equal(now) || stored.Status != state {
			t.Fatalf("history rewritten: %+v %v", stored, err)
		}
	}
}
