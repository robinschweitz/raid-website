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
	var current raid.Raid
	if err := json.Unmarshal(body, &current); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || current.ID != tonight.ID {
		t.Fatalf("want raid %d, got %d %s", tonight.ID, status, body)
	}
}
