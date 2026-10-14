package loot

import (
	"context"
	"path/filepath"
	"testing"

	"raid-website/internal/db"
	"raid-website/internal/migrations"
)

func TestAwardMethodMigrationPreservesHistory(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "migration.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	initial, err := migrations.MigrationFiles.ReadFile("001_init.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, string(initial)); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"INSERT INTO players (id, name, created_at, updated_at) VALUES (1, 'Historical', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"INSERT INTO raids (id, name, raid_leader_id, status, created_at, updated_at) VALUES (1, 'Historical', 1, 'COMPLETED', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"INSERT INTO loot_awards (raid_id, boss, item_id, item_name, awarded_to_player_id, awarded_by_player_id, source, is_main_spec, awarded_at, created_at, updated_at) VALUES (1, '', '', 'main', 1, 1, 'legacy_import', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP), (1, '', '', 'second', 1, 1, 'legacy_import', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"INSERT INTO enhance_point_transactions (player_id, amount, type, reason, actor_id, created_at) VALUES (1, -50, 'enhance_roll', 'old roll', 1, CURRENT_TIMESTAMP)",
	} {
		if _, err := database.DB.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	up, err := migrations.MigrationFiles.ReadFile("006_award_methods.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	// Inspect the version-six schema without requiring later columns.
	var awards []Award
	err = database.DB.NewSelect().Model(&awards).ExcludeColumn("catalog_boss_id", "awarded_to_character_name", "withdrawn_at", "withdrawn_by_player_id", "withdrawn_reason").Where("raid_id = ?", 1).Order("id ASC").Scan(ctx)
	if err != nil || len(awards) != 2 {
		t.Fatalf("awards %v error %v", awards, err)
	}
	if awards[0].AwardMethod != Main || awards[1].AwardMethod != Second {
		t.Fatalf("inferred methods: %v", awards)
	}
	for _, award := range awards {
		if award.EnhancedCost != 0 || award.AwardedToCharacterID != nil || award.RequestID != nil {
			t.Fatalf("fabricated association: %+v", award)
		}
	}
	var amount, count int
	var linked *int64
	if err := database.DB.NewSelect().TableExpr("enhance_point_transactions").Column("amount", "loot_award_id").Where("type = 'enhance_roll'").Scan(ctx, &amount, &linked); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.NewSelect().TableExpr("enhance_point_transactions").ColumnExpr("COUNT(*)").Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if amount != -50 || linked != nil || count != 1 {
		t.Fatal("historical ledger changed")
	}
	down, err := migrations.MigrationFiles.ReadFile("006_award_methods.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
}
