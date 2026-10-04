package raidcompletion

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestConcurrentRaidLifecycle(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	charID := seedCharacter(t, ctx, character.NewRepo(db), actor, "Actorchar")
	raids := raid.NewRepo(db)
	lifecycle := raid.NewService(raids)
	completion := NewService(NewRepo(db))
	for _, competing := range []string{"complete", "cancel", "archive", "cancel_and_archive"} {
		t.Run(competing, func(t *testing.T) {
			for i := 0; i < 20; i++ {
				now := time.Now().UTC()
				rd := &raid.Raid{Name: fmt.Sprintf("%s-%d", competing, i), RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
				if err := raids.Create(ctx, rd); err != nil {
					t.Fatal(err)
				}
				a := &attendance.Attendance{RaidID: rd.ID, PlayerID: actor, CharacterID: charID, Role: validation.RoleDD, IsBench: true, CreatedAt: now, UpdatedAt: now}
				if err := attendance.NewRepo(db).Create(ctx, a); err != nil {
					t.Fatal(err)
				}
				start := make(chan struct{})
				results := make(chan error, 3)
				go func() { <-start; results <- completion.CompleteRaid(ctx, rd.ID, actor) }()
				count := 2
				if competing == "complete" {
					go func() { <-start; results <- completion.CompleteRaid(ctx, rd.ID, actor) }()
				}
				if competing == "cancel" || competing == "cancel_and_archive" {
					go func() { <-start; _, err := lifecycle.CancelRaid(ctx, rd.ID, actor); results <- err }()
				}
				if competing == "archive" || competing == "cancel_and_archive" {
					go func() { <-start; _, err := lifecycle.ArchiveRaid(ctx, rd.ID, actor); results <- err }()
				}
				if competing == "cancel_and_archive" {
					count = 3
				}
				close(start)
				for j := 0; j < count; j++ {
					err := <-results
					if err != nil && !errors.Is(err, ErrRaidCancelled) && !errors.Is(err, ErrRaidNotCompletable) && !errors.Is(err, raid.ErrNotCancellable) {
						t.Fatal(err)
					}
				}
				stored, err := raids.Get(ctx, rd.ID)
				if err != nil {
					t.Fatal(err)
				}
				var amounts []int
				if err := db.NewSelect().TableExpr("enhance_point_transactions").Column("amount").Where("raid_id = ?", rd.ID).Scan(ctx, &amounts); err != nil {
					t.Fatal(err)
				}
				if stored.Status == validation.RaidStatusCompleted {
					if stored.CompletedAt == nil || stored.CancelledAt != nil || len(amounts) != 1 || amounts[0] != 10 {
						t.Fatalf("completed raid/ledger mismatch: %+v %v", stored, amounts)
					}
					if err := completion.CompleteRaid(ctx, rd.ID, actor); err != nil {
						t.Fatal(err)
					}
				} else if stored.CompletedAt != nil || len(amounts) != 0 {
					t.Fatalf("uncompleted raid has completion effects: %+v %v", stored, amounts)
				}
				if competing == "archive" || competing == "cancel_and_archive" {
					if stored.ArchivedAt == nil {
						t.Fatal("archive was overwritten")
					}
				}
			}
		})
	}
}

func TestLifecycleMissingActorAndRaid(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	raids := raid.NewRepo(db)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Empty", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	lifecycle := raid.NewService(raids)
	operations := []func(int64, int64) error{
		func(id, actor int64) error { return NewService(NewRepo(db)).CompleteRaid(ctx, id, actor) },
		func(id, actor int64) error { _, err := lifecycle.CancelRaid(ctx, id, actor); return err },
		func(id, actor int64) error { _, err := lifecycle.ArchiveRaid(ctx, id, actor); return err },
	}
	for _, operation := range operations {
		if err := operation(rd.ID, 999999); !errors.Is(err, raid.ErrActorNotFound) {
			t.Fatalf("missing actor: %v", err)
		}
		if err := operation(999999, actor); !errors.Is(err, raid.ErrNotFound) {
			t.Fatalf("missing raid: %v", err)
		}
	}
	stored, err := raids.Get(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != validation.RaidStatusOpen || stored.ArchivedAt != nil || stored.CompletedAt != nil || stored.CancelledAt != nil {
		t.Fatalf("failed operation mutated raid: %+v", stored)
	}
}

func TestCompletionRollsBackLedgersAndLocks(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	charID := seedCharacter(t, ctx, character.NewRepo(db), actor, "Actorchar")
	raids := raid.NewRepo(db)
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Rollback", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	before, err := raids.Get(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	a := &attendance.Attendance{RaidID: rd.ID, PlayerID: actor, CharacterID: charID, Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
	if err := attendance.NewRepo(db).Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	lock := &enhancepoints.ItemLock{PlayerID: actor, RaidID: rd.ID, ItemName: "Item", Status: validation.LockStatusActive, LockedAt: now, ActorID: actor, CreatedAt: now, UpdatedAt: now}
	if _, err := db.NewInsert().Model(lock).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TRIGGER fail_expiry BEFORE UPDATE ON item_locks BEGIN SELECT RAISE(ABORT, 'expiry failed'); END"); err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewRepo(db))
	if err := svc.CompleteRaid(ctx, rd.ID, actor); err == nil {
		t.Fatal("expected expiry failure")
	}
	stored, err := raids.Get(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != validation.RaidStatusOpen || stored.CompletedAt != nil || !stored.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("completion did not roll back: %+v", stored)
	}
	balance, err := enhancepoints.NewRepo(db).Balance(ctx, actor)
	if err != nil || balance != 0 {
		t.Fatalf("ledger did not roll back: %d %v", balance, err)
	}
	if err := db.NewSelect().Model(lock).WherePK().Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if lock.Status != validation.LockStatusActive || lock.ExpiredAt != nil {
		t.Fatalf("lock did not roll back: %+v", lock)
	}
	if _, err := db.ExecContext(ctx, "DROP TRIGGER fail_expiry"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := svc.CompleteRaid(ctx, rd.ID, actor); err != nil {
			t.Fatal(err)
		}
	}
	balance, err = enhancepoints.NewRepo(db).Balance(ctx, actor)
	if err != nil || balance != 10 {
		t.Fatalf("retry ledger: %d %v", balance, err)
	}
}

func TestCancelAndArchiveRollback(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(db), "Actor")
	raids := raid.NewRepo(db)
	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Rollback", RaidLeaderID: actor, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	before, err := raids.Get(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TRIGGER fail_lifecycle BEFORE UPDATE ON raids BEGIN SELECT RAISE(ABORT, 'lifecycle failed'); END"); err != nil {
		t.Fatal(err)
	}
	svc := raid.NewService(raids)
	if _, err := svc.CancelRaid(ctx, rd.ID, actor); err == nil {
		t.Fatal("expected cancellation failure")
	}
	if _, err := svc.ArchiveRaid(ctx, rd.ID, actor); err == nil {
		t.Fatal("expected archive failure")
	}
	stored, err := raids.Get(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != validation.RaidStatusOpen || stored.ArchivedAt != nil || stored.CancelledAt != nil || !stored.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("failed lifecycle mutated raid: %+v", stored)
	}
	if _, err := db.ExecContext(ctx, "DROP TRIGGER fail_lifecycle"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelRaid(ctx, rd.ID, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ArchiveRaid(ctx, rd.ID, actor); err != nil {
		t.Fatal(err)
	}
	stored, err = raids.Get(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != validation.RaidStatusCancelled || stored.ArchivedAt == nil || stored.CancelledAt == nil || stored.CompletedAt != nil {
		t.Fatalf("retry lifecycle: %+v", stored)
	}
}
