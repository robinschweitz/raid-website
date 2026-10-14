package legacyimport

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"raid-website/internal/character"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidcompletion"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

func TestSyncRejectsUnmappedRaidsWithoutBootstrapWrites(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()

	rep, err := s.DryRunSync(ctx, "source", "sheet")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rep.Errors, "\n"), "sync_unmapped_raid: ICC25-01") {
		t.Fatalf("unmapped report: %+v", rep)
	}
	if rep.Created != (Counts{}) {
		t.Fatalf("created: %+v", rep.Created)
	}
	if _, err := s.players.FindByName(ctx, "Nylama"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("sync created player: %v", err)
	}
	if _, err := s.raids.FindByName(ctx, "ICC25-01"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("sync created raid: %v", err)
	}
	merged, err := s.MergeSync(ctx, actor, "source", "sheet", rep.RawHash, "")
	if err != nil || merged.Created != (Counts{}) || len(merged.Errors) == 0 {
		t.Fatalf("unmapped merge: %+v %v", merged, err)
	}
}

func TestSyncOnlyAppliesLinkedRaidsAndChecksFreshPreview(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	setLoot(s, stableHeader+stableDrop+"a\n", "bootstrap")
	if _, err := s.DryRun(ctx, "source", "sheet"); err != nil {
		t.Fatal(err)
	}
	if rep, err := s.Merge(ctx, actor, "source", "sheet", "bootstrap", ""); err != nil || len(rep.Errors) > 0 {
		t.Fatalf("bootstrap: %+v %v", rep, err)
	}
	candidates, err := s.DryRunSyncAdoption(ctx, "source", "sheet")
	if err != nil || candidates.Raids[0].Mapped != 0 || len(candidates.Players[0].PlayerIDs) != 1 || len(candidates.Characters[0].Characters) != 1 {
		t.Fatalf("adoption dry-run: %+v %v", candidates, err)
	}
	if _, err := s.AdoptSync(ctx, actor, "source", "sheet", SyncAdoptionInput{}); !errors.Is(err, ErrAdoptionReasonRequired) {
		t.Fatalf("missing adoption reason: %v", err)
	}
	adoption, err := s.AdoptSync(ctx, actor, "source", "sheet", SyncAdoptionInput{Reason: "Synchronisierung uebernehmen"})
	if err != nil || adoption.Raids[0].Mapped == 0 || adoption.Players[0].Mapped == 0 || adoption.Characters[0].Mapped == 0 {
		t.Fatalf("adoption: %+v %v", adoption, err)
	}
	pointsBefore, err := s.imports.pointTransactionCount(ctx)
	if err != nil {
		t.Fatal(err)
	}

	setLoot(s, stableHeader+stableDrop+"a\n"+stableDrop+"b\n", "sync-preview")
	preview, err := s.DryRunSync(ctx, "source", "sheet")
	if err != nil || len(preview.Errors) > 0 || preview.Created.Loot != 1 {
		t.Fatalf("sync preview: %+v %v", preview, err)
	}
	setLoot(s, stableHeader+stableDrop+"a\n"+stableDrop+"b\n", "sync-changed")
	if _, err := s.MergeSync(ctx, actor, "source", "sheet", preview.RawHash, "sheet correction"); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("changed source: %v", err)
	}

	preview, err = s.DryRunSync(ctx, "source", "sheet")
	if err != nil || len(preview.Errors) > 0 {
		t.Fatalf("changed preview: %+v %v", preview, err)
	}
	merged, err := s.MergeSync(ctx, actor, "source", "sheet", preview.RawHash, "sheet correction")
	if err != nil || len(merged.Errors) > 0 || merged.Created != (Counts{Loot: 1}) {
		t.Fatalf("sync merge: %+v %v", merged, err)
	}
	pointsAfter, err := s.imports.pointTransactionCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pointsAfter != pointsBefore {
		t.Fatalf("sync imported opening points: before=%d after=%d", pointsBefore, pointsAfter)
	}
}

func TestAdoptSyncKeepsDifferentLegacyRaidProvenance(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	setLoot(s, stableHeader+stableDrop+"a\n", "bootstrap")
	if _, err := s.DryRun(ctx, "source", "old-sheet"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Merge(ctx, actor, "source", "old-sheet", "bootstrap", ""); err != nil {
		t.Fatal(err)
	}
	adoption, err := s.AdoptSync(ctx, actor, "source", "sheet", SyncAdoptionInput{Reason: "Neues Sheet uebernehmen"})
	if err != nil || adoption.Raids[0].Mapped == 0 {
		t.Fatalf("adoption: %+v %v", adoption, err)
	}
	sheetIDs, err := s.ListSyncSheetIDs(ctx, "source")
	if err != nil || len(sheetIDs) != 1 || sheetIDs[0] != "sheet" {
		t.Fatalf("adopted sheets: %+v %v", sheetIDs, err)
	}
	audits, err := s.imports.db.NewSelect().TableExpr("audit_log").Where("entity_type = ?", "legacy_sync_mapping").Count(ctx)
	if err != nil || audits != 5 { // raid, two players, and two characters.
		t.Fatalf("adoption audit rows=%d err=%v", audits, err)
	}
	var oldSheet string
	if err := s.imports.db.NewSelect().TableExpr("legacy_raid_mappings").Column("sheet_id").Scan(ctx, &oldSheet); err != nil || oldSheet != "old-sheet" {
		t.Fatalf("legacy provenance=%q err=%v", oldSheet, err)
	}
	preview, err := s.DryRunSync(ctx, "source", "sheet")
	if err != nil || len(preview.Errors) != 0 {
		t.Fatalf("sync preview: %+v %v", preview, err)
	}
}

func TestSyncFinalizesActiveMappedRaidAndReconcilesSuccessorSetup(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	data := sampleSheet()
	data.Raid = []byte(strings.ReplaceAll(string(data.Raid), "ICC25-01", "ICC25-35"))
	// An archived Raid Archive row is sufficient to finalize a no-loot raid.
	data.Loot = []byte(strings.SplitN(string(data.Loot), "\n", 2)[0] + "\n")
	data.Setup = []byte(",Charakter,Spieler,Klasse,Spec,Rolle\n1,Nylama,Nylama,Todesritter,Blut (Tank),Tank\n2,Amisu,Amisu,Priester,Disziplin,Heiler\n")
	data.Hash = "active-completion"
	s.fetcher = fakeFetcher{data: data}

	series := &raidseries.Series{Name: "ICC25", RaidLeaderID: actor, Weekdays: []int{1}, IntervalWeeks: 1, StartTime: "19:00", Timezone: "UTC", NameTemplate: "ICC25-{number}", NextRaidNumber: 36}
	if err := s.series.Save(ctx, series); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.October, 12, 19, 0, 0, 0, time.UTC)
	active := &raid.Raid{Name: "ICC25-35", RaidLeaderID: actor, SeriesID: &series.ID, Status: validation.RaidStatusOpen, ScheduledStart: &start, CreatedAt: start, UpdatedAt: start}
	if err := s.raids.Create(ctx, active); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := s.imports.insertSyncRaidMapping(ctx, "source", "sheet", "ICC25-35", active.ID, actor, now); err != nil {
		t.Fatal(err)
	}
	for _, identity := range []struct{ name, class string }{{"Nylama", "Todesritter"}, {"Amisu", "Priester"}} {
		playerRow := &player.Player{Name: identity.name, Role: validation.RoleGuest, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := s.players.Create(ctx, playerRow); err != nil {
			t.Fatal(err)
		}
		characterRow := &character.Character{PlayerID: playerRow.ID, Name: identity.name, Class: identity.class, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := s.chars.Create(ctx, characterRow); err != nil {
			t.Fatal(err)
		}
		if err := s.imports.insertSyncPlayerMapping(ctx, "source", "sheet", identity.name, playerRow.ID, actor, now); err != nil {
			t.Fatal(err)
		}
		if err := s.imports.insertSyncCharacterMapping(ctx, "source", "sheet", identity.name, identity.name, characterRow.ID, playerRow.ID, actor, now); err != nil {
			t.Fatal(err)
		}
	}
	s.WithCompletionService(raidcompletion.NewService(raidcompletion.NewRepo(s.imports.db)))

	preview, err := s.DryRunSync(ctx, "source", "sheet")
	if err != nil || len(preview.Errors) > 0 || preview.FinalizedRaids != 1 || preview.Setup == nil || preview.Setup.Name != "ICC25-36" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	merged, err := s.MergeSync(ctx, actor, "source", "sheet", preview.RawHash, "")
	if err != nil || len(merged.Errors) > 0 || merged.FinalizedRaids != 1 {
		t.Fatalf("merge: %+v %v", merged, err)
	}
	completed, err := s.raids.Get(ctx, active.ID)
	if err != nil || completed.Status != validation.RaidStatusCompleted || completed.ArchivedAt == nil {
		t.Fatalf("completed raid: %+v %v", completed, err)
	}
	successorKey, err := s.imports.syncRaidMapping(ctx, "source", "sheet", "ICC25-36")
	if err != nil {
		t.Fatal(err)
	}
	successor, err := s.raids.Get(ctx, successorKey.RaidID)
	if err != nil || successor.Status != validation.RaidStatusPlanned {
		t.Fatalf("successor: %+v %v", successor, err)
	}
	roster, err := s.att.ListByRaid(ctx, successor.ID)
	if err != nil || len(roster) != 2 {
		t.Fatalf("successor roster: %+v %v", roster, err)
	}
	count, err := s.imports.pointTransactionCount(ctx)
	if err != nil || count != 2 {
		t.Fatalf("EP award count=%d err=%v", count, err)
	}
	replay, err := s.MergeSync(ctx, actor, "source", "sheet", preview.RawHash, "")
	if err != nil || !replay.Skipped {
		t.Fatalf("replay: %+v %v", replay, err)
	}
}
