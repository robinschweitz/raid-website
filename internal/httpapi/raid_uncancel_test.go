package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/enhancepoints"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

// FR-10.18 (ADR 115): staff take back a cancellation; the raid gets its earlier
// status back, expired locks stay expired, and a member gets 403.
func TestReqRaid_UncancelRestoresStatusKeepsLocksExpired(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	staffID := seedAccount(t, bdb, "uncancelstaff", "password", validation.RoleRaidassistent)
	memberID := seedAccount(t, bdb, "uncancelmember", "password", validation.RoleMember)
	staff := login(t, client, srv, "uncancelstaff", "password")
	member := login(t, client, srv, "uncancelmember", "password")
	raids := raid.NewRepo(bdb)
	now := time.Now().UTC()
	start := now.Add(48 * time.Hour)

	for _, status := range []validation.RaidStatus{validation.RaidStatusPlanned, validation.RaidStatusOpen} {
		rd := &raid.Raid{Name: "Abgesagt " + string(status), RaidLeaderID: staffID, Status: status, ScheduledStart: &start, CreatedAt: now, UpdatedAt: now}
		if err := raids.Create(ctx, rd); err != nil {
			t.Fatal(err)
		}
		lock := &enhancepoints.ItemLock{PlayerID: memberID, RaidID: rd.ID, PoolID: 1, ItemID: "50730", ItemName: "Bryntroll", Status: validation.LockStatusActive, LockedAt: now, ActorID: memberID, CreatedAt: now, UpdatedAt: now}
		if err := enhancepoints.NewRepo(bdb).CreateLock(ctx, lock); err != nil {
			t.Fatal(err)
		}
		url := fmt.Sprintf("%s/api/v1/raids/%d", srv.URL, rd.ID)
		if status, body := do(t, client, http.MethodPost, url+"/uncancel", staff, nil); status != http.StatusConflict || errorCode(t, body) != "raid_not_cancelled" {
			t.Fatalf("uncancel active raid: %d %s", status, body)
		}
		if status, body := do(t, client, http.MethodPost, url+"/cancel", staff, nil); status != http.StatusOK {
			t.Fatalf("cancel: %d %s", status, body)
		}
		if status, _ := do(t, client, http.MethodPost, url+"/uncancel", "", nil); status != http.StatusUnauthorized {
			t.Fatalf("anonymous uncancel: %d", status)
		}
		if status, _ := do(t, client, http.MethodPost, url+"/uncancel", member, nil); status != http.StatusForbidden {
			t.Fatalf("member uncancel: %d", status)
		}
		if status, body := do(t, client, http.MethodPost, url+"/uncancel", staff, nil); status != http.StatusOK {
			t.Fatalf("uncancel: %d %s", status, body)
		}
		stored, err := raids.Get(ctx, rd.ID)
		if err != nil || stored.Status != status || stored.CancelledAt != nil {
			t.Fatalf("restored raid: %+v %v", stored, err)
		}
		storedLock, err := enhancepoints.NewRepo(bdb).GetLock(ctx, lock.ID)
		if err != nil || storedLock.Status != validation.LockStatusExpired {
			t.Fatalf("lock must stay expired: %+v %v", storedLock, err)
		}
		var audits int
		if err := bdb.NewRaw("SELECT COUNT(*) FROM audit_log WHERE entity_type = 'raid' AND entity_id = ? AND action = 'uncancel'", fmt.Sprint(rd.ID)).Scan(ctx, &audits); err != nil || audits != 1 {
			t.Fatalf("uncancel audit entries: %d %v", audits, err)
		}
	}

	// A raid cancelled without an audit entry (before ADR 115) becomes PLANNED.
	legacy := &raid.Raid{Name: "Alt abgesagt", RaidLeaderID: staffID, Status: validation.RaidStatusCancelled, CancelledAt: &now, ScheduledStart: &start, CreatedAt: now, UpdatedAt: now}
	if err := raids.Create(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if status, body := do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/uncancel", srv.URL, legacy.ID), staff, nil); status != http.StatusOK {
		t.Fatalf("uncancel legacy: %d %s", status, body)
	}
	if stored, err := raids.Get(ctx, legacy.ID); err != nil || stored.Status != validation.RaidStatusPlanned {
		t.Fatalf("legacy raid: %+v %v", stored, err)
	}
}
