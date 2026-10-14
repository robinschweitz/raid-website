package legacyraidlink

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func setupService(t *testing.T) (*Service, *player.Player, *raid.Raid) {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "legacy-link.db")+"?_pragma=foreign_keys(1)")
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
	now := time.Now().UTC()
	actor := &player.Player{Name: "Admin", Role: validation.RoleAdmin, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := player.NewRepo(database.DB).Create(ctx, actor); err != nil {
		t.Fatal(err)
	}
	rd := &raid.Raid{Name: "ICC", RaidLeaderID: actor.ID, Status: validation.RaidStatusPlanned, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(database.DB).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	return NewService(database.DB), actor, rd
}

func TestCreateStoresImmutableLinkAndAudit(t *testing.T) {
	ctx := context.Background()
	service, actor, rd := setupService(t)

	created, err := service.Create(ctx, rd.ID, actor.ID, CreateInput{Source: " google_sheets ", SheetID: " sheet ", RaidKey: " ICC25-01 ", Reason: "Historischen Raid zuordnen"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Source != GoogleSheetsSource || created.SheetID != "sheet" || created.RaidKey != "ICC25-01" || created.Reason != "Historischen Raid zuordnen" {
		t.Fatalf("created link = %+v", created)
	}
	got, err := service.Get(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != created.Source || got.SheetID != created.SheetID || got.RaidKey != created.RaidKey || got.RaidID != created.RaidID || got.LinkedByPlayerID != created.LinkedByPlayerID || !got.LinkedAt.Equal(created.LinkedAt) || got.Reason != created.Reason {
		t.Fatalf("stored link = %+v, want %+v", got, created)
	}
	logs, err := audit.NewRepo(service.db).ListByEntity(ctx, "legacy_raid_link", "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Action != "link" || logs[0].Reason != created.Reason || logs[0].ActorID == nil || *logs[0].ActorID != actor.ID {
		t.Fatalf("audit logs = %+v", logs)
	}
}

func TestCreateRejectsInvalidOrConflictingLinksWithoutPartialWrite(t *testing.T) {
	ctx := context.Background()
	service, actor, rd := setupService(t)
	for _, input := range []CreateInput{
		{Source: "csv", SheetID: "sheet", RaidKey: "ICC25-01", Reason: "reason"},
		{Source: GoogleSheetsSource, RaidKey: "ICC25-01", Reason: "reason"},
		{Source: GoogleSheetsSource, SheetID: "sheet", Reason: "reason"},
		{Source: GoogleSheetsSource, SheetID: "sheet", RaidKey: "ICC25-01"},
	} {
		if _, err := service.Create(ctx, rd.ID, actor.ID, input); err == nil {
			t.Fatalf("Create(%+v) succeeded", input)
		}
	}
	if _, err := service.Get(ctx, rd.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after invalid input error = %v, want ErrNotFound", err)
	}
	if _, err := service.Create(ctx, rd.ID, actor.ID, CreateInput{Source: GoogleSheetsSource, SheetID: "sheet", RaidKey: "ICC25-01", Reason: "reason"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, rd.ID, actor.ID, CreateInput{Source: GoogleSheetsSource, SheetID: "sheet", RaidKey: "ICC25-02", Reason: "reason"}); !errors.Is(err, ErrAlreadyLinked) {
		t.Fatalf("duplicate raid error = %v, want ErrAlreadyLinked", err)
	}
}

func TestGetReturnsSyncAdoptionLink(t *testing.T) {
	ctx := context.Background()
	service, actor, rd := setupService(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := service.db.ExecContext(ctx, "INSERT INTO legacy_sync_raid_mappings (source, sheet_id, raid_key, raid_id, adopted_by_player_id, adopted_at) VALUES (?, ?, ?, ?, ?, ?)", GoogleSheetsSource, "sync-sheet", "ICC25-34", rd.ID, actor.ID, now); err != nil {
		t.Fatal(err)
	}

	link, err := service.Get(ctx, rd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if link.SheetID != "sync-sheet" || link.RaidKey != "ICC25-34" || link.LinkedByPlayerID != actor.ID || !link.LinkedAt.Equal(now) {
		t.Fatalf("sync link = %+v", link)
	}
}
