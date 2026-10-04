package enhancepoints_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"raid-website/internal/db"
	"raid-website/internal/enhancepoints"
	v1 "raid-website/internal/httpapi/v1"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestCreateLockHTTPRules(t *testing.T) {
	for _, tc := range []struct {
		name       string
		itemName   string
		raidStatus validation.RaidStatus
		archived   bool
		missing    bool
		balance    int
		role       validation.Role
		existing   int
		status     int
		code       string
	}{
		{"valid trimmed", " Item ", validation.RaidStatusPlanned, false, false, 150, validation.RoleMember, 0, http.StatusCreated, ""},
		{"blank name", " \t ", validation.RaidStatusPlanned, false, false, 150, validation.RoleMember, 0, http.StatusBadRequest, "invalid_request"},
		{"low balance", "Item", validation.RaidStatusOpen, false, false, 149, validation.RoleMember, 0, http.StatusUnprocessableEntity, "lock_not_allowed"},
		{"completed", "Item", validation.RaidStatusCompleted, false, false, 150, validation.RoleMember, 0, http.StatusConflict, "lock_raid_closed"},
		{"cancelled", "Item", validation.RaidStatusCancelled, false, false, 150, validation.RoleMember, 0, http.StatusConflict, "lock_raid_closed"},
		{"archived", "Item", validation.RaidStatusOpen, true, false, 150, validation.RoleMember, 0, http.StatusConflict, "lock_raid_closed"},
		{"missing raid", "Item", validation.RaidStatusPlanned, false, true, 150, validation.RoleMember, 0, http.StatusNotFound, "not_found"},
		{"existing name only", "Item", validation.RaidStatusOpen, false, false, 150, validation.RoleMember, 1, http.StatusConflict, "lock_conflict"},
		{"legacy ambiguity", "Item", validation.RaidStatusOpen, false, false, 150, validation.RoleMember, 2, http.StatusConflict, "lock_conflict"},
		{"guest own lock", "Item", validation.RaidStatusOpen, false, false, 150, validation.RoleGuest, 0, http.StatusForbidden, "forbidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			bdb, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "lock_http.db")+"?_pragma=foreign_keys(1)")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = bdb.Close() })
			m, err := migrations.NewMigrator(bdb.DB)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.Migrate(ctx); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			actor := &player.Player{Name: "Owner", Role: tc.role, Active: true, CreatedAt: now, UpdatedAt: now}
			if err := player.NewRepo(bdb.DB).Create(ctx, actor); err != nil {
				t.Fatal(err)
			}
			raidRecord := &raid.Raid{Name: "ICC", RaidLeaderID: actor.ID, Status: tc.raidStatus, CreatedAt: now, UpdatedAt: now}
			if tc.archived {
				raidRecord.ArchivedAt = &now
			}
			if err := raid.NewRepo(bdb.DB).Create(ctx, raidRecord); err != nil {
				t.Fatal(err)
			}
			points := enhancepoints.NewRepo(bdb.DB)
			if err := points.InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: actor.ID, Amount: tc.balance, Type: "adjustment", Reason: "test balance", ActorID: actor.ID, CreatedAt: now}); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < tc.existing; i++ {
				if err := points.CreateLock(ctx, &enhancepoints.ItemLock{PlayerID: actor.ID, RaidID: raidRecord.ID, ItemName: "Item", Status: validation.LockStatusActive, ActorID: actor.ID, LockedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
			}
			raidID := raidRecord.ID
			if tc.missing {
				raidID++
			}
			body, err := json.Marshal(map[string]any{"raid_id": raidID, "item_id": " 123 ", "item_name": tc.itemName, "boss": " Boss ", "notes": " Notes "})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/players/%d/locks", actor.ID), bytes.NewReader(body))
			request = request.WithContext(v1.WithActor(ctx, actor))
			mux := http.NewServeMux()
			v1.NewAPI(v1.Deps{Points: points}).Register(mux)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			var out struct {
				Data  enhancepoints.ItemLock `json:"data"`
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.status || out.Error.Code != tc.code {
				t.Fatalf("want %d/%s, got %d %s", tc.status, tc.code, response.Code, response.Body.String())
			}
			locks, err := points.ListLocksByPlayer(ctx, actor.ID)
			wantCount := tc.existing
			if tc.status == http.StatusCreated {
				wantCount++
				if out.Data.ItemName != "Item" || out.Data.ItemID != "123" || out.Data.Boss != "Boss" || out.Data.Notes != "Notes" || out.Data.ActorID != actor.ID {
					t.Fatalf("invalid created lock: %+v", out.Data)
				}
			}
			if err != nil || len(locks) != wantCount {
				t.Fatalf("want %d locks, got %v, err %v", wantCount, locks, err)
			}
			balance, err := points.Balance(ctx, actor.ID)
			if err != nil || balance != tc.balance {
				t.Fatalf("lock request changed balance: %d, err %v", balance, err)
			}
		})
	}
}
