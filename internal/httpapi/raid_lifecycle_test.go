package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/raid"
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
	for _, action := range []string{"complete", "cancel", "archive"} {
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

func TestRaidLifecycleArchivedCompletion(t *testing.T) {
	srv, db := setupServer(t)
	actor := seedAccount(t, db, "lifecycle-assistant", "password", validation.RoleRaidassistent)
	token := login(t, srv.Client(), srv, "lifecycle-assistant", "password")
	for _, state := range []validation.RaidStatus{validation.RaidStatusPlanned, validation.RaidStatusOpen, validation.RaidStatusCompleted, validation.RaidStatusCancelled} {
		t.Run(string(state), func(t *testing.T) {
			now := time.Now().UTC()
			rd := &raid.Raid{Name: string(state), RaidLeaderID: actor, Status: state, CreatedAt: now, UpdatedAt: now}
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
				if status != http.StatusOK {
					t.Fatalf("archive: %d %s", status, body)
				}
				stored, err := raid.NewRepo(db).Get(context.Background(), rd.ID)
				if err != nil {
					t.Fatal(err)
				}
				if stored.Status != state || stored.ArchivedAt == nil {
					t.Fatalf("archive changed lifecycle: %+v", stored)
				}
				if i == 0 {
					rd = stored
				} else if !stored.ArchivedAt.Equal(*rd.ArchivedAt) || !stored.UpdatedAt.Equal(rd.UpdatedAt) {
					t.Fatal("archive is not idempotent")
				}
			}
			want, code := http.StatusConflict, "raid_not_completable"
			if state == validation.RaidStatusCompleted {
				want = http.StatusOK
			}
			if state == validation.RaidStatusCancelled {
				code = "raid_cancelled"
			}
			status, body := do(t, srv.Client(), http.MethodPost, url+"/complete", token, nil)
			var response struct{ Error struct{ Code string } }
			if err := json.Unmarshal(body, &response); err != nil {
				t.Fatal(err)
			}
			if status != want || (want == http.StatusConflict && response.Error.Code != code) {
				t.Fatalf("complete: %d %s", status, body)
			}
			status, body = do(t, srv.Client(), http.MethodPost, url+"/cancel", token, nil)
			if status != http.StatusConflict {
				t.Fatalf("cancel archived: %d %s", status, body)
			}
		})
	}
}
