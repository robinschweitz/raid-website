package enhancepoints

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

func setupLockCreation(t *testing.T, status validation.RaidStatus, isArchived bool) (context.Context, *Repo, *testRaid, []int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	bdb, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "locks.db")+"?_pragma=foreign_keys(1)")
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
	repo := NewRepo(bdb.DB)
	var playerIDs []int64
	for _, name := range []string{"First", "Second"} {
		p := &player.Player{Name: name, Role: validation.RoleMember, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := player.NewRepo(bdb.DB).Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		playerIDs = append(playerIDs, p.ID)
		if err := repo.InsertTransaction(ctx, &Transaction{PlayerID: p.ID, Amount: 150, Type: "adjustment", Reason: "test balance", ActorID: p.ID, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	raidRecord := &testRaid{Name: "ICC", RaidLeaderID: playerIDs[0], Status: status, CreatedAt: now, UpdatedAt: now}
	if isArchived {
		raidRecord.ArchivedAt = &now
	}
	if err := createTestRaid(ctx, bdb.DB, raidRecord); err != nil {
		t.Fatal(err)
	}
	return ctx, repo, raidRecord, playerIDs
}

func TestItemsMatch(t *testing.T) {
	for _, tc := range []struct {
		name, id, itemName, otherID, otherName string
		want                                   bool
	}{
		{"same ID renamed", " 123 ", "Old", "123", "New", true},
		{"different IDs same name", "123", "Item", "456", "Item", false},
		{"legacy name fallback", "123", " Item ", "", "Item", true},
		{"incoming name fallback", "", "Item", "123", " Item ", true},
		{"both name only", "", " Item ", "", "Item", true},
		{"case sensitive", "", "Item", "", "item", false},
		{"different names", "123", "Item", "", "Other", false},
		{"empty fallback", "123", " ", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ItemsMatch(tc.id, tc.itemName, tc.otherID, tc.otherName); got != tc.want {
				t.Fatalf("want %t got %t", tc.want, got)
			}
		})
	}
}

func TestConcurrentCompetingLockCreation(t *testing.T) {
	for _, itemID := range []string{"123", "", "mixed"} {
		t.Run("item_id="+itemID, func(t *testing.T) {
			ctx, repo, raidRecord, players := setupLockCreation(t, validation.RaidStatusOpen, false)
			service := NewService(repo)
			start := make(chan struct{})
			errs := make(chan error, 8)
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				candidateID := itemID
				if itemID == "mixed" {
					candidateID = "123"
					if i%2 == 0 {
						candidateID = ""
					}
				}
				wg.Add(1)
				go func(playerID int64, candidateID string) {
					defer wg.Done()
					<-start
					errs <- service.CreateLock(ctx, &ItemLock{PlayerID: playerID, RaidID: raidRecord.ID, ItemID: candidateID, ItemName: " Item "}, playerID)
				}(players[i%len(players)], candidateID)
			}
			close(start)
			wg.Wait()
			close(errs)
			successes, conflicts := 0, 0
			for err := range errs {
				if err == nil {
					successes++
				} else if errors.Is(err, ErrLockConflict) {
					conflicts++
				} else {
					t.Errorf("unexpected error: %v", err)
				}
			}
			if successes != 1 || conflicts != 7 {
				t.Fatalf("want one success and seven conflicts, got %d/%d", successes, conflicts)
			}
			locks, err := repo.ListActiveLocksByRaid(ctx, raidRecord.ID)
			if err != nil || len(locks) != 1 {
				t.Fatalf("want one active lock, got %v, err %v", locks, err)
			}
			for _, playerID := range players {
				balance, err := repo.Balance(ctx, playerID)
				if err != nil || balance != 150 {
					t.Fatalf("balance changed: %d, err %v", balance, err)
				}
			}
		})
	}
}

func TestResolveActiveLockIgnoresInactive(t *testing.T) {
	locks := []ItemLock{
		{ID: 1, ItemID: "123", ItemName: "Item", Status: validation.LockStatusCancelled},
		{ID: 2, ItemID: "123", ItemName: "Item", Status: validation.LockStatusUsed},
		{ID: 3, ItemID: "123", ItemName: "Item", Status: validation.LockStatusExpired},
		{ID: 4, ItemID: "123", ItemName: "Item", Status: validation.LockStatusActive},
	}
	match, err := ResolveActiveLock(locks, "123", "Renamed")
	if err != nil || match == nil || match.ID != 4 {
		t.Fatalf("want sole active match 4, got %+v, err %v", match, err)
	}
	match, err = ResolveActiveLock(locks[:3], "123", "Item")
	if err != nil || match != nil {
		t.Fatalf("inactive locks must not match: %+v, err %v", match, err)
	}
}

func TestCreateLockRaidLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   validation.RaidStatus
		archived bool
		want     error
	}{
		{"planned", validation.RaidStatusPlanned, false, nil},
		{"open", validation.RaidStatusOpen, false, nil},
		{"completed", validation.RaidStatusCompleted, false, ErrLockRaidClosed},
		{"cancelled", validation.RaidStatusCancelled, false, ErrLockRaidClosed},
		{"archived planned", validation.RaidStatusPlanned, true, ErrLockRaidClosed},
		{"archived open", validation.RaidStatusOpen, true, ErrLockRaidClosed},
		{"missing", validation.RaidStatusPlanned, false, ErrLockRaidNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, repo, raidRecord, players := setupLockCreation(t, tc.status, tc.archived)
			raidID := raidRecord.ID
			if tc.name == "missing" {
				raidID++
			}
			err := NewService(repo).CreateLock(ctx, &ItemLock{PlayerID: players[0], RaidID: raidID, ItemName: "Item"}, players[0])
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			locks, err := repo.ListLocksByPlayer(ctx, players[0])
			wantCount := 0
			if tc.want == nil {
				wantCount = 1
			}
			if err != nil || len(locks) != wantCount {
				t.Fatalf("want %d locks, got %v, err %v", wantCount, locks, err)
			}
		})
	}
}

func TestCreateLockValidationAndMultipleItems(t *testing.T) {
	ctx, repo, raidRecord, players := setupLockCreation(t, validation.RaidStatusPlanned, false)
	service := NewService(repo)
	for _, itemName := range []string{"", " \t\n "} {
		if err := service.CreateLock(ctx, &ItemLock{PlayerID: players[0], RaidID: raidRecord.ID, ItemID: "123", ItemName: itemName}, players[0]); !errors.Is(err, ErrLockItemNameRequired) {
			t.Fatalf("blank item name: got %v", err)
		}
	}
	first := &ItemLock{PlayerID: players[0], RaidID: raidRecord.ID, ItemID: " 123 ", ItemName: " Item ", Boss: " Boss ", Notes: " Notes "}
	if err := service.CreateLock(ctx, first, players[0]); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetLock(ctx, first.ID)
	if err != nil || stored.ItemID != "123" || stored.ItemName != "Item" || stored.Boss != "Boss" || stored.Notes != "Notes" {
		t.Fatalf("fields not trimmed: %+v, err %v", stored, err)
	}
	for _, candidate := range []ItemLock{
		{PlayerID: players[0], RaidID: raidRecord.ID, ItemID: "123", ItemName: "Renamed"},
		{PlayerID: players[1], RaidID: raidRecord.ID, ItemName: " Item "},
	} {
		if err := service.CreateLock(ctx, &candidate, candidate.PlayerID); !errors.Is(err, ErrLockConflict) {
			t.Fatalf("competing lock: got %v", err)
		}
	}
	for _, candidate := range []ItemLock{
		{PlayerID: players[0], RaidID: raidRecord.ID, ItemID: "456", ItemName: "Item"},
		{PlayerID: players[0], RaidID: raidRecord.ID, ItemName: "Other"},
	} {
		if err := service.CreateLock(ctx, &candidate, candidate.PlayerID); err != nil {
			t.Fatalf("different item should allow: %v", err)
		}
	}
	if _, err := service.CancelLock(ctx, players[0], first.ID, players[0]); err != nil {
		t.Fatal(err)
	}
	if err := service.CreateLock(ctx, &ItemLock{PlayerID: players[1], RaidID: raidRecord.ID, ItemID: "123", ItemName: "Item"}, players[1]); err != nil {
		t.Fatalf("cancelled lock must not compete: %v", err)
	}
}

func TestFindActiveLockByItemLegacyAmbiguity(t *testing.T) {
	ctx, repo, raidRecord, players := setupLockCreation(t, validation.RaidStatusOpen, false)
	now := time.Now().UTC()
	for i, itemID := range []string{"", "123"} {
		lock := &ItemLock{PlayerID: players[i], RaidID: raidRecord.ID, ItemID: itemID, ItemName: " Item ", Status: validation.LockStatusActive, ActorID: players[i], LockedAt: now, CreatedAt: now, UpdatedAt: now}
		if err := repo.CreateLock(ctx, lock); err != nil {
			t.Fatal(err)
		}
	}
	match, err := repo.FindActiveLockByItem(ctx, raidRecord.ID, "123", "Item")
	if match != nil || !errors.Is(err, ErrLockConflict) {
		t.Fatalf("ambiguous legacy locks: match %+v, err %v", match, err)
	}
	if err := NewService(repo).CreateLock(ctx, &ItemLock{PlayerID: players[0], RaidID: raidRecord.ID, ItemID: "123", ItemName: "Item"}, players[0]); !errors.Is(err, ErrLockConflict) {
		t.Fatalf("ambiguous creation: got %v", err)
	}
	match, err = repo.FindActiveLockByItem(ctx, raidRecord.ID, "456", "Item")
	if err != nil || match == nil || match.ItemID != "" {
		t.Fatalf("different ID should only match name-only lock: %+v, err %v", match, err)
	}
	match, err = repo.FindActiveLockByItem(ctx, raidRecord.ID+1, "123", "Item")
	if err != nil || match != nil {
		t.Fatalf("another raid should not match: %+v, err %v", match, err)
	}
	locks, err := repo.ListActiveLocksByRaid(ctx, raidRecord.ID)
	if err != nil || len(locks) != 2 {
		t.Fatalf("legacy ambiguity must not mutate locks: %v, err %v", locks, err)
	}
}

// testRaid is a minimal raids row; importing the raid package here would form
// an import cycle because raid expires locks on cancel (FR-17.6).
type testRaid struct {
	bun.BaseModel `bun:"table:raids"`

	ID           int64                 `bun:"id,pk,autoincrement"`
	Name         string                `bun:"name,notnull"`
	RaidLeaderID int64                 `bun:"raid_leader_id,notnull"`
	Status       validation.RaidStatus `bun:"status,notnull"`
	ArchivedAt   *time.Time            `bun:"archived_at,nullzero"`
	CreatedAt    time.Time             `bun:"created_at,notnull"`
	UpdatedAt    time.Time             `bun:"updated_at,notnull"`
}

func createTestRaid(ctx context.Context, db bun.IDB, rd *testRaid) error {
	_, err := db.NewInsert().Model(rd).Exec(ctx)
	return err
}
