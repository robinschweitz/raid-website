package loot

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestWithdrawnAwardDoesNotBlockMainSpecEligibility(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "sync.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	migrator, err := migrations.NewMigrator(database.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	actor := &player.Player{Name: "Sync actor", Role: validation.RoleAdmin, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := player.NewRepo(database.DB).Create(ctx, actor); err != nil {
		t.Fatal(err)
	}
	rd := &raid.Raid{Name: "Legacy sync", RaidLeaderID: actor.ID, Status: validation.RaidStatusCompleted, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(database.DB).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(database.DB)
	award := &Award{
		RaidID:            rd.ID,
		Boss:              "Boss",
		ItemID:            "1",
		ItemName:          "Item",
		AwardedToPlayerID: actor.ID,
		AwardedByPlayerID: actor.ID,
		Source:            "legacy_import",
		AwardMethod:       Main,
		AwardedAt:         now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := repo.CreateAward(ctx, award); err != nil {
		t.Fatal(err)
	}
	if has, err := repo.PlayerHasMainSpecAward(ctx, rd.ID, actor.ID); err != nil || !has {
		t.Fatalf("active main-spec award: has=%t err=%v", has, err)
	}
	if _, err := database.DB.ExecContext(ctx, "UPDATE loot_awards SET withdrawn_at = ?, withdrawn_by_player_id = ?, withdrawn_reason = ? WHERE id = ?", now, actor.ID, "missing from source", award.ID); err != nil {
		t.Fatal(err)
	}
	if has, err := repo.PlayerHasMainSpecAward(ctx, rd.ID, actor.ID); err != nil || has {
		t.Fatalf("withdrawn main-spec award: has=%t err=%v", has, err)
	}
	stored, err := repo.Get(ctx, award.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.WithdrawnAt == nil || stored.WithdrawnByPlayerID == nil || stored.WithdrawnReason == nil || *stored.WithdrawnReason != "missing from source" {
		t.Fatalf("withdrawal metadata: %+v", stored)
	}
}
