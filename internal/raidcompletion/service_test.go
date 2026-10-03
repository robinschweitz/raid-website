package raidcompletion

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/db"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func setupDB(t *testing.T) *bun.DB {
	t.Helper()
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)"
	bdb, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = bdb.Close() })
	m, err := migrations.NewMigrator(bdb.DB)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return bdb.DB
}

func seedPlayer(t *testing.T, ctx context.Context, players *player.Repo, name string) int64 {
	t.Helper()
	p := &player.Player{Name: name, Active: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := players.Create(ctx, p); err != nil {
		t.Fatalf("create player %s: %v", name, err)
	}
	return p.ID
}

func seedCharacter(t *testing.T, ctx context.Context, chars *character.Repo, playerID int64, name string) int64 {
	t.Helper()
	c := &character.Character{
		PlayerID:   playerID,
		Name:       name,
		Class:      "Krieger",
		MainSpec:   "Waffen",
		SecondSpec: "Furor",
		Active:     true,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if err := chars.Create(ctx, c); err != nil {
		t.Fatalf("create character %s: %v", name, err)
	}
	return c.ID
}

func TestCompleteRaidAwardsPointsWithoutMainSpecLoot(t *testing.T) {
	ctx := context.Background()
	bdb := setupDB(t)

	players := player.NewRepo(bdb)
	raids := raid.NewRepo(bdb)
	attendanceRepo := attendance.NewRepo(bdb)
	lootRepo := loot.NewRepo(bdb)
	pointsRepo := enhancepoints.NewRepo(bdb)

	leaderID := seedPlayer(t, ctx, players, "Leader")
	plainID := seedPlayer(t, ctx, players, "Plain")
	rewardedID := seedPlayer(t, ctx, players, "Rewarded")

	chars := character.NewRepo(bdb)
	plainCharID := seedCharacter(t, ctx, chars, plainID, "Plainchar")
	rewardedCharID := seedCharacter(t, ctx, chars, rewardedID, "Rewardedchar")

	now := time.Now().UTC()
	rd := &raid.Raid{Name: "ICC", RaidLeaderID: leaderID, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatalf("create raid: %v", err)
	}

	for _, entry := range []struct{ playerID, charID int64 }{{plainID, plainCharID}, {rewardedID, rewardedCharID}} {
		a := &attendance.Attendance{RaidID: rd.ID, PlayerID: entry.playerID, CharacterID: entry.charID, Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
		if err := attendanceRepo.Create(ctx, a); err != nil {
			t.Fatalf("create attendance: %v", err)
		}
	}

	mainSpecItem := &loot.Award{
		RaidID:            rd.ID,
		Boss:              "Saurfang",
		ItemID:            "50001",
		ItemName:          "Testitem",
		AwardedToPlayerID: rewardedID,
		AwardedByPlayerID: leaderID,
		Source:            "manual",
		IsMainSpec:        true,
		AwardedAt:         now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := lootRepo.CreateAward(ctx, mainSpecItem); err != nil {
		t.Fatalf("create award: %v", err)
	}

	svc := New(bdb, raids, attendanceRepo, lootRepo, pointsRepo)
	if err := svc.CompleteRaid(ctx, rd.ID, leaderID); err != nil {
		t.Fatalf("complete raid: %v", err)
	}

	plainBalance, err := pointsRepo.Balance(ctx, plainID)
	if err != nil {
		t.Fatalf("balance plain: %v", err)
	}
	if plainBalance != 10 {
		t.Fatalf("plain balance: want 10 got %d", plainBalance)
	}

	rewardedBalance, err := pointsRepo.Balance(ctx, rewardedID)
	if err != nil {
		t.Fatalf("balance rewarded: %v", err)
	}
	if rewardedBalance != 0 {
		t.Fatalf("rewarded balance: want 0 got %d", rewardedBalance)
	}

	// Idempotency: completing again must not double-award.
	if err := svc.CompleteRaid(ctx, rd.ID, leaderID); err != nil {
		t.Fatalf("complete raid again: %v", err)
	}
	plainBalance, err = pointsRepo.Balance(ctx, plainID)
	if err != nil {
		t.Fatalf("balance plain after second: %v", err)
	}
	if plainBalance != 10 {
		t.Fatalf("plain balance after second completion: want 10 got %d", plainBalance)
	}
}

func TestCompleteRaidCancelledAwardsNoPoints(t *testing.T) {
	ctx := context.Background()
	bdb := setupDB(t)

	players := player.NewRepo(bdb)
	raids := raid.NewRepo(bdb)
	attendanceRepo := attendance.NewRepo(bdb)
	lootRepo := loot.NewRepo(bdb)
	pointsRepo := enhancepoints.NewRepo(bdb)

	leaderID := seedPlayer(t, ctx, players, "Leader")
	plainID := seedPlayer(t, ctx, players, "Plain")

	chars := character.NewRepo(bdb)
	plainCharID := seedCharacter(t, ctx, chars, plainID, "Plainchar")

	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Cancelled", RaidLeaderID: leaderID, Status: validation.RaidStatusCancelled, CreatedAt: now, UpdatedAt: now}
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatalf("create raid: %v", err)
	}
	a := &attendance.Attendance{RaidID: rd.ID, PlayerID: plainID, CharacterID: plainCharID, Role: validation.RoleDD, CreatedAt: now, UpdatedAt: now}
	if err := attendanceRepo.Create(ctx, a); err != nil {
		t.Fatalf("create attendance: %v", err)
	}

	svc := New(bdb, raids, attendanceRepo, lootRepo, pointsRepo)
	if err := svc.CompleteRaid(ctx, rd.ID, leaderID); !errors.Is(err, ErrRaidCancelled) {
		t.Fatalf("want ErrRaidCancelled got %v", err)
	}

	balance, err := pointsRepo.Balance(ctx, plainID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if balance != 0 {
		t.Fatalf("cancelled raid must not award, got %d", balance)
	}
}

func TestCompleteRaidExpiresActiveLocks(t *testing.T) {
	ctx := context.Background()
	bdb := setupDB(t)

	players := player.NewRepo(bdb)
	raids := raid.NewRepo(bdb)
	attendanceRepo := attendance.NewRepo(bdb)
	lootRepo := loot.NewRepo(bdb)
	pointsRepo := enhancepoints.NewRepo(bdb)

	leaderID := seedPlayer(t, ctx, players, "Leader")
	lockerID := seedPlayer(t, ctx, players, "Locker")

	now := time.Now().UTC()
	rd := &raid.Raid{Name: "Lockraid", RaidLeaderID: leaderID, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raids.Create(ctx, rd); err != nil {
		t.Fatalf("create raid: %v", err)
	}

	lock := &enhancepoints.ItemLock{
		PlayerID:  lockerID,
		RaidID:    rd.ID,
		ItemName:  "Testitem",
		Status:    validation.LockStatusActive,
		LockedAt:  now,
		ActorID:   lockerID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := bdb.NewInsert().Model(lock).Exec(ctx); err != nil {
		t.Fatalf("create lock: %v", err)
	}

	svc := New(bdb, raids, attendanceRepo, lootRepo, pointsRepo)
	if err := svc.CompleteRaid(ctx, rd.ID, leaderID); err != nil {
		t.Fatalf("complete raid: %v", err)
	}

	stored := new(enhancepoints.ItemLock)
	if err := bdb.NewSelect().Model(stored).Where("id = ?", lock.ID).Scan(ctx); err != nil {
		t.Fatalf("load lock: %v", err)
	}
	if stored.Status != validation.LockStatusExpired {
		t.Fatalf("want expiration, got %s", stored.Status)
	}
	if stored.ExpiredAt == nil {
		t.Fatal("expired_at must be set")
	}
}
