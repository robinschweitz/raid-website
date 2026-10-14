package sheetsync

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"
	"raid-website/internal/attendance"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/legacyimport"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidcompletion"
)

func archiveFixture(t *testing.T, initial, target int, method string, missingInitialBalance ...bool) (*bun.DB, *Service, *validationFetcher, int64, int64) {
	t.Helper()
	ctx := context.Background()
	b, imp, f, actor := validationDB(t)
	f.data.Points = []byte(fmt.Sprintf("Spieler,Punkte,Inaktiv\nHero,%d,Nein\n", initial))
	if len(missingInitialBalance) > 0 && missingInitialBalance[0] {
		f.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,,Nein\n")
	}
	f.data = historyHash(f.data)
	cfg := &legacyimport.SetupConfig{Name: "ICC25", Leader: "Hero", FirstDate: "2026-10-14", StartTime: "19:00", Timezone: "Europe/Berlin", Weekdays: []int{3}, IntervalWeeks: 1}
	preview, err := imp.DryRunWithSetup(ctx, Source, validationCopy, cfg)
	if err != nil || len(preview.Errors) > 0 {
		t.Fatal(err, preview)
	}
	merged, err := imp.MergeWithSetup(ctx, actor, Source, validationCopy, preview.RawHash, "initial setup", cfg)
	if err != nil || len(merged.Errors) > 0 {
		t.Fatal(err, merged)
	}
	var raidID int64
	if err = b.NewSelect().TableExpr("legacy_setup_mappings").ColumnExpr("raid_id").Scan(ctx, &raidID); err != nil {
		t.Fatal(err)
	}
	rd, err := raid.NewRepo(b).Get(ctx, raidID)
	if err != nil {
		t.Fatal(err)
	}
	f.data.Raid = append(f.data.Raid, []byte(fmt.Sprintf("%s,14.10.2026,1,Hero,Mdd,Paladin,Vergeltung\n", rd.Name))...)
	if method != "" {
		f.data.Loot = append(f.data.Loot, []byte(fmt.Sprintf("%s,14.10.2026,Hero,Weiheabzeichen des Bezwingers,%s,52025,\n", rd.Name, method))...)
	}
	f.data.Points = []byte(fmt.Sprintf("Spieler,Punkte,Inaktiv\nHero,%d,Nein\n", target))
	f.data = historyHash(f.data)
	svc, err := New(b, imp, &validationWriter{}, validationCopy, actor, "Sheet-Archivübernahme", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return b, svc, f, actor, raidID
}

func TestSheetArchiveAdoptionAndRepeats(t *testing.T) {
	for _, test := range []struct {
		name                  string
		initial, target       int
		oldBaseline, original bool
	}{
		{"zero opening", 0, 10, false, false},
		{"existing balance", 170, 180, false, false},
		{"lower authoritative balance", 170, 120, false, false},
		{"target zero", 10, 0, false, false},
		{"old import without baselines", 0, 10, true, false},
		{"original provenance", 170, 180, false, true},
		{"old original without baselines", 0, 10, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, s, f, a, id := archiveFixture(t, test.initial, test.target, "Second")
			ctx := context.Background()
			if test.original {
				const original = "1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU"
				historyExec(t, b, "DROP TRIGGER legacy_setup_mappings_no_update")
				historyExec(t, b, "UPDATE legacy_setup_mappings SET sheet_id=?", original)
				historyExec(t, b, "UPDATE legacy_imports SET source_url=?", original)
				historyExec(t, b, "UPDATE sheet_source_baselines SET sheet_id=?", original)
				historyExec(t, b, "UPDATE sheet_point_baselines SET sheet_id=?", original)
			}
			if test.oldBaseline {
				historyExec(t, b, "DELETE FROM sheet_source_baselines")
				historyExec(t, b, "DELETE FROM sheet_point_baselines")
			}
			before := archiveSnapshot(t, b)
			preview, err := s.Preflight(ctx)
			if err != nil || !preview.Ready || len(preview.Import.ArchiveTransfers) != 1 || len(preview.Import.PointTransfers) != 1 {
				t.Fatal("preflight", preview, err)
			}
			if !historyUnchanged(before, archiveSnapshot(t, b)) {
				t.Fatal("preflight mutated domain/provenance")
			}
			result, err := s.RunOnce(ctx)
			if err != nil {
				t.Fatal(err, result)
			}
			after := archiveSnapshot(t, b)
			if after.Balances["Hero"] != test.target || after.Counts["loot_awards"] != 1 || after.Counts["raids"] != 1 {
				t.Fatal("wrong adopted state", after)
			}
			stored, err := raid.NewRepo(b).Get(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status != "COMPLETED" || stored.ArchivedAt == nil || stored.CompletedAt == nil {
				t.Fatal("not archived", stored)
			}
			for i := 0; i < 3; i++ {
				if _, err = s.RunOnce(ctx); err != nil {
					t.Fatal(err)
				}
				if !historyUnchanged(after, archiveSnapshot(t, b)) {
					t.Fatal("repeat altered adopted history")
				}
			}
			_, next, err := raidcompletion.NewService(raidcompletion.NewRepo(b)).ArchiveRaid(ctx, id, a)
			if err != nil || next != nil || !historyUnchanged(after, archiveSnapshot(t, b)) {
				t.Fatal("website rearchive repeated rewards/successor", err, next)
			}
			// A later legitimate website adjustment is retained with unchanged Sheet EP.
			hero, err := player.NewRepo(b).FindByName(ctx, "Hero")
			if err != nil {
				t.Fatal(err)
			}
			if err = enhancepoints.NewRepo(b).InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: hero.ID, Amount: 5, Type: "adjustment", ActorID: a, Reason: "later website edit", CreatedAt: time.Now().UTC()}); err != nil {
				t.Fatal(err)
			}
			if _, err = s.RunOnce(ctx); err != nil {
				t.Fatal(err)
			}
			if archiveSnapshot(t, b).Balances["Hero"] != test.target+5 {
				t.Fatal("unchanged source overwrote subsequent website EP")
			}
			// An edited no-ID historical row is never mistaken for a second award.
			f.data.Loot = []byte(strings.ReplaceAll(string(f.data.Loot), "52025", "52026"))
			f.data = historyHash(f.data)
			beforeConflict := archiveSnapshot(t, b)
			if _, err = s.RunOnce(ctx); err == nil {
				t.Fatal("edited no-ID history accepted")
			}
			if !historyUnchanged(beforeConflict, archiveSnapshot(t, b)) {
				t.Fatal("conflict partially changed history")
			}
		})
	}
}

func TestSheetArchiveConflictsRollbackEverything(t *testing.T) {
	for _, change := range []string{"website points", "website zero-sum history", "website points after intervening sync", "roster conflict", "unmapped website loot", "invalid timestamp", "missing participant points", "character edit", "source conflict rollback", "late setup validation", "demoted actor", "inactive actor", "cancelled raid", "inconsistent archived raid", "inconsistent timestamp"} {
		t.Run(change, func(t *testing.T) {
			b, s, f, a, id := archiveFixture(t, 0, 10, "Second")
			ctx := context.Background()
			hero, err := player.NewRepo(b).FindByName(ctx, "Hero")
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "website points", "website zero-sum history", "website points after intervening sync":
				amount := 5
				if change == "website zero-sum history" {
					amount = 0
				}
				if err = enhancepoints.NewRepo(b).InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: hero.ID, Amount: amount, Type: "adjustment", Reason: "website edit", ActorID: a, CreatedAt: time.Now().UTC()}); err != nil {
					t.Fatal(err)
				}
				if change == "website points after intervening sync" {
					pending := f.data
					var raw string
					if err = b.NewSelect().TableExpr("sheet_source_baselines").ColumnExpr("data_json").Scan(ctx, &raw); err != nil {
						t.Fatal(err)
					}
					if err = json.Unmarshal([]byte(raw), &f.data); err != nil {
						t.Fatal(err)
					}
					if _, err = s.RunOnce(ctx); err != nil {
						t.Fatal("unchanged source sync", err)
					}
					f.data = pending
				}
			case "demoted actor":
				historyExec(t, b, "UPDATE players SET role='MEMBER' WHERE id=?", a)
			case "inactive actor":
				historyExec(t, b, "UPDATE players SET active=0 WHERE id=?", a)
			case "cancelled raid":
				historyExec(t, b, "UPDATE raids SET status='CANCELLED' WHERE id=?", id)
			case "inconsistent archived raid":
				historyExec(t, b, "UPDATE raids SET archived_at=CURRENT_TIMESTAMP WHERE id=?", id)
			case "inconsistent timestamp":
				rd, err := raid.NewRepo(b).Get(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				f.data.Raid = append(f.data.Raid, []byte(fmt.Sprintf("%s,15.10.2026,2,Hero,Mdd,Paladin,Vergeltung\n", rd.Name))...)
			case "roster conflict":
				historyExec(t, b, "UPDATE raid_attendance SET spec='Schutz' WHERE raid_id=?", id)
			case "unmapped website loot":
				historyExec(t, b, "INSERT INTO loot_awards(raid_id,boss,item_id,item_name,awarded_to_player_id,awarded_by_player_id,source,award_method,awarded_at,created_at,updated_at) VALUES(?,?,'',?,?,?,'website','SECOND',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)", id, "test", "existing item", hero.ID, a)
			case "invalid timestamp":
				f.data.Raid = []byte(strings.ReplaceAll(string(f.data.Raid), "14.10.2026", "invalid"))
			case "missing participant points":
				f.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,,Nein\n")
			case "character edit":
				f.data.Character = []byte(strings.ReplaceAll(string(f.data.Character), "Vergeltung", "Schutz"))
			case "late setup validation":
				f.data.Setup = append(f.data.Setup, []byte("1,Hero,Hero,Paladin,Vergeltung,Mdd\n")...)
				f.data.Character = append(f.data.Character, []byte("Newchar,Newplayer,Paladin,Vergeltung,\n")...)
			case "source conflict rollback":
				f.data.Character = append(f.data.Character, []byte("Newchar,Newplayer,Paladin,Vergeltung,\n")...)
				f.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,,Nein\n")
			}
			f.data = historyHash(f.data)
			before := archiveSnapshot(t, b)
			report, err := s.Preflight(ctx)
			if err != nil || report.Ready {
				t.Fatal("conflict preflight", report, err)
			}
			if _, err = s.RunOnce(ctx); err == nil {
				t.Fatal("conflicting adoption accepted")
			}
			if !historyUnchanged(before, archiveSnapshot(t, b)) {
				t.Fatal("failed adoption committed domain data or provenance")
			}
		})
	}
}

func archiveSnapshot(t *testing.T, b *bun.DB) historyState {
	t.Helper()
	state := historySnapshot(t, b)
	for _, table := range []string{"sheet_provenance_scopes", "sheet_source_baselines", "sheet_point_baselines", "legacy_imports", "import_previews"} {
		rows, err := b.QueryContext(context.Background(), "SELECT * FROM "+table+" ORDER BY 1,2")
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		data := [][]any{}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err = rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			data = append(data, values)
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err = rows.Close(); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		state.Hashes[table] = fmt.Sprintf("%x", sha256.Sum256(raw))
		state.Counts[table] = len(data)
	}
	return state
}

func TestSheetArchiveReconcilesLaterRaidsInSameSeries(t *testing.T) {
	b, s, f, a, id := archiveFixture(t, 0, 10, "Second")
	ctx := context.Background()
	if _, err := s.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := raid.NewRepo(b).Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	scheduled := first.ScheduledStart.AddDate(0, 0, 7)
	second := &raid.Raid{Name: "ICC25-02", SeriesID: first.SeriesID, RaidLeaderID: a, Status: "OPEN", ScheduledStart: &scheduled, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err = raid.NewRepo(b).Create(ctx, second); err != nil {
		t.Fatal(err)
	}
	roster, err := attendance.NewRepo(b).ListByRaid(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	row := roster[0]
	row.ID = 0
	row.RaidID = second.ID
	if err = attendance.NewRepo(b).Create(ctx, &row); err != nil {
		t.Fatal(err)
	}
	f.data.Raid = append(f.data.Raid, []byte("ICC25-02,21.10.2026,1,Hero,Mdd,Paladin,Vergeltung\n")...)
	f.data.Loot = append(f.data.Loot, []byte("ICC25-02,21.10.2026,Hero,Weiheabzeichen des Bezwingers,Second,52025,\n")...)
	f.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,20,Nein\n")
	f.data = historyHash(f.data)
	if _, err = s.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	after := archiveSnapshot(t, b)
	if after.Balances["Hero"] != 20 || after.Counts["raids"] != 2 || after.Counts["loot_awards"] != 2 {
		t.Fatal("later archive", after)
	}
	stored, err := raid.NewRepo(b).Get(ctx, second.ID)
	if err != nil || stored.ArchivedAt == nil {
		t.Fatal("later raid stayed active", stored, err)
	}
	for i := 0; i < 3; i++ {
		if _, err = s.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
		if !historyUnchanged(after, archiveSnapshot(t, b)) {
			t.Fatal("later archive repeat changed history")
		}
	}
}

type archiveFailingWriter struct {
	validationWriter
	fail bool
}

func (w *archiveFailingWriter) Publish(ctx context.Context, id string, tabs map[string][][]string, prev map[string]string) (map[string]string, error) {
	if w.fail {
		return nil, fmt.Errorf("simulated export failure")
	}
	return w.validationWriter.Publish(ctx, id, tabs, prev)
}
func TestSheetArchiveExportFailureRetryDoesNotReplayEP(t *testing.T) {
	b, s, _, _, _ := archiveFixture(t, 0, 10, "Second")
	ctx := context.Background()
	writer := &archiveFailingWriter{fail: true}
	s.writer = writer
	if _, err := s.RunOnce(ctx); err == nil {
		t.Fatal("expected export failure")
	}
	after := archiveSnapshot(t, b)
	if after.Balances["Hero"] != 10 || after.Counts["loot_awards"] != 1 {
		t.Fatal("domain archive not committed before export", after)
	}
	writer.fail = false
	if _, err := s.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if !historyUnchanged(after, archiveSnapshot(t, b)) {
		t.Fatal("export retry repeated archive or EP")
	}
}

func TestSheetArchiveMissingBalanceWithMainLootPreservesWebsiteEP(t *testing.T) {
	b, s, f, a, _ := archiveFixture(t, 0, 0, "Second", true)
	ctx := context.Background()
	hero, err := player.NewRepo(b).FindByName(ctx, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err = enhancepoints.NewRepo(b).InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: hero.ID, Amount: 170, Type: "adjustment", ActorID: a, Reason: "existing website balance, source balance absent", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	f.data.Loot = []byte(strings.ReplaceAll(string(f.data.Loot), "Weiheabzeichen des Bezwingers,Second,52025", "Armschienen aus Eisbärenklauen,Main,50659"))
	f.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,,Nein\n")
	f.data = historyHash(f.data)
	if _, err := s.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	state := archiveSnapshot(t, b)
	if state.Balances["Hero"] != 170 {
		t.Fatal("absent balance was invented/reset", state)
	}
}

func TestSheetArchiveDoesNotAcceptUnrelatedSourcePointEdits(t *testing.T) {
	b, s, f, a, _ := archiveFixture(t, 0, 10, "Second")
	ctx := context.Background()
	pending := f.data
	var raw string
	if err := b.NewSelect().TableExpr("sheet_source_baselines").ColumnExpr("data_json").Scan(ctx, &raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(raw), &f.data); err != nil {
		t.Fatal(err)
	}
	f.data.Points = append(f.data.Points, []byte("Other,30,Nein\n")...)
	f.data = historyHash(f.data)
	preview, err := s.importer.DryRun(ctx, Source, validationCopy)
	if err != nil || len(preview.Errors) > 0 {
		t.Fatal(err, preview)
	}
	merged, err := s.importer.Merge(ctx, a, Source, validationCopy, preview.RawHash, "second existing player")
	if err != nil || len(merged.Errors) > 0 {
		t.Fatal(err, merged)
	}
	f.data = pending
	f.data.Points = append(f.data.Points, []byte("Other,99,Nein\n")...)
	f.data = historyHash(f.data)
	before := archiveSnapshot(t, b)
	if _, err = s.RunOnce(ctx); err == nil {
		t.Fatal("unrelated EP edit accepted")
	}
	if !historyUnchanged(before, archiveSnapshot(t, b)) {
		t.Fatal("unrelated point conflict partially changed archive")
	}
}

func TestSheetArchiveExpiresLocksWithoutAdditionalEP(t *testing.T) {
	b, s, _, a, id := archiveFixture(t, 170, 180, "Second")
	ctx := context.Background()
	hero, err := player.NewRepo(b).FindByName(ctx, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	lock := &enhancepoints.ItemLock{PlayerID: hero.ID, RaidID: id, ItemID: "50709", ItemName: "Bryntroll, der Knochenrächer", Status: "ACTIVE", LockedAt: time.Now().UTC(), ActorID: a, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err = enhancepoints.NewRepo(b).CreateLock(ctx, lock); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	stored, err := enhancepoints.NewRepo(b).GetLock(ctx, lock.ID)
	if err != nil || stored.Status != "EXPIRED" || stored.ExpiredAt == nil {
		t.Fatal("lock not expired", stored, err)
	}
	if archiveSnapshot(t, b).Balances["Hero"] != 180 {
		t.Fatal("lock changed EP")
	}
}

func TestSheetArchiveAlreadyCompletedOnWebsiteIsNotRewardedAgain(t *testing.T) {
	b, s, _, a, id := archiveFixture(t, 0, 10, "Second")
	ctx := context.Background()
	finalizer := raidcompletion.NewService(raidcompletion.NewRepo(b))
	_, next, err := finalizer.ArchiveRaid(ctx, id, a)
	if err != nil || next == nil {
		t.Fatal("website fixture completion", err)
	}
	if archiveSnapshot(t, b).Balances["Hero"] != 10 {
		t.Fatal("website fixture reward")
	}
	if _, err = s.RunOnce(ctx); err != nil {
		t.Fatal("adoption of already completed website raid", err)
	}
	after := archiveSnapshot(t, b)
	if after.Balances["Hero"] != 10 || after.Counts["raids"] != 2 || after.Counts["loot_awards"] != 1 {
		t.Fatal("website completion replayed", after)
	}
	_, again, err := finalizer.ArchiveRaid(ctx, id, a)
	if err != nil || again == nil || again.ID != next.ID || !historyUnchanged(after, archiveSnapshot(t, b)) {
		t.Fatal("existing successor changed", err)
	}
}

func TestSheetArchiveDetectsWebsiteEditOfMappedLoot(t *testing.T) {
	b, s, f, a, id := archiveFixture(t, 0, 10, "Second")
	ctx := context.Background()
	rd, err := raid.NewRepo(b).Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	historyExec(t, b, "INSERT INTO legacy_raid_mappings(source,sheet_id,raid_key,raid_id) VALUES(?,?,?,?)", Source, validationCopy, rd.Name, id)
	f.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,0,Nein\n")
	f.data.Loot = []byte(strings.ReplaceAll(string(f.data.Loot), "52025,\n", "52025,mapped-before-archive\n"))
	f.data = historyHash(f.data)
	preview, err := s.importer.DryRun(ctx, Source, validationCopy)
	if err != nil || len(preview.Errors) > 0 {
		t.Fatal(err, preview)
	}
	merged, err := s.importer.Merge(ctx, a, Source, validationCopy, preview.RawHash, "legacy partial loot import into active mapped raid")
	if err != nil || len(merged.Errors) > 0 {
		t.Fatal(err, merged)
	}
	historyExec(t, b, "UPDATE loot_awards SET item_name='Website correction' WHERE raid_id=?", id)
	f.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,10,Nein\n")
	f.data = historyHash(f.data)
	before := archiveSnapshot(t, b)
	if _, err = s.RunOnce(ctx); err == nil || !strings.Contains(err.Error(), "Website-Loot wurde") {
		t.Fatal("website loot correction not detected", err)
	}
	if !historyUnchanged(before, archiveSnapshot(t, b)) {
		t.Fatal("loot conflict partially archived raid")
	}
}
