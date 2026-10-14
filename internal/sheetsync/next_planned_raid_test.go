package sheetsync

import (
	"context"
	"strings"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/legacyimport"
	"raid-website/internal/raid"
)

// ICC25-34 in the archival source gives the first imported setup raid the
// configured ICC25-35 identity. Adopting its Sheet archive must schedule
// ICC25-36 with the current live setup, not just archive ICC25-35.
func TestNextPlannedICC25Raid36FromLiveSetup(t *testing.T) {
	db, importer, fetcher, actor := validationDB(t)
	ctx := context.Background()
	fetcher.data.Raid = []byte("Raid ID,Timestamp,Position,Charakter,Rolle,Klasse,Spec\nICC25-34,07.10.2026,1,Hero,Mdd,Paladin,Vergeltung\n")
	fetcher.data = historyHash(fetcher.data)
	cfg := &legacyimport.SetupConfig{
		Name: "ICC25", Leader: "Hero", FirstDate: "2026-10-14",
		StartTime: "19:00", Timezone: "Europe/Berlin",
		Weekdays: []int{3}, IntervalWeeks: 1,
	}
	initial, err := importer.DryRunWithSetup(ctx, Source, validationCopy, cfg)
	if err != nil || len(initial.Errors) != 0 {
		t.Fatalf("initial setup preview: %v %+v", err, initial)
	}
	merged, err := importer.MergeWithSetup(ctx, actor, Source, validationCopy, initial.RawHash, "Initialserie", cfg)
	if err != nil || len(merged.Errors) != 0 {
		t.Fatalf("initial setup merge: %v %+v", err, merged)
	}
	first, err := raid.NewRepo(db).FindByName(ctx, "ICC25-35")
	if err != nil || first.SeriesID == nil || first.ArchivedAt != nil {
		t.Fatalf("initial planned ICC25-35 missing: %+v %v", first, err)
	}
	// The sheet has now finalized ICC25-35. Its EP total is authoritative.
	fetcher.data.Raid = append(fetcher.data.Raid, []byte("ICC25-35,14.10.2026,1,Hero,Mdd,Paladin,Vergeltung\n")...)
	fetcher.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,180,Nein\n")
	fetcher.data = historyHash(fetcher.data)
	svc, err := New(db, importer, &validationWriter{}, validationCopy, actor, "Sheet archive und nächster Raid", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	before := archiveSnapshot(t, db)
	preflight, err := svc.Preflight(ctx)
	if err != nil || !preflight.Ready || !preflight.Setup.CreatedRaid ||
		preflight.Setup.RaidName != "ICC25-36" || preflight.Setup.Added != 1 {
		t.Fatalf("new planned raid preview: %+v %v", preflight, err)
	}
	if !historyUnchanged(before, archiveSnapshot(t, db)) {
		t.Fatal("preview must not create ICC25-36")
	}
	if _, err = raid.NewRepo(db).FindByName(ctx, "ICC25-36"); err == nil {
		t.Fatal("preflight created a real raid")
	}
	result, err := svc.RunOnce(ctx)
	if err != nil || result.CurrentSetup == nil || !result.CurrentSetup.CreatedRaid ||
		result.CurrentSetup.RaidName != "ICC25-36" {
		t.Fatalf("planned raid sync: %+v %v", result, err)
	}
	old, err := raid.NewRepo(db).Get(ctx, first.ID)
	if err != nil || old.ArchivedAt == nil || old.Status != "COMPLETED" {
		t.Fatalf("sheet archive not adopted: %+v %v", old, err)
	}
	next, err := raid.NewRepo(db).FindByName(ctx, "ICC25-36")
	if err != nil || next.Status != "PLANNED" || next.ArchivedAt != nil ||
		next.PreviousRaidID == nil || *next.PreviousRaidID != first.ID ||
		next.SeriesID == nil || *next.SeriesID != *first.SeriesID ||
		next.ScheduledStart == nil || !next.ScheduledStart.After(time.Now()) {
		t.Fatalf("next scheduled raid invalid: %+v %v", next, err)
	}
	rows, err := attendance.NewRepo(db).ListByRaid(ctx, next.ID)
	if err != nil || len(rows) != 1 || rows[0].CharacterName != "Hero" {
		t.Fatalf("live setup roster not assigned to ICC25-36: %+v %v", rows, err)
	}
	stable := archiveSnapshot(t, db)
	if _, err := svc.RunOnce(ctx); err != nil || !historyUnchanged(stable, archiveSnapshot(t, db)) {
		t.Fatal("repeated sync created another planned raid or duplicate attendance", err)
	}
}

// A separate existing raid of the requested identity is not silently modified.
func TestICC25NextPlannedRaidNameConflictRollsBack(t *testing.T) {
	db, importer, fetcher, actor := validationDB(t)
	ctx := context.Background()
	fetcher.data.Raid = []byte("Raid ID,Timestamp,Position,Charakter,Rolle,Klasse,Spec\nICC25-34,07.10.2026,1,Hero,Mdd,Paladin,Vergeltung\n")
	fetcher.data = historyHash(fetcher.data)
	cfg := &legacyimport.SetupConfig{Name: "ICC25", Leader: "Hero", FirstDate: "2026-10-14", StartTime: "19:00", Timezone: "Europe/Berlin", Weekdays: []int{3}, IntervalWeeks: 1}
	p, err := importer.DryRunWithSetup(ctx, Source, validationCopy, cfg)
	if err != nil || len(p.Errors) != 0 {
		t.Fatal(err, p)
	}
	if _, err := importer.MergeWithSetup(ctx, actor, Source, validationCopy, p.RawHash, "initial", cfg); err != nil {
		t.Fatal(err)
	}
	fetcher.data.Raid = append(fetcher.data.Raid, []byte("ICC25-35,14.10.2026,1,Hero,Mdd,Paladin,Vergeltung\n")...)
	fetcher.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,180,Nein\n")
	fetcher.data = historyHash(fetcher.data)
	now := time.Now().UTC()
	if err := raid.NewRepo(db).Create(ctx, &raid.Raid{Name: "ICC25-36", RaidLeaderID: actor, Status: "PLANNED", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc, err := New(db, importer, &validationWriter{}, validationCopy, actor, "collision", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	before := archiveSnapshot(t, db)
	rep, err := svc.Preflight(ctx)
	if err != nil || rep.Ready || !strings.Contains(strings.Join(rep.Blockers, " "), "ICC25-36") {
		t.Fatalf("expected name collision warning: %+v %v", rep, err)
	}
	if _, err := svc.RunOnce(ctx); err == nil || !historyUnchanged(before, archiveSnapshot(t, db)) {
		t.Fatal("conflicting raid overwrote data or adopted partial archive", err)
	}
}
