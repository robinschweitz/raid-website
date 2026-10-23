package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestCurrentRaidOpensTheNewestBegunRaid(t *testing.T) {
	srv, db := setupServer(t)
	actor := seedAccount(t, db, "current-member", "password", validation.RoleMember)
	member := login(t, srv.Client(), srv, "current-member", "password")
	url := srv.URL + "/api/v1/raids/current"

	status, body := do(t, srv.Client(), http.MethodGet, url, "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d %s", status, body)
	}
	status, body = do(t, srv.Client(), http.MethodGet, url, member, nil)
	var failure struct{ Error struct{ Code string } }
	if err := json.Unmarshal(body, &failure); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusNotFound || failure.Error.Code != "no_current_raid" {
		t.Fatalf("no raids: %d %s", status, body)
	}

	now := time.Now().UTC()
	repo := raid.NewRepo(db)
	create := func(name string, status validation.RaidStatus, start time.Time) *raid.Raid {
		rd := &raid.Raid{Name: name, RaidLeaderID: actor, Status: status, ScheduledStart: &start, CreatedAt: now, UpdatedAt: now}
		if err := repo.Create(context.Background(), rd); err != nil {
			t.Fatal(err)
		}
		return rd
	}
	create("Last week", validation.RaidStatusCompleted, now.AddDate(0, 0, -7))
	tonight := create("Tonight", validation.RaidStatusOpen, now.Add(-time.Hour))
	create("Next week", validation.RaidStatusOpen, now.AddDate(0, 0, 7))
	create("Cancelled tonight", validation.RaidStatusCancelled, now.Add(-time.Minute))

	status, body = do(t, srv.Client(), http.MethodGet, url, member, nil)
	var current struct{ Data raid.Raid }
	if err := json.Unmarshal(body, &current); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || current.Data.ID != tonight.ID {
		t.Fatalf("want raid %d, got %d %s", tonight.ID, status, body)
	}
}

func TestCurrentRaidSkipsCompletedRaids(t *testing.T) {
	srv, db := setupServer(t)
	actor := seedAccount(t, db, "current-next-member", "password", validation.RoleMember)
	member := login(t, srv.Client(), srv, "current-next-member", "password")
	url := srv.URL + "/api/v1/raids/current"
	now := time.Now().UTC()
	repo := raid.NewRepo(db)
	create := func(status validation.RaidStatus, start time.Time) *raid.Raid {
		rd := &raid.Raid{Name: "Navigation", RaidLeaderID: actor, Status: status, ScheduledStart: &start, CreatedAt: now, UpdatedAt: now}
		if err := repo.Create(context.Background(), rd); err != nil {
			t.Fatal(err)
		}
		return rd
	}
	create(validation.RaidStatusCompleted, now.Add(-24*time.Hour))
	next := create(validation.RaidStatusPlanned, now.Add(time.Hour))
	create(validation.RaidStatusOpen, now.Add(7*24*time.Hour))
	status, body := do(t, srv.Client(), http.MethodGet, url, member, nil)
	var current struct{ Data raid.Raid }
	if err := json.Unmarshal(body, &current); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || current.Data.ID != next.ID {
		t.Fatalf("want upcoming raid %d, got %d %s", next.ID, status, body)
	}
}

func TestCurrentRaidReturnsNotFoundAfterAllRaidsEnd(t *testing.T) {
	srv, db := setupServer(t)
	actor := seedAccount(t, db, "current-finished-member", "password", validation.RoleMember)
	member := login(t, srv.Client(), srv, "current-finished-member", "password")
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Finished", RaidLeaderID: actor, Status: validation.RaidStatusCompleted, ScheduledStart: &now, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(db).Create(context.Background(), rd); err != nil {
		t.Fatal(err)
	}
	status, body := do(t, srv.Client(), http.MethodGet, srv.URL+"/api/v1/raids/current", member, nil)
	var failure struct{ Error struct{ Code string } }
	if err := json.Unmarshal(body, &failure); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusNotFound || failure.Error.Code != "no_current_raid" {
		t.Fatalf("want no_current_raid, got %d %s", status, body)
	}
}
