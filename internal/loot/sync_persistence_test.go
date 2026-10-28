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


// Bonus eligibility ignores tokens and shards even when imported as MAIN.
func TestTokensAndShardsDoNotBlockNoMainSpecBonus(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "eligibility.db")+"?_pragma=foreign_keys(1)")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = database.Close() })
	migrator, err := migrations.NewMigrator(database.DB)
	if err != nil { t.Fatal(err) }
	if _, err := migrator.Migrate(ctx); err != nil { t.Fatal(err) }
	now := time.Now().UTC().Truncate(time.Second)
	actor := &player.Player{Name:"Eligibility actor", Role:validation.RoleAdmin, Active:true, CreatedAt:now, UpdatedAt:now}
	if err := player.NewRepo(database.DB).Create(ctx, actor); err != nil { t.Fatal(err) }
	rd := &raid.Raid{Name:"Eligibility raid", RaidLeaderID:actor.ID, Status:validation.RaidStatusCompleted, CreatedAt:now, UpdatedAt:now}
	if err := raid.NewRepo(database.DB).Create(ctx, rd); err != nil { t.Fatal(err) }
	repo := NewRepo(database.DB)
	for _, tc := range []struct{ id, name string }{
		{"52027", "Weiheabzeichen des Eroberers"},
		{"52030", "Weiheabzeichen des Beschützers"},
		{"52029", "Weiheabzeichen des Bezwingers"},
		{"50274", "Schattenfrostsplitter"},
		{"", "Seelensplitter"},
		{"", "Shadowfrost Shard"},
	} {
		award := &Award{RaidID:rd.ID, Boss:"Boss", ItemID:tc.id, ItemName:tc.name, AwardedToPlayerID:actor.ID, AwardedByPlayerID:actor.ID, Source:"legacy_import", AwardMethod:Main, AwardedAt:now, CreatedAt:now, UpdatedAt:now}
		if err := repo.CreateAward(ctx, award); err != nil { t.Fatal(err) }
		if has, err := repo.PlayerHasMainSpecAward(ctx, rd.ID, actor.ID); err != nil || has { t.Fatalf("%q should not block +10 EP: has=%t err=%v", tc.name, has, err) }
	}
	ordinary := &Award{RaidID:rd.ID, Boss:"Boss", ItemID:"123", ItemName:"Ordinary main-spec loot", AwardedToPlayerID:actor.ID, AwardedByPlayerID:actor.ID, Source:"legacy_import", AwardMethod:Main, AwardedAt:now, CreatedAt:now, UpdatedAt:now}
	if err := repo.CreateAward(ctx, ordinary); err != nil { t.Fatal(err) }
	if has, err := repo.PlayerHasMainSpecAward(ctx, rd.ID, actor.ID); err != nil || !has { t.Fatalf("ordinary main-spec loot should block +10 EP: has=%t err=%v", has, err) }
	if _, err := database.DB.ExecContext(ctx, "UPDATE loot_awards SET withdrawn_at = ? WHERE id = ?", now, ordinary.ID); err != nil { t.Fatal(err) }
	if has, err := repo.PlayerHasMainSpecAward(ctx, rd.ID, actor.ID); err != nil || has { t.Fatalf("withdrawn ordinary loot should not block +10 EP: has=%t err=%v", has, err) }
}
