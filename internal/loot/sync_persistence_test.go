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

func TestLegacyLootSyncRecordsAreAppendOnly(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "sync-records.db")+"?_pragma=foreign_keys(1)")
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
	rd := &raid.Raid{Name: "Linked website raid", RaidLeaderID: actor.ID, Status: validation.RaidStatusCompleted, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(database.DB).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	award := &Award{RaidID: rd.ID, Boss: "Boss", ItemID: "1", ItemName: "Item", AwardedToPlayerID: actor.ID, AwardedByPlayerID: actor.ID, Source: "legacy_import", AwardMethod: Main, AwardedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := NewRepo(database.DB).CreateAward(ctx, award); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, "INSERT INTO legacy_raid_mappings (source, sheet_id, raid_key, raid_id) VALUES (?, ?, ?, ?)", "sheet", "source-id", "ICC25-01", rd.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, "INSERT INTO legacy_raid_sync_links (source, sheet_id, raid_key, linked_by_player_id, linked_at) VALUES (?, ?, ?, ?, ?)", "sheet", "source-id", "ICC25-01", actor.ID, now); err != nil {
		t.Fatal(err)
	}
	result, err := database.DB.ExecContext(ctx, "INSERT INTO legacy_loot_mappings (source, sheet_id, tab, identity, external_id, fingerprint, loot_award_id) VALUES (?, ?, ?, ?, ?, ?, ?)", "sheet", "source-id", "Loot Archive", "import-id:123", "123", "fingerprint", award.ID)
	if err != nil {
		t.Fatal(err)
	}
	mappingID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	result, err = database.DB.ExecContext(ctx, "INSERT INTO legacy_loot_sync_states (source, sheet_id, raid_key, raw_hash, synced_by_player_id, synced_at) VALUES (?, ?, ?, ?, ?, ?)", "sheet", "source-id", "ICC25-01", "hash", actor.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	stateID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, "INSERT INTO legacy_loot_sync_events (sync_state_id, legacy_loot_mapping_id, loot_award_id, event_type, actor_id, created_at) VALUES (?, ?, ?, ?, ?, ?)", stateID, mappingID, award.ID, "WITHDRAWN", actor.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, "UPDATE legacy_loot_sync_states SET raw_hash = ? WHERE id = ?", "changed", stateID); err == nil {
		t.Fatal("updated append-only sync state")
	}
	if _, err := database.DB.ExecContext(ctx, "DELETE FROM legacy_loot_sync_events WHERE sync_state_id = ?", stateID); err == nil {
		t.Fatal("deleted append-only sync event")
	}
}
