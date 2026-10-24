package raidcompletion

import (
	"context"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/enhancepoints"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func seedCancelRaid(t *testing.T, raids *raid.Repo, actor int64, name string, status validation.RaidStatus) *raid.Raid {
	t.Helper()
	now := time.Now().UTC()
	rd := &raid.Raid{Name: name, RaidLeaderID: actor, Status: status, CreatedAt: now, UpdatedAt: now}
	if err := raids.Create(context.Background(), rd); err != nil {
		t.Fatal(err)
	}
	return rd
}

func seedActiveLock(t *testing.T, db *bun.DB, playerID, raidID int64, item string) *enhancepoints.ItemLock {
	t.Helper()
	now := time.Now().UTC()
	lock := &enhancepoints.ItemLock{PlayerID: playerID, RaidID: raidID, ItemName: item, Status: validation.LockStatusActive, LockedAt: now, ActorID: playerID, CreatedAt: now, UpdatedAt: now}
	if _, err := db.NewInsert().Model(lock).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	return lock
}

func lockStatus(t *testing.T, db *bun.DB, lock *enhancepoints.ItemLock) enhancepoints.ItemLock {
	t.Helper()
	var stored enhancepoints.ItemLock
	if err := db.NewSelect().Model(&stored).Where("id = ?", lock.ID).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	return stored
}

func TestReqRaid010_AC01(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	other := seedPlayer(t, ctx, player.NewRepo(db), "Other")
	raids := raid.NewRepo(db)
	rd := seedCancelRaid(t, raids, actor, "Cancel me", validation.RaidStatusOpen)
	elsewhere := seedCancelRaid(t, raids, actor, "Stays open", validation.RaidStatusOpen)
	first := seedActiveLock(t, db, actor, rd.ID, "Item A")
	second := seedActiveLock(t, db, other, rd.ID, "Item B")
	untouched := seedActiveLock(t, db, actor, elsewhere.ID, "Item A")
	opening := &enhancepoints.Transaction{PlayerID: actor, Amount: 200, Type: "import_opening", Reason: "imported balance", ActorID: actor, CreatedAt: time.Now().UTC()}
	if err := enhancepoints.NewRepo(db).InsertTransaction(ctx, opening); err != nil {
		t.Fatal(err)
	}

	if _, err := raid.NewService(raids).CancelRaid(ctx, rd.ID, actor); err != nil {
		t.Fatal(err)
	}

	for _, lock := range []*enhancepoints.ItemLock{first, second} {
		stored := lockStatus(t, db, lock)
		if stored.Status != validation.LockStatusExpired || stored.ExpiredAt == nil {
			t.Fatalf("lock %d not expired: %+v", lock.ID, stored)
		}
	}
	if stored := lockStatus(t, db, untouched); stored.Status != validation.LockStatusActive {
		t.Fatalf("lock of another raid changed: %+v", stored)
	}
	balance, err := enhancepoints.NewRepo(db).Balance(ctx, actor)
	if err != nil || balance != 200 {
		t.Fatalf("EP changed: %d %v", balance, err)
	}
	var expiries int
	if err := db.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("entity_type = ?", "item_lock").Where("action = ?", "expire").Scan(ctx, &expiries); err != nil || expiries != 2 {
		t.Fatalf("lock expiry audit entries: %d %v", expiries, err)
	}
	var cancels int
	if err := db.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("entity_type = ?", "raid").Where("action = ?", "cancel").Scan(ctx, &cancels); err != nil || cancels != 1 {
		t.Fatalf("cancel audit entries: %d %v", cancels, err)
	}
}

func TestReqRaid010_AC02(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	raids := raid.NewRepo(db)
	// A raid cancelled before FR-17.6 still holds an active lock; nothing may touch it.
	old := seedCancelRaid(t, raids, actor, "Old cancelled", validation.RaidStatusCancelled)
	oldLock := seedActiveLock(t, db, actor, old.ID, "Item")
	rd := seedCancelRaid(t, raids, actor, "New cancel", validation.RaidStatusOpen)
	seedActiveLock(t, db, actor, rd.ID, "Other item")

	if _, err := raid.NewService(raids).CancelRaid(ctx, rd.ID, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := raid.NewService(raids).CancelRaid(ctx, old.ID, actor); err == nil {
		t.Fatal("cancelled raid must not be cancellable again")
	}
	if stored := lockStatus(t, db, oldLock); stored.Status != validation.LockStatusActive || stored.ExpiredAt != nil {
		t.Fatalf("lock of earlier cancellation changed: %+v", stored)
	}
}

func TestReqRaid010_AC03(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	raids := raid.NewRepo(db)
	rd := seedCancelRaid(t, raids, actor, "Rollback", validation.RaidStatusOpen)
	lock := seedActiveLock(t, db, actor, rd.ID, "Item")
	before, err := raids.Get(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TRIGGER fail_expiry BEFORE UPDATE ON item_locks BEGIN SELECT RAISE(ABORT, 'expiry failed'); END"); err != nil {
		t.Fatal(err)
	}

	if _, err := raid.NewService(raids).CancelRaid(ctx, rd.ID, actor); err == nil {
		t.Fatal("expected failure")
	}
	stored, err := raids.Get(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != validation.RaidStatusOpen || stored.CancelledAt != nil || !stored.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("raid did not roll back: %+v", stored)
	}
	if got := lockStatus(t, db, lock); got.Status != validation.LockStatusActive {
		t.Fatalf("lock changed: %+v", got)
	}
	var entries int
	if err := db.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("entity_type = ?", "raid").Scan(ctx, &entries); err != nil || entries != 0 {
		t.Fatalf("audit entries survived rollback: %d %v", entries, err)
	}
}
