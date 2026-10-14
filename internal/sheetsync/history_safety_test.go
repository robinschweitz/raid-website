package sheetsync

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/uptrace/bun"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/legacyimport"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidcompletion"
	"testing"
	"time"
)

func historyHash(d legacyimport.SheetData) legacyimport.SheetData {
	h := sha256.New()
	for _, v := range [][]byte{d.Raid, d.Loot, d.Character, d.Points, d.Setup} {
		h.Write(v)
		h.Write([]byte{0})
	}
	d.Hash = fmt.Sprintf("%x", h.Sum(nil))
	return d
}

func historySafetyFixture(t *testing.T) (*bun.DB, *legacyimport.Service, *validationFetcher, int64) {
	t.Helper()
	b, imp, f, a := validationDB(t)
	f.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,0,Nein\n")
	f.data = historyHash(f.data)
	cfg := &legacyimport.SetupConfig{Name: "Safety fixture", Leader: "Hero", FirstDate: "2026-10-19", StartTime: "19:00", Timezone: "Europe/Berlin", Weekdays: []int{1}, IntervalWeeks: 1}
	p, e := imp.DryRunWithSetup(context.Background(), Source, validationCopy, cfg)
	if e != nil || len(p.Errors) > 0 {
		t.Fatal(e, p.Errors)
	}
	m, e := imp.MergeWithSetup(context.Background(), a, Source, validationCopy, p.RawHash, "isolated fixture", cfg)
	if e != nil || len(m.Errors) > 0 {
		t.Fatal(e, m.Errors)
	}
	return b, imp, f, a
}
func TestSafetyHistoryOpeningMustNotDoubleArchiveReward(t *testing.T) {
	b, imp, f, a := historySafetyFixture(t)
	ctx := context.Background()
	rd, e := raid.NewRepo(b).FindByName(ctx, "Safety fixture")
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = raidcompletion.NewService(raidcompletion.NewRepo(b)).ArchiveRaid(ctx, rd.ID, a)
	if e != nil {
		t.Fatal(e)
	}
	before := historySnapshot(t, b)
	if before.Balances["Hero"] != 10 {
		t.Fatal("fixture reward not 10")
	}
	f.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,10,Nein\n")
	f.data = historyHash(f.data)
	s, e := New(b, imp, &validationWriter{}, validationCopy, a, "test", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.RunOnce(ctx)
	if e == nil {
		t.Fatal("changed opening balance must be rejected")
	}
	after := historySnapshot(t, b)
	if after.Balances["Hero"] != 10 {
		t.Fatalf("already awarded website EP counted again as opening balance: expected 10, got %d", after.Balances["Hero"])
	}
}
func TestSafetyHistorySetupConflictMustNotPartiallyImport(t *testing.T) {
	b, imp, f, a := historySafetyFixture(t)
	ctx := context.Background()
	before := historySnapshot(t, b)
	f.data.Character = append(f.data.Character, []byte("Newchar,Newplayer,Paladin,Vergeltung,\n")...)
	f.data.Setup = append(f.data.Setup, []byte("2,Unknown,Hero,Paladin,Vergeltung,Mdd\n")...)
	f.data = historyHash(f.data)
	s, e := New(b, imp, &validationWriter{}, validationCopy, a, "test", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.RunOnce(ctx)
	if e == nil {
		t.Fatal("expected setup conflict")
	}
	after := historySnapshot(t, b)
	if !historyUnchanged(before, after) {
		t.Fatal("failed sync has already committed player and character changes before setup conflict")
	}
}

func TestSafetyPreflightRollsBackAllWritesAndReportsLegacyBaseline(t *testing.T) {
	b, imp, f, a := historySafetyFixture(t)
	ctx := context.Background()
	s, err := New(b, imp, &validationWriter{}, validationCopy, a, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	before := historySnapshot(t, b)
	var receiptsBefore, receiptsAfter int
	if err = b.NewSelect().TableExpr("legacy_imports").ColumnExpr("COUNT(*)").Scan(ctx, &receiptsBefore); err != nil {
		t.Fatal(err)
	}
	report, err := s.Preflight(ctx)
	if err != nil || !report.Ready {
		t.Fatalf("safe preview: %+v %v", report, err)
	}
	if err = b.NewSelect().TableExpr("legacy_imports").ColumnExpr("COUNT(*)").Scan(ctx, &receiptsAfter); err != nil {
		t.Fatal(err)
	}
	if receiptsBefore != receiptsAfter || !historyUnchanged(before, historySnapshot(t, b)) {
		t.Fatal("preflight committed simulated import")
	}
	f.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,10,Nein\n")
	f.data = historyHash(f.data)
	report, err = s.Preflight(ctx)
	if err != nil || report.Ready || len(report.Blockers) == 0 {
		t.Fatal("unsafe preview not blocked", report, err)
	}
	historyExec(t, b, "DELETE FROM sheet_source_baselines")
	report, err = s.Preflight(ctx)
	if err != nil || report.Ready || len(report.Blockers) == 0 {
		t.Fatal("legacy baseline must block readiness", report, err)
	}
}

func TestSafetyRemovedSetupParticipantIsConflict(t *testing.T) {
	b, imp, f, a := validationDB(t)
	ctx := context.Background()
	f.data.Character = append(f.data.Character, []byte("Healer,Healer,Priester,Disziplin,\n")...)
	f.data = historyHash(f.data)
	cfg := &legacyimport.SetupConfig{Name: "Removal fixture", Leader: "Hero", FirstDate: "2026-10-19", StartTime: "19:00", Timezone: "Europe/Berlin", Weekdays: []int{1}, IntervalWeeks: 1}
	preview, err := imp.DryRunWithSetup(ctx, Source, validationCopy, cfg)
	if err != nil || len(preview.Errors) > 0 {
		t.Fatal(err, preview.Errors)
	}
	if _, err = imp.MergeWithSetup(ctx, a, Source, validationCopy, preview.RawHash, "fixture", cfg); err != nil {
		t.Fatal(err)
	}
	s, err := New(b, imp, &validationWriter{}, validationCopy, a, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	f.data.Setup = append(f.data.Setup, []byte("2,Healer,Healer,Priester,Disziplin,Heiler\n")...)
	f.data = historyHash(f.data)
	status, err := s.RunOnce(ctx)
	if err != nil || status.CurrentSetup.Added != 1 {
		t.Fatal("addition failed", status, err)
	}
	before := historySnapshot(t, b)
	f.data.Setup = []byte(",Charakter,Spieler,Klasse,Spec,Rolle\n1,Hero,Hero,Paladin,Vergeltung,Mdd\n")
	f.data = historyHash(f.data)
	if _, err = s.RunOnce(ctx); err == nil {
		t.Fatal("removal must be a conflict")
	}
	if !historyUnchanged(before, historySnapshot(t, b)) {
		t.Fatal("conflicting removal changed roster")
	}
	setup, err := imp.SyncCurrentSetup(ctx, a, Source, validationCopy, f.data.Hash)
	if err != nil || len(setup.Conflicts) == 0 || setup.Added != 0 {
		t.Fatal("direct setup removal not rejected", setup, err)
	}
}

func TestSafetyOldSetupDoesNotPopulateSuccessor(t *testing.T) {
	b, imp, _, a := historySafetyFixture(t)
	ctx := context.Background()
	rd, err := raid.NewRepo(b).FindByName(ctx, "Safety fixture")
	if err != nil {
		t.Fatal(err)
	}
	_, next, err := raidcompletion.NewService(raidcompletion.NewRepo(b)).ArchiveRaid(ctx, rd.ID, a)
	if err != nil || next == nil {
		t.Fatal(err)
	}
	before := historySnapshot(t, b)
	s, err := New(b, imp, &validationWriter{}, validationCopy, a, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.RunOnce(ctx)
	if err != nil || status.CurrentSetup.Added != 0 || len(status.CurrentSetup.Warnings) == 0 {
		t.Fatal(status, err)
	}
	if !historyUnchanged(before, historySnapshot(t, b)) {
		t.Fatal("stale setup changed successor or EP")
	}
}

func TestSafetyInactiveCharacterCannotBeDuplicated(t *testing.T) {
	b, imp, _, a := historySafetyFixture(t)
	ctx := context.Background()
	historyExec(t, b, "UPDATE characters SET active=0 WHERE name='Hero'")
	before := historySnapshot(t, b)
	s, err := New(b, imp, &validationWriter{}, validationCopy, a, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RunOnce(ctx); err == nil {
		t.Fatal("inactive source character must block automatic recreation")
	}
	if !historyUnchanged(before, historySnapshot(t, b)) {
		t.Fatal("inactive character was duplicated")
	}
}

type historyState struct {
	Hashes   map[string]string `json:"hashes"`
	Counts   map[string]int    `json:"counts"`
	Balances map[string]int    `json:"balances"`
}

func historySnapshot(t *testing.T, b *bun.DB) historyState {
	t.Helper()
	ctx := context.Background()
	s := historyState{Hashes: map[string]string{}, Counts: map[string]int{}, Balances: map[string]int{}}
	for _, table := range []string{"players", "characters", "raids", "raid_attendance", "loot_awards", "enhance_point_transactions", "raid_series", "audit_log", "item_locks", "legacy_raid_mappings", "legacy_loot_mappings", "legacy_setup_mappings"} {
		rows, e := b.QueryContext(ctx, "SELECT * FROM "+table+" ORDER BY 1,2")
		if e != nil {
			t.Fatal(e)
		}
		cols, e := rows.Columns()
		if e != nil {
			t.Fatal(e)
		}
		data := [][]any{}
		for rows.Next() {
			v := make([]any, len(cols))
			p := make([]any, len(cols))
			for i := range p {
				p[i] = &v[i]
			}
			if e = rows.Scan(p...); e != nil {
				t.Fatal(e)
			}
			data = append(data, v)
		}
		if e = rows.Err(); e != nil {
			t.Fatal(e)
		}
		rows.Close()
		raw, _ := json.Marshal(data)
		s.Hashes[table] = fmt.Sprintf("%x", sha256.Sum256(raw))
		s.Counts[table] = len(data)
	}
	players, e := player.NewRepo(b).List(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range players {
		bal, e := enhancepoints.NewRepo(b).Balance(ctx, p.ID)
		if e != nil {
			t.Fatal(e)
		}
		s.Balances[p.Name] = bal
	}
	return s
}
func historyUnchanged(a, b historyState) bool {
	for k, v := range a.Hashes {
		if b.Hashes[k] != v {
			return false
		}
	}
	return true
}
func historyExec(t *testing.T, b *bun.DB, q string, args ...any) {
	t.Helper()
	if _, e := b.ExecContext(context.Background(), q, args...); e != nil {
		t.Fatal(e)
	}
}
