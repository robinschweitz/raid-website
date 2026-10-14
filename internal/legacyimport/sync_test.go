package legacyimport

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
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
