package raidseries_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"raid-website/internal/db"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

func TestSeriesMigrationPreservesLegacyHistoryWithoutDefaults(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "legacy.db"))
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
	for _, name := range []string{"001_init.up.sql", "002_auth.up.sql", "003_import_identity.up.sql"} {
		apply(name)
	}
	now := time.Date(2026, 1, 5, 19, 0, 0, 0, time.UTC)
	actor := &player.Player{Name: "Existing leader", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := player.NewRepo(database.DB).Create(ctx, actor); err != nil {
		t.Fatal(err)
	}
	for _, status := range []validation.RaidStatus{validation.RaidStatusCompleted, validation.RaidStatusOpen} {
		var completed, archived *time.Time
		if status == validation.RaidStatusCompleted {
			completed = &now
		} else {
			archived = &now
		}
		if _, err := database.DB.ExecContext(ctx, "INSERT INTO raids (name, raid_leader_id, status, scheduled_start, completed_at, archived_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", string(status), actor.ID, status, now, completed, archived, now, now); err != nil {
			t.Fatal(err)
		}
	}
	opening := &enhancepoints.Transaction{PlayerID: actor.ID, Amount: 91, Type: "import_opening", Reason: "Opening balance", ActorID: actor.ID, CreatedAt: now}
	if err := enhancepoints.NewRepo(database.DB).InsertTransaction(ctx, opening); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		apply("004_raid_series.up.sql")
		series, err := raidseries.NewRepo(database.DB).List(ctx)
		if err != nil || len(series) != 0 {
			t.Fatalf("migration seeded guild defaults: %+v %v", series, err)
		}
		stored, err := raid.NewRepo(database.DB).List(ctx, false)
		if err != nil || len(stored) != 2 {
			t.Fatalf("history changed: %+v %v", stored, err)
		}
		for _, rd := range stored {
			if rd.SeriesID != nil || rd.PreviousRaidID != nil || !rd.UpdatedAt.Equal(now) || string(rd.Status) != rd.Name {
				t.Fatalf("migration rewrote history: %+v", rd)
			}
			if rd.Status == validation.RaidStatusCompleted && (rd.CompletedAt == nil || rd.ArchivedAt != nil) {
				t.Fatalf("completed raid changed: %+v", rd)
			}
			if rd.Status == validation.RaidStatusOpen && (rd.CompletedAt != nil || rd.ArchivedAt == nil) {
				t.Fatalf("old archived active raid changed: %+v", rd)
			}
		}
		balance, err := enhancepoints.NewRepo(database.DB).Balance(ctx, actor.ID)
		if err != nil || balance != 91 {
			t.Fatalf("migration wrote rewards: %d %v", balance, err)
		}
		if pass == 0 {
			apply("004_raid_series.down.sql")
		}
	}
}
