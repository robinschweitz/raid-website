package shadowmourne_test

import (
	"context"
	"io/fs"
	"path/filepath"
	"sort"
	"testing"

	"raid-website/internal/db"
	"raid-website/internal/migrations"
)

// ADR 087: raid choices made before migration 073 keep their character.
func TestRaidCharacterMigrationKeepsExistingChoices(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "choices.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	names, err := fs.Glob(migrations.MigrationFiles, "*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	apply := func(name string) {
		t.Helper()
		content, err := migrations.MigrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.DB.ExecContext(ctx, string(content)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, name := range names {
		if name >= "073" {
			break
		}
		apply(name)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := database.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	now := "2026-10-19T18:00:00Z"
	exec("INSERT INTO players (id, name, active, created_at, updated_at) VALUES (1, 'Leader', 1, ?, ?)", now, now)
	exec("INSERT INTO characters (id, player_id, name, class, main_spec, second_spec, active, created_at, updated_at) VALUES (7, 1, 'Axtträger', 'Krieger', 'Waffen', '', 1, ?, ?)", now, now)
	exec("INSERT INTO raids (id, name, raid_leader_id, status, created_at, updated_at) VALUES (3, 'ICC', 1, 'PLANNED', ?, ?)", now, now)
	exec("INSERT INTO shadowmourne_entries (id, character_id, priority, stage, shards_collected, shard_target, created_at, updated_at) VALUES (5, 7, 1, 'BLOOD', 0, 50, ?, ?)", now, now)
	exec("INSERT INTO shadowmourne_raid_assignments (raid_id, task, entry_id, created_at, updated_at) VALUES (3, 'BLOOD', 5, ?, ?), (3, 'FROST', NULL, ?, ?)", now, now, now, now)

	apply("073_shadowmourne_raid_character.up.sql")

	var rows []struct {
		Task        string
		CharacterID *int64
	}
	if err := database.DB.NewSelect().TableExpr("shadowmourne_raid_assignments").ColumnExpr("task, character_id").OrderExpr("task").Scan(ctx, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Task != "BLOOD" || rows[0].CharacterID == nil || *rows[0].CharacterID != 7 || rows[1].CharacterID != nil {
		t.Fatalf("choices after migration: %#v", rows)
	}
}
