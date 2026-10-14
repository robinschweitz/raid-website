package loot

import (
	"context"
	"path/filepath"
	"testing"

	"raid-website/internal/db"
	"raid-website/internal/migrations"
)

func TestCatalogAwardMigrationPreservesHistoryAndForeignKey(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "catalog-migration.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	apply := func(name string) {
		t.Helper()
		content, err := migrations.MigrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.DB.ExecContext(ctx, string(content)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"001_init.up.sql", "004_raid_series.up.sql", "006_award_methods.up.sql", "007_loot_catalog.up.sql"} {
		apply(name)
	}
	for _, statement := range []string{
		"INSERT INTO players (id, name, created_at, updated_at) VALUES (1, 'Historical', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"INSERT INTO raids (id, name, raid_leader_id, status, created_at, updated_at) VALUES (1, 'Historical', 1, 'COMPLETED', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"INSERT INTO loot_awards (raid_id, boss, item_id, item_name, awarded_to_player_id, awarded_by_player_id, source, is_main_spec, award_method, awarded_at, created_at, updated_at) VALUES (1, 'Old boss', '', 'Old item', 1, 1, 'legacy_import', 1, 'MAIN', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"INSERT INTO enhance_point_transactions (player_id, amount, type, reason, actor_id, created_at) VALUES (1, 91, 'import_opening', 'old history', 1, CURRENT_TIMESTAMP)",
	} {
		if _, err := database.DB.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	for pass := 0; pass < 2; pass++ {
		apply("008_catalog_awards.up.sql")
		// Inspect the version-eight schema without requiring later columns.
		var awards []Award
		err := database.DB.NewSelect().Model(&awards).ExcludeColumn("awarded_to_character_name", "withdrawn_at", "withdrawn_by_player_id", "withdrawn_reason").Where("raid_id = ?", 1).Scan(ctx)
		if err != nil || len(awards) != 1 {
			t.Fatalf("history: %v %v", awards, err)
		}
		a := awards[0]
		if a.CatalogBossID != nil || a.Boss != "Old boss" || a.ItemName != "Old item" || a.AwardMethod != Main || a.EnhancedCost != 0 {
			t.Fatalf("history rewritten: %+v", a)
		}
		var balance, count int
		if err := database.DB.NewSelect().TableExpr("enhance_point_transactions").ColumnExpr("SUM(amount)").ColumnExpr("COUNT(*)").Scan(ctx, &balance, &count); err != nil {
			t.Fatal(err)
		}
		if balance != 91 || count != 1 {
			t.Fatalf("ledger rewritten: %d %d", balance, count)
		}
		if _, err := database.DB.ExecContext(ctx, "UPDATE loot_awards SET catalog_boss_id = 'unknown' WHERE id = ?", a.ID); err == nil {
			t.Fatal("unknown catalog boss did not violate foreign key")
		}
		if pass == 0 {
			apply("008_catalog_awards.down.sql")
		}
	}
	for pass := 0; pass < 2; pass++ {
		apply("012_loot_character_name.up.sql")
		var awards []Award
		err := database.DB.NewSelect().Model(&awards).ExcludeColumn("withdrawn_at", "withdrawn_by_player_id", "withdrawn_reason").Where("raid_id = ?", 1).Order("id ASC").Scan(ctx)
		if err != nil || len(awards) != 1 {
			t.Fatalf("character snapshot migration: %+v %v", awards, err)
		}
		if awards[0].AwardedToCharacterName != "" || awards[0].AwardedToCharacterID != nil || awards[0].AwardedToPlayerID != 1 || awards[0].ItemName != "Old item" {
			t.Fatalf("character snapshot migration fabricated history: %+v", awards[0])
		}
		if pass == 0 {
			apply("012_loot_character_name.down.sql")
		}
	}
}
