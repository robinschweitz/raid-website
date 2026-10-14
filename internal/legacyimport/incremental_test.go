package legacyimport

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"raid-website/internal/character"
	"raid-website/internal/player"
)

const stableHeader = "Raid ID,Timestamp,Spieler,Loot,Priority,ItemId,Import ID\n"
const stableDrop = "ICC25-01,09.02.2026,Nylama,Blutfall,Main,50178,"

func importFixture(t *testing.T) (*Service, int64) {
	t.Helper()
	_, svc, players, _, _, _ := setup(t)
	actor := &player.Player{Name: "Importer", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := players.Create(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	return svc, actor.ID
}

func setLoot(s *Service, csv, hash string) {
	data := sampleSheet()
	data.Loot = []byte(csv)
	data.Hash = hash
	s.fetcher = fakeFetcher{data: data}
}

func previewImport(t *testing.T, s *Service, source, sheetID string) Report {
	t.Helper()
	rep, err := s.DryRun(context.Background(), source, sheetID)
	if err != nil || len(rep.Errors) > 0 {
		t.Fatalf("preview: %+v %v", rep, err)
	}
	return rep
}

func TestStableIncrementalImport(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	setLoot(s, stableHeader+stableDrop+"a\n", "first")
	preview, err := s.DryRun(ctx, "source", "sheet")
	if err != nil || preview.Created.Loot != 1 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if history, _ := s.List(ctx); len(history) != 0 {
		t.Fatal("preview persisted import")
	}
	first, err := s.Merge(ctx, actor, "source", "sheet", preview.RawHash, "")
	if err != nil || first.Created.Loot != 1 {
		t.Fatalf("first: %+v %v", first, err)
	}
	replay, err := s.Merge(ctx, actor, "source", "sheet", "", "")
	if err != nil || !replay.Skipped {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	replay.Skipped = false
	if !reflect.DeepEqual(first, replay) {
		t.Fatal("saved report was not returned unchanged")
	}
	setLoot(s, stableHeader+stableDrop+"a\n"+stableDrop+"b\n", "second")
	preview, err = s.DryRun(ctx, "source", "sheet")
	if err != nil || preview.Created != (Counts{Loot: 1}) {
		t.Fatalf("incremental preview: %+v %v", preview, err)
	}
	second, err := s.Merge(ctx, actor, "source", "sheet", "second", "additional historical drop")
	if err != nil || second.Created.Loot != 1 {
		t.Fatalf("second: %+v %v", second, err)
	}
	mappings, err := s.imports.mappings(ctx, "source", "sheet")
	if err != nil || len(mappings) != 2 || mappings[0].LootAwardID == mappings[1].LootAwardID {
		t.Fatalf("mapping multiplicity: %+v %v", mappings, err)
	}
}

func TestImportConflictsAreAtomic(t *testing.T) {
	for _, scenario := range []string{"duplicate", "no_id_added", "no_id_removed", "legacy", "owner"} {
		t.Run(scenario, func(t *testing.T) {
			s, actor := importFixture(t)
			ctx := context.Background()
			initial := stableHeader + stableDrop + "a\n"
			if strings.HasPrefix(scenario, "no_id") {
				initial = stableHeader + stableDrop + "\n" + stableDrop + "\n"
			}
			setLoot(s, initial, "first")
			if scenario == "legacy" {
				if err := s.imports.Insert(ctx, &Import{ActorID: actor, Source: "source", SourceURL: "sheet", RawHash: "first", Status: "MERGED"}); err != nil {
					t.Fatal(err)
				}
				replay, err := s.Merge(ctx, actor, "source", "sheet", "", "")
				if err != nil || !replay.Skipped || replay.Created != (Counts{}) || len(replay.Warnings) == 0 {
					t.Fatalf("legacy replay: %+v %v", replay, err)
				}
			} else {
				previewImport(t, s, "source", "sheet")
				if _, err := s.Merge(ctx, actor, "source", "sheet", "first", ""); err != nil {
					t.Fatal(err)
				}
			}
			changed := initial
			switch scenario {
			case "duplicate":
				changed += stableDrop + "a\n"
			case "no_id_added":
				changed += stableDrop + "\n"
			case "no_id_removed":
				changed = stableHeader + stableDrop + "\n"
			case "owner":
				other := &player.Player{Name: "Other"}
				if err := s.players.Create(ctx, other); err != nil {
					t.Fatal(err)
				}
				c, err := s.chars.FindByName(ctx, "Nylama")
				if err != nil {
					t.Fatal(err)
				}
				c.PlayerID = other.ID
				if err := s.chars.Update(ctx, c); err != nil {
					t.Fatal(err)
				}
			}
			setLoot(s, changed, "changed")
			before, _ := s.List(ctx)
			preview, err := s.DryRun(ctx, "source", "sheet")
			if err != nil || len(preview.Errors) == 0 || preview.Created != (Counts{}) {
				t.Fatalf("preview conflict: %+v %v", preview, err)
			}
			rep, err := s.Merge(ctx, actor, "source", "sheet", "changed", "")
			if !errors.Is(err, ErrPreviewRequired) || rep.Created != (Counts{}) {
				t.Fatalf("merge conflict: %+v %v", rep, err)
			}
			after, _ := s.List(ctx)
			if len(before) != len(after) {
				t.Fatal("rejected hash persisted")
			}
		})
	}
}

func TestPermanentImportIDCorrectsWithdrawsAndRestores(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	initial := stableHeader + stableDrop + "stable\n"
	setLoot(s, initial, "first")
	previewImport(t, s, "source", "sheet")
	if _, err := s.Merge(ctx, actor, "source", "sheet", "first", "initial history"); err != nil {
		t.Fatal(err)
	}

	changed := strings.ReplaceAll(initial, ",Main,", ",Second,")
	setLoot(s, changed, "changed")
	preview := previewImport(t, s, "source", "sheet")
	if preview.Updated != 1 || preview.Created.Loot != 0 {
		t.Fatalf("correction preview: %+v", preview)
	}
	rep, err := s.Merge(ctx, actor, "source", "sheet", "changed", "sheet correction")
	if err != nil || rep.Updated != 1 {
		t.Fatalf("correction merge: %+v %v", rep, err)
	}
	mappings, err := s.imports.mappings(ctx, "source", "sheet")
	if err != nil || len(mappings) != 1 {
		t.Fatalf("mapping: %+v %v", mappings, err)
	}
	award, err := s.loot.Get(ctx, mappings[0].LootAwardID)
	if err != nil || award.AwardMethod != "SECOND" || award.WithdrawnAt != nil {
		t.Fatalf("corrected award: %+v %v", award, err)
	}
	pointCorrections, err := s.imports.db.NewSelect().TableExpr("enhance_point_transactions").Where("type = ?", "correction_no_mainspec_loot").Where("reason = ?", "sheet correction").Count(ctx)
	if err != nil || pointCorrections != 1 {
		t.Fatalf("correction EP: %d %v", pointCorrections, err)
	}
	lootAudits, err := s.imports.db.NewSelect().TableExpr("audit_log").Where("entity_type = ?", "loot_award").Where("reason = ?", "sheet correction").Count(ctx)
	if err != nil || lootAudits != 1 {
		t.Fatalf("correction audit: %d %v", lootAudits, err)
	}

	setLoot(s, stableHeader, "omitted")
	preview = previewImport(t, s, "source", "sheet")
	if preview.Withdrawn != 1 {
		t.Fatalf("withdraw preview: %+v", preview)
	}
	rep, err = s.Merge(ctx, actor, "source", "sheet", "omitted", "missing from sheet")
	if err != nil || rep.Withdrawn != 1 {
		t.Fatalf("withdraw merge: %+v %v", rep, err)
	}
	award, err = s.loot.Get(ctx, award.ID)
	if err != nil || award.WithdrawnAt == nil || award.WithdrawnReason == nil || *award.WithdrawnReason != "missing from sheet" {
		t.Fatalf("withdrawn award: %+v %v", award, err)
	}

	setLoot(s, changed, "restored")
	preview = previewImport(t, s, "source", "sheet")
	if preview.Restored != 1 {
		t.Fatalf("restore preview: %+v", preview)
	}
	rep, err = s.Merge(ctx, actor, "source", "sheet", "restored", "returned to sheet")
	if err != nil || rep.Restored != 1 {
		t.Fatalf("restore merge: %+v %v", rep, err)
	}
	award, err = s.loot.Get(ctx, award.ID)
	if err != nil || award.WithdrawnAt != nil {
		t.Fatalf("restored award: %+v %v", award, err)
	}
	var events []struct{ EventType string }
	if err := s.imports.db.NewSelect().TableExpr("legacy_loot_sync_events").Column("event_type").Order("id ASC").Scan(ctx, &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].EventType != "CORRECTED" || events[1].EventType != "WITHDRAWN" || events[2].EventType != "RESTORED" {
		t.Fatalf("sync events: %+v", events)
	}
}

func TestNoIDReorderPreservesMultiplicity(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	rows := stableHeader + stableDrop + "\n" + stableDrop + "\n" + strings.ReplaceAll(stableDrop, "Blutfall", "Other") + "\n"
	setLoot(s, rows, "first")
	previewImport(t, s, "source", "sheet")
	first, err := s.Merge(ctx, actor, "source", "sheet", "first", "")
	if err != nil || first.Created.Loot != 3 {
		t.Fatalf("first: %+v %v", first, err)
	}
	setLoot(s, stableHeader+strings.ReplaceAll(stableDrop, "Blutfall", "Other")+"\n"+stableDrop+"\n"+stableDrop+"\n", "reordered")
	previewImport(t, s, "source", "sheet")
	rep, err := s.Merge(ctx, actor, "source", "sheet", "reordered", "")
	if err != nil || len(rep.Errors) > 0 || rep.Created != (Counts{}) {
		t.Fatalf("reordered: %+v %v", rep, err)
	}
}

func TestPreviewGateAndSheetScope(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	if _, err := s.Merge(ctx, actor, "source", "sheet", "", ""); !errors.Is(err, ErrPreviewRequired) {
		t.Fatal(err)
	}
	if _, err := s.Merge(ctx, actor, "source", "sheet", "old", ""); !errors.Is(err, ErrSourceChanged) {
		t.Fatal(err)
	}
	if _, err := s.Merge(ctx, actor, "source", "sheet", sampleSheet().Hash, ""); !errors.Is(err, ErrPreviewRequired) {
		t.Fatal("known hash without server preview authorized merge", err)
	}
	previewImport(t, s, "source", "sheet")
	for _, scope := range [][2]string{{"other", "sheet"}, {"source", "different"}} {
		if _, err := s.Merge(ctx, actor, scope[0], scope[1], sampleSheet().Hash, ""); !errors.Is(err, ErrPreviewRequired) {
			t.Fatal("preview leaked across scope", err)
		}
	}
	if _, err := s.Merge(ctx, actor, "source", "sheet", sampleSheet().Hash, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Merge(ctx, actor, "source", "different", "", ""); !errors.Is(err, ErrPreviewRequired) {
		t.Fatal("hash lookup leaked across sheet scope", err)
	}
}

func TestConcurrentImport(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	setLoot(s, stableHeader+stableDrop+"a\n", "first")
	previewImport(t, s, "source", "sheet")
	var wg sync.WaitGroup
	results := make(chan Report, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rep, err := s.Merge(ctx, actor, "source", "sheet", "first", "")
			results <- rep
			failures <- err
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	skipped := 0
	for rep := range results {
		if rep.Skipped {
			skipped++
		}
	}
	if skipped != 1 {
		t.Fatalf("skipped %d", skipped)
	}
	mappings, err := s.imports.mappings(ctx, "source", "sheet")
	if err != nil || len(mappings) != 1 {
		t.Fatalf("mappings: %+v %v", mappings, err)
	}
}

func TestCharacterMatchesCorrectOwner(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	other := &player.Player{Name: "Other"}
	owner := &player.Player{Name: "Nylama"}
	for _, p := range []*player.Player{other, owner} {
		if err := s.players.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []*player.Player{other, owner} {
		if err := s.chars.Create(ctx, &character.Character{Name: "Nylama", PlayerID: p.ID}); err != nil {
			t.Fatal(err)
		}
	}
	previewImport(t, s, "source", "sheet")
	rep, err := s.Merge(ctx, actor, "source", "sheet", sampleSheet().Hash, "")
	if err != nil || len(rep.Errors) > 0 || rep.Created.Characters != 1 {
		t.Fatalf("owner match: %+v %v", rep, err)
	}
}

func TestPriorityDoesNotDeriveTokens(t *testing.T) {
	s, actor := importFixture(t)
	setLoot(s, stableHeader+strings.ReplaceAll(stableDrop, "Main", "Token")+"a\n", "first")
	previewImport(t, s, "source", "sheet")
	rep, err := s.Merge(context.Background(), actor, "source", "sheet", "first", "")
	if err != nil || rep.Tokens != 0 || rep.Created.Tokens != 0 {
		t.Fatalf("tokens: %+v %v", rep, err)
	}
}
