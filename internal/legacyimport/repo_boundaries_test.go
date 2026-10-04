package legacyimport

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"raid-website/internal/enhancepoints"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func seedImportedRaid(t *testing.T, s *Service, actor int64) int64 {
	t.Helper()
	setLoot(s, stableHeader+stableDrop+"a\n", "first")
	previewImport(t, s, "source", "sheet")
	rep, err := s.Merge(context.Background(), actor, "source", "sheet", "first", "")
	if err != nil || len(rep.Errors) > 0 {
		t.Fatalf("seed: %+v %v", rep, err)
	}
	mapping, err := s.imports.raidMapping(context.Background(), "source", "sheet", "ICC25-01")
	if err != nil {
		t.Fatal(err)
	}
	return mapping.RaidID
}

func TestUnrelatedExistingRaidNameIsConflict(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	row := &raid.Raid{Name: "ICC25-01", RaidLeaderID: actor, Status: validation.RaidStatusCompleted}
	if err := s.raids.Create(ctx, row); err != nil {
		t.Fatal(err)
	}
	rep, err := s.DryRun(ctx, "source", "sheet")
	if err != nil || len(rep.Errors) == 0 || !strings.Contains(strings.Join(rep.Errors, "\n"), "provenance") || rep.Created != (Counts{}) {
		t.Fatalf("collision: %+v %v", rep, err)
	}
	if rows, err := s.att.ListByRaid(ctx, row.ID); err != nil || len(rows) != 0 {
		t.Fatalf("attendance: %+v %v", rows, err)
	}
	if rows, err := s.loot.ListByRaid(ctx, row.ID); err != nil || len(rows) != 0 {
		t.Fatalf("loot: %+v %v", rows, err)
	}
}

func TestMappedRaidLifecycleRejectsAdditions(t *testing.T) {
	for _, archived := range []bool{false, true} {
		name := "cancelled"
		if archived {
			name = "archived"
		}
		t.Run(name, func(t *testing.T) {
			s, actor := importFixture(t)
			ctx := context.Background()
			raidID := seedImportedRaid(t, s, actor)
			rd, err := s.raids.Get(ctx, raidID)
			if err != nil {
				t.Fatal(err)
			}
			if archived {
				now := time.Now().UTC()
				rd.ArchivedAt = &now
			} else {
				rd.Status = validation.RaidStatusCancelled
			}
			if err := s.raids.Update(ctx, rd); err != nil {
				t.Fatal(err)
			}
			// A changed snapshot containing only already-known rows is harmless.
			setLoot(s, stableHeader+stableDrop+"a\n", "unchanged-rows")
			previewImport(t, s, "source", "sheet")
			if rep, err := s.Merge(ctx, actor, "source", "sheet", "unchanged-rows", ""); err != nil || rep.Created != (Counts{}) {
				t.Fatalf("unchanged: %+v %v", rep, err)
			}
			setLoot(s, stableHeader+stableDrop+"a\n"+stableDrop+"b\n", "new-loot")
			rep, err := s.DryRun(ctx, "source", "sheet")
			if err != nil || len(rep.Errors) == 0 || rep.Created != (Counts{}) {
				t.Fatalf("lifecycle: %+v %v", rep, err)
			}
			// Valid preview can also become invalid before merge.
			if err := s.imports.recordPreview(ctx, "source", "sheet", "new-loot"); err != nil {
				t.Fatal(err)
			}
			rep, err = s.Merge(ctx, actor, "source", "sheet", "new-loot", "historical correction")
			if err != nil || len(rep.Errors) == 0 || rep.Created != (Counts{}) {
				t.Fatalf("merge: %+v %v", rep, err)
			}
			awards, err := s.loot.ListByRaid(ctx, raidID)
			if err != nil || len(awards) != 1 {
				t.Fatalf("loot changed: %+v %v", awards, err)
			}
			data := sampleSheet()
			data.Hash = "new-attendance"
			data.Loot = []byte(stableHeader + stableDrop + "a\n")
			data.Character = append(data.Character, []byte("Newchar,Newplayer,Priester,Disziplin,,,\n")...)
			data.Raid = append(data.Raid, []byte("ICC25-01,09.02.2026,3,Newchar,Heiler,Priester,Disziplin,\n")...)
			s.fetcher = fakeFetcher{data: data}
			rep, err = s.DryRun(ctx, "source", "sheet")
			if err != nil || len(rep.Errors) == 0 || rep.Created != (Counts{}) {
				t.Fatalf("attendance lifecycle: %+v %v", rep, err)
			}
			if err := s.imports.recordPreview(ctx, "source", "sheet", data.Hash); err != nil {
				t.Fatal(err)
			}
			rep, err = s.Merge(ctx, actor, "source", "sheet", data.Hash, "historical attendee")
			if err != nil || len(rep.Errors) == 0 || rep.Created != (Counts{}) {
				t.Fatalf("attendance merge: %+v %v", rep, err)
			}
			if _, err := s.players.FindByName(ctx, "Newplayer"); err == nil {
				t.Fatal("forbidden attendance created player")
			}
		})
	}
}

func TestIncrementalCorrectionReasonEPAndAudit(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	raidID := seedImportedRaid(t, s, actor)
	Amisu, err := s.players.FindByName(ctx, "Amisu")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := s.points.ListByPlayer(ctx, Amisu.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(initial) != 1 || initial[0].Type != "legacy_open" {
		t.Fatalf("initial import backfilled raid EP: %+v", initial)
	}
	if err := s.points.InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: Amisu.ID, RaidID: &raidID, Amount: 10, Type: "award_no_mainspec_loot", ActorID: actor, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	newDrop := "ICC25-01,09.07.2026,Amisu,Weiheabzeichen des Eroberers,Main,52027,b\n"
	setLoot(s, stableHeader+stableDrop+"a\n"+newDrop, "second")
	preview := previewImport(t, s, "source", "sheet")
	if preview.Created.Points != 1 {
		t.Fatalf("preview omitted EP compensation: %+v", preview)
	}
	if !strings.Contains(strings.Join(preview.Warnings, "\n"), "reason_required") {
		t.Fatal("missing preview warning")
	}
	rep, err := s.Merge(ctx, actor, "source", "sheet", "second", "  ")
	if err != nil || len(rep.Errors) == 0 || rep.Created != (Counts{}) {
		t.Fatalf("missing reason: %+v %v", rep, err)
	}
	if history, err := s.List(ctx); err != nil || len(history) != 1 {
		t.Fatalf("rejected import persisted: %+v %v", history, err)
	}
	reason := "additional historical main-spec token"
	rep, err = s.Merge(ctx, actor, "source", "sheet", "second", reason)
	if err != nil || len(rep.Errors) > 0 || rep.Created.Loot != 1 || rep.Created.Tokens != 1 || rep.Created.Points != 1 {
		t.Fatalf("correction: %+v %v", rep, err)
	}
	balance, err := s.points.Balance(ctx, Amisu.ID)
	if err != nil || balance != 30 {
		t.Fatalf("EP delta: %d %v", balance, err)
	}
	awards, err := s.loot.ListByRaid(ctx, raidID)
	if err != nil || len(awards) != 2 {
		t.Fatalf("awards: %+v %v", awards, err)
	}
	var importedID int64
	for _, award := range awards {
		if award.AwardedToPlayerID == Amisu.ID {
			importedID = award.ID
			if award.Source != "legacy_import" || award.AwardedAt.UTC().Format(time.RFC3339) != "2026-07-08T22:00:00Z" {
				t.Fatalf("historical metadata overwritten: %+v", award)
			}
		}
	}
	var logs []struct {
		ActorID    int64
		Reason     string
		BeforeJSON string
		AfterJSON  string
	}
	if err := s.imports.db.NewSelect().TableExpr("audit_log").Column("actor_id", "reason", "before_json", "after_json").Where("entity_type = ?", "loot_award").Scan(ctx, &logs); err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].ActorID != actor || logs[0].Reason != reason || logs[0].BeforeJSON != "null" || !json.Valid([]byte(logs[0].AfterJSON)) {
		t.Fatalf("audit: %+v", logs)
	}
	txns, err := s.tokens.ListByPlayer(ctx, Amisu.ID)
	if err != nil || len(txns) != 1 || txns[0].LootAwardID == nil || *txns[0].LootAwardID != importedID {
		t.Fatalf("tokens: %+v %v", txns, err)
	}
	history, err := s.List(ctx)
	if err != nil || history[0].Notes != reason {
		t.Fatalf("reason not saved: %+v %v", history, err)
	}
	replay, err := s.Merge(ctx, actor, "source", "sheet", "", "")
	if err != nil || !replay.Skipped {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	balance, err = s.points.Balance(ctx, Amisu.ID)
	if err != nil || balance != 30 {
		t.Fatalf("replay repeated correction: %d %v", balance, err)
	}
}

func TestIncrementalAttendanceIsAudited(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	raidID := seedImportedRaid(t, s, actor)
	data := sampleSheet()
	data.Hash = "attendance"
	data.Loot = []byte(stableHeader + stableDrop + "a\n")
	data.Character = append(data.Character, []byte("Newchar,Newplayer,Priester,Disziplin,,,\n")...)
	data.Raid = append(data.Raid, []byte("ICC25-01,09.02.2026,3,Newchar,Heiler,Priester,Disziplin,\n")...)
	s.fetcher = fakeFetcher{data: data}
	previewImport(t, s, "source", "sheet")
	rep, err := s.Merge(ctx, actor, "source", "sheet", "attendance", "")
	if err != nil || len(rep.Errors) == 0 {
		t.Fatalf("missing reason: %+v %v", rep, err)
	}
	if _, err := s.players.FindByName(ctx, "Newplayer"); err == nil {
		t.Fatal("missing reason wrote player")
	}
	rep, err = s.Merge(ctx, actor, "source", "sheet", "attendance", "missing attendee")
	if err != nil || len(rep.Errors) > 0 || rep.Created.Attendance != 1 {
		t.Fatalf("attendance: %+v %v", rep, err)
	}
	pl, err := s.players.FindByName(ctx, "Newplayer")
	if err != nil {
		t.Fatal(err)
	}
	balance, err := s.points.Balance(ctx, pl.ID)
	if err != nil || balance != 10 {
		t.Fatalf("attendance eligibility delta: %d %v", balance, err)
	}
	count, err := s.imports.db.NewSelect().TableExpr("audit_log").Where("entity_type = ?", "attendance").Where("reason = ?", "missing attendee").Count(ctx)
	if err != nil || count != 1 {
		t.Fatalf("attendance audit %d %v", count, err)
	}
	rows, err := s.att.ListByRaid(ctx, raidID)
	if err != nil || len(rows) != 3 {
		t.Fatalf("attendance rows: %+v %v", rows, err)
	}
}

func TestCorrectionTokenFailureRollsBackWholeImport(t *testing.T) {
	s, actor := importFixture(t)
	ctx := context.Background()
	raidID := seedImportedRaid(t, s, actor)
	data := sampleSheet()
	data.Hash = "rollback"
	data.Loot = []byte(stableHeader + stableDrop + "a\n" + "ICC25-01,09.02.2026,Newplayer,Weiheabzeichen des Eroberers,Main,52027,b\n")
	data.Character = append(data.Character, []byte("Newchar,Newplayer,Priester,Disziplin,,,\n")...)
	data.Raid = append(data.Raid, []byte("ICC25-01,09.02.2026,3,Newchar,Heiler,Priester,Disziplin,\n")...)
	s.fetcher = fakeFetcher{data: data}
	previewImport(t, s, "source", "sheet")
	if _, err := s.imports.db.ExecContext(ctx, "CREATE TRIGGER reject_import_token BEFORE INSERT ON token_transactions BEGIN SELECT RAISE(ABORT, 'token failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Merge(ctx, actor, "source", "sheet", "rollback", "missing rows"); err == nil {
		t.Fatal("expected token failure")
	}
	if _, err := s.players.FindByName(ctx, "Newplayer"); err == nil {
		t.Fatal("player survived rollback")
	}
	rows, err := s.att.ListByRaid(ctx, raidID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("attendance survived: %+v %v", rows, err)
	}
	awards, err := s.loot.ListByRaid(ctx, raidID)
	if err != nil || len(awards) != 1 {
		t.Fatalf("loot survived: %+v %v", awards, err)
	}
	count, err := s.imports.db.NewSelect().TableExpr("audit_log").Count(ctx)
	if err != nil || count != 0 {
		t.Fatalf("audit survived %d %v", count, err)
	}
	count, err = s.imports.db.NewSelect().TableExpr("enhance_point_transactions").Where("type = ?", "correction_no_mainspec_loot").Count(ctx)
	if err != nil || count != 0 {
		t.Fatalf("EP survived %d %v", count, err)
	}
	mappings, err := s.imports.mappings(ctx, "source", "sheet")
	if err != nil || len(mappings) != 1 {
		t.Fatalf("mappings survived: %+v %v", mappings, err)
	}
	history, err := s.List(ctx)
	if err != nil || len(history) != 1 {
		t.Fatalf("report survived: %+v %v", history, err)
	}
}
