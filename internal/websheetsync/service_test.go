package websheetsync

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"raid-website/internal/db"
	"raid-website/internal/googlesheets"
	"raid-website/internal/loot"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestFlushAppendsThenUpdatesPermanentImportID(t *testing.T) {
	ctx, database, actor, award := deliveryFixture(t)
	fake := &fakeSheets{values: [][]string{{"Raid ID", "Timestamp", "Spieler", "Loot", "Priority", "ItemID", "Import ID"}}}
	service := NewService(database.DB, fake)
	if err := service.EnqueueAward(ctx, database.DB, award, actor.ID); err != nil {
		t.Fatal(err)
	}
	result, err := service.Flush(ctx)
	if err != nil || result != (FlushResult{Sent: 1}) {
		t.Fatalf("first flush result=%+v err=%v", result, err)
	}
	if len(fake.appended) != 2 { // Loot Archive and Website Sync Audit.
		t.Fatalf("append calls = %d, want 2", len(fake.appended))
	}
	lootRow := fake.appended[0][0]
	if lootRow[0] != "ICC25-01" || lootRow[6] != "website-award-1" || lootRow[10] != "ACTIVE" {
		t.Fatalf("loot row = %#v", lootRow)
	}
	if lootRow[1] != "09.02.2026" {
		t.Fatalf("loot timestamp = %q, want German sheet date", lootRow[1])
	}
	var mapping struct {
		Identity    string `bun:"identity"`
		ExternalID  string `bun:"external_id"`
		Fingerprint string `bun:"fingerprint"`
		LootAwardID int64  `bun:"loot_award_id"`
	}
	if err := database.DB.NewSelect().TableExpr("legacy_loot_mappings").Column("identity", "external_id", "fingerprint", "loot_award_id").Scan(ctx, &mapping); err != nil {
		t.Fatal(err)
	}
	expectedIdentity := fmt.Sprintf("id:%x", sha256.Sum256([]byte("website-award-1")))
	fingerprintSource := []byte(`["ICC25-01","09.02.2026","Nylama","Original item","123","SECOND"]`)
	expectedFingerprint := fmt.Sprintf("%x", sha256.Sum256(fingerprintSource))
	if mapping.Identity != expectedIdentity || mapping.ExternalID != "website-award-1" || mapping.Fingerprint != expectedFingerprint || mapping.LootAwardID != award.ID {
		t.Fatalf("mapping = %+v", mapping)
	}

	award.ItemName = "Corrected item"
	award.UpdatedAt = award.UpdatedAt.Add(time.Minute)
	if err := service.EnqueueAward(ctx, database.DB, award, actor.ID); err != nil {
		t.Fatal(err)
	}
	fake.values = append(fake.values, lootRow)
	result, err = service.Flush(ctx)
	if err != nil || result != (FlushResult{Sent: 1}) {
		t.Fatalf("second flush result=%+v err=%v", result, err)
	}
	if len(fake.updated) != 1 || fake.updated[0][0][3] != "Corrected item" {
		t.Fatalf("updates = %#v", fake.updated)
	}
}

func TestEnqueueAwardRejectsMappingForAnotherAward(t *testing.T) {
	ctx, database, actor, award := deliveryFixture(t)
	other := *award
	other.ID = 0
	if err := loot.NewRepo(database.DB).CreateAward(ctx, &other); err != nil {
		t.Fatal(err)
	}
	_, identity, err := legacyLootIdentity("ICC25-01", "09.02.2026", "Nylama", "Original item", "123", "SECOND", "website-award-1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.DB.ExecContext(ctx, "INSERT INTO legacy_loot_mappings (source, sheet_id, tab, identity, external_id, fingerprint, loot_award_id) VALUES (?, ?, ?, ?, ?, ?, ?)", "google_sheets", "sheet-1", lootArchiveTab, identity, "website-award-1", "old", other.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = NewService(database.DB, nil).EnqueueAward(ctx, database.DB, award, actor.ID)
	if err == nil || !strings.Contains(err.Error(), "belongs to award") {
		t.Fatalf("EnqueueAward error = %v, want mapping conflict", err)
	}
	count, err := database.DB.NewSelect().TableExpr("website_loot_delivery_outbox").Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("outbox rows = %d, want 0 after conflict", count)
	}
}

func TestFlushRetainsFailureForRetry(t *testing.T) {
	ctx, database, actor, award := deliveryFixture(t)
	fake := &fakeSheets{err: errors.New("permission denied")}
	service := NewService(database.DB, fake)
	if err := service.EnqueueAward(ctx, database.DB, award, actor.ID); err != nil {
		t.Fatal(err)
	}
	result, err := service.Flush(ctx)
	if err != nil || result != (FlushResult{Failed: 1}) {
		t.Fatalf("flush result=%+v err=%v", result, err)
	}
	var row struct {
		Status   string `bun:"status"`
		Attempts int    `bun:"attempt_count"`
		Error    string `bun:"attempt_error"`
	}
	if err := database.DB.NewSelect().TableExpr("website_loot_delivery_outbox").Column("status", "attempt_count", "attempt_error").Scan(ctx, &row); err != nil {
		t.Fatal(err)
	}
	if row.Status != "FAILED" || row.Attempts != 1 || row.Error != "permission denied" {
		t.Fatalf("outbox row = %+v", row)
	}
}

func TestNewerOperationSupersedesUnsentAwardVersion(t *testing.T) {
	ctx, database, actor, award := deliveryFixture(t)
	fake := &fakeSheets{values: [][]string{{"Raid ID", "Timestamp", "Spieler", "Loot", "Priority", "ItemID", "Import ID"}}}
	service := NewService(database.DB, fake)
	if err := service.EnqueueAward(ctx, database.DB, award, actor.ID); err != nil {
		t.Fatal(err)
	}
	award.ItemName = "Latest item"
	award.UpdatedAt = award.UpdatedAt.Add(time.Minute)
	if err := service.EnqueueAward(ctx, database.DB, award, actor.ID); err != nil {
		t.Fatal(err)
	}
	result, err := service.Flush(ctx)
	if err != nil || result != (FlushResult{Sent: 1}) {
		t.Fatalf("flush result=%+v err=%v", result, err)
	}
	if got := fake.appended[0][0][3]; got != "Latest item" {
		t.Fatalf("sent stale item %q", got)
	}
	superseded, err := database.DB.NewSelect().TableExpr("website_loot_delivery_outbox").Where("status = 'SUPERSEDED'").Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if superseded != 1 {
		t.Fatalf("superseded rows = %d, want 1", superseded)
	}
}

func deliveryFixture(t *testing.T) (context.Context, *db.BunDB, *player.Player, *loot.Award) {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "delivery.db")+"?_pragma=foreign_keys(1)")
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
	now := time.Date(2026, time.February, 9, 12, 0, 0, 0, time.UTC)
	actor := &player.Player{Name: "Actor", Role: validation.RoleAdmin, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := player.NewRepo(database.DB).Create(ctx, actor); err != nil {
		t.Fatal(err)
	}
	rd := &raid.Raid{Name: "Linked", RaidLeaderID: actor.ID, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(database.DB).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	award := &loot.Award{RaidID: rd.ID, Boss: "Boss", ItemID: "123", ItemName: "Original item", AwardedToPlayerID: actor.ID, AwardedToCharacterName: "Nylama", AwardedByPlayerID: actor.ID, Source: "manual", AwardMethod: loot.Second, AwardedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := loot.NewRepo(database.DB).CreateAward(ctx, award); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, "INSERT INTO legacy_sync_raid_mappings (source, sheet_id, raid_key, raid_id, adopted_by_player_id, adopted_at) VALUES ('google_sheets', 'sheet-1', 'ICC25-01', ?, ?, ?)", rd.ID, actor.ID, now); err != nil {
		t.Fatal(err)
	}
	return ctx, database, actor, award
}

type fakeSheets struct {
	values   [][]string
	appended [][][]string
	updated  [][][]string
	err      error
}

func (f *fakeSheets) Values(context.Context, string, string) ([][]string, error) {
	return f.values, f.err
}
func (f *fakeSheets) UpdateValues(_ context.Context, _ string, _ string, rows [][]string) error {
	f.updated = append(f.updated, rows)
	return f.err
}
func (f *fakeSheets) AppendRows(_ context.Context, _ string, _ string, rows [][]string) error {
	f.appended = append(f.appended, rows)
	return f.err
}
func (f *fakeSheets) AddSheet(context.Context, string, string) error {
	return googlesheets.ErrSheetExists
}
