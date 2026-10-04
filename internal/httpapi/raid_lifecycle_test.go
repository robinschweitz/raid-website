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

func TestRaidLifecyclePermissionsAndNotFound(t *testing.T) {
	srv, db := setupServer(t)
	actor := seedAccount(t, db, "lifecycle-admin", "password", validation.RoleAdmin)
	seedAccount(t, db, "lifecycle-member", "password", validation.RoleMember)
	admin := login(t, srv.Client(), srv, "lifecycle-admin", "password")
	member := login(t, srv.Client(), srv, "lifecycle-member", "password")
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Permission boundary", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(db).Create(context.Background(), rd); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"cancel", "archive"} {
		t.Run(action, func(t *testing.T) {
			url := fmt.Sprintf("%s/api/v1/raids/%d/%s", srv.URL, rd.ID, action)
			status, body := do(t, srv.Client(), http.MethodPost, url, member, nil)
			if status != http.StatusForbidden {
				t.Fatalf("member: %d %s", status, body)
			}
			status, body = do(t, srv.Client(), http.MethodPost, url, "", nil)
			if status != http.StatusUnauthorized {
				t.Fatalf("unauthenticated: %d %s", status, body)
			}
			url = srv.URL + "/api/v1/raids/999999/" + action
			status, body = do(t, srv.Client(), http.MethodPost, url, admin, nil)
			var response struct{ Error struct{ Code string } }
			if err := json.Unmarshal(body, &response); err != nil {
				t.Fatal(err)
			}
			if status != http.StatusNotFound || response.Error.Code != "not_found" {
				t.Fatalf("missing raid: %d %s", status, body)
			}
		})
	}
	stored, err := raid.NewRepo(db).Get(context.Background(), rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != validation.RaidStatusOpen || stored.ArchivedAt != nil || stored.CompletedAt != nil || stored.CancelledAt != nil {
		t.Fatalf("unauthorized lifecycle mutated raid: %+v", stored)
	}
}

func TestRaidLifecycleArchiveFinalizesOrPreservesHistory(t *testing.T) {
	srv, db := setupServer(t)
	actor := seedAccount(t, db, "lifecycle-assistant", "password", validation.RoleRaidassistent)
	token := login(t, srv.Client(), srv, "lifecycle-assistant", "password")
	for _, state := range []validation.RaidStatus{validation.RaidStatusPlanned, validation.RaidStatusOpen, validation.RaidStatusCompleted, validation.RaidStatusCancelled} {
		t.Run(string(state), func(t *testing.T) {
			now := time.Now().UTC()
			rd := &raid.Raid{Name: string(state), RaidLeaderID: actor, Status: state, CreatedAt: now, UpdatedAt: now}
			if state == validation.RaidStatusPlanned || state == validation.RaidStatusOpen {
				series := &raidseries.Series{Name: string(state), RaidLeaderID: actor, Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin"}
				if err := raidseries.NewService(raidseries.NewRepo(db)).Save(context.Background(), series); err != nil {
					t.Fatal(err)
				}
				rd.SeriesID = &series.ID
				start := now.AddDate(0, 0, -7)
				rd.ScheduledStart = &start
			}
			if state == validation.RaidStatusCompleted {
				rd.CompletedAt = &now
			}
			if state == validation.RaidStatusCancelled {
				rd.CancelledAt = &now
			}
			if err := raid.NewRepo(db).Create(context.Background(), rd); err != nil {
				t.Fatal(err)
			}
			url := fmt.Sprintf("%s/api/v1/raids/%d", srv.URL, rd.ID)
			for i := 0; i < 2; i++ {
				status, body := do(t, srv.Client(), http.MethodPost, url+"/archive", token, nil)
				if state == validation.RaidStatusCancelled {
					assertArchiveCode(t, status, body, "raid_cancelled")
					stored, err := raid.NewRepo(db).Get(context.Background(), rd.ID)
					if err != nil || stored.ArchivedAt != nil || stored.Status != state {
						t.Fatalf("cancelled raid changed: %+v %v", stored, err)
					}
					continue
				}
				if status != http.StatusOK {
					t.Fatalf("archive: %d %s", status, body)
				}
				stored, err := raid.NewRepo(db).Get(context.Background(), rd.ID)
				if err != nil {
					t.Fatal(err)
				}
				if stored.Status != validation.RaidStatusCompleted || stored.ArchivedAt == nil || stored.CompletedAt == nil {
					t.Fatalf("archive changed lifecycle: %+v", stored)
				}
				if i == 0 {
					rd = stored
				} else if !stored.ArchivedAt.Equal(*rd.ArchivedAt) || !stored.UpdatedAt.Equal(rd.UpdatedAt) {
					t.Fatal("archive is not idempotent")
				}
			}
			status, body := do(t, srv.Client(), http.MethodPost, url+"/complete", token, nil)
			if status != http.StatusNotFound {
				t.Fatalf("complete: %d %s", status, body)
			}
			status, body = do(t, srv.Client(), http.MethodPost, url+"/cancel", token, nil)
			if status != http.StatusConflict {
				t.Fatalf("cancel archived: %d %s", status, body)
			}
		})
	}
}
