package sheetsync

import (
	"context"
	"strings"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/raid"
)

// A changed live roster is allowed to follow the next active series occurrence.
// The unchanged former setup must never leak into an unproven successor.
func TestCurrentICC25SetupMovesToUpcomingRaidAfterSheetArchive(t *testing.T) {
	b, svc, source, actor, initialID := archiveFixture(t, 0, 10, "Second")
	ctx := context.Background()
	archivedSource := source.data

	// The original, unarchived setup was previously synchronized.
	source.data.Raid = []byte("Raid ID,Timestamp,Position,Charakter,Rolle,Klasse,Spec\n")
	source.data.Loot = []byte("Raid ID,Timestamp,Spieler,Loot,Priority,ItemId,Import ID\n")
	source.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,0,Nein\n")
	source.data = historyHash(source.data)
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal("initial setup baseline", err)
	}
	var baseline string
	if err := b.NewSelect().TableExpr("sheet_setup_baselines").Column("setup_csv").Where("raid_id=?", initialID).Scan(ctx, &baseline); err != nil || baseline == "" {
		t.Fatal("prior live setup not saved", err)
	}

	source.data = historyHash(archivedSource)
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal("archive adoption", err)
	}
	initial, err := raid.NewRepo(b).Get(ctx, initialID)
	if err != nil || initial.ArchivedAt == nil {
		t.Fatal("initial raid not archived", err)
	}
	nextStart := initial.ScheduledStart.AddDate(0, 0, 7)
	next := &raid.Raid{
		Name: "ICC25-02", RaidLeaderID: actor, SeriesID: initial.SeriesID,
		ScheduledStart: &nextStart, Status: "PLANNED",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := raid.NewRepo(b).Create(ctx, next); err != nil {
		t.Fatal(err)
	}

	// Before the Sheet roster changes, not even an empty successor is populated.
	preview, err := svc.Preflight(ctx)
	if err != nil || !preview.Ready || preview.Setup.Added != 0 ||
		!strings.Contains(strings.Join(preview.Setup.Warnings, " "), "identisch") {
		t.Fatalf("stale setup should be skipped: %+v %v", preview, err)
	}
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal("stale setup run", err)
	}
	rows, err := attendance.NewRepo(b).ListByRaid(ctx, next.ID)
	if err != nil || len(rows) != 0 {
		t.Fatal("stale setup copied into next raid", rows, err)
	}

	// A visibly changed first numbered block now belongs to the next unique active raid.
	source.data.Setup = []byte(strings.Replace(string(source.data.Setup), "Vergeltung", "Schutz", 1))
	source.data = historyHash(source.data)
	before := archiveSnapshot(t, b)
	preview, err = svc.Preflight(ctx)
	if err != nil || !preview.Ready || preview.Setup.Added != 1 ||
		preview.Setup.RaidID != next.ID || preview.Setup.RaidName != next.Name {
		t.Fatalf("next roster preview: %+v %v", preview, err)
	}
	if !historyUnchanged(before, archiveSnapshot(t, b)) {
		t.Fatal("preflight changed history")
	}
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal("next roster sync", err)
	}
	rows, err = attendance.NewRepo(b).ListByRaid(ctx, next.ID)
	if err != nil || len(rows) != 1 || rows[0].Spec != "Schutz" {
		t.Fatal("next roster not imported", rows, err)
	}
	oldRows, err := attendance.NewRepo(b).ListByRaid(ctx, initialID)
	if err != nil || len(oldRows) != 1 || oldRows[0].Spec != "Vergeltung" {
		t.Fatal("archived predecessor changed", oldRows, err)
	}
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal("retry should be idempotent", err)
	}
	rows, err = attendance.NewRepo(b).ListByRaid(ctx, next.ID)
	if err != nil || len(rows) != 1 {
		t.Fatal("repeat added duplicate participant", rows, err)
	}
}

func TestCurrentICC25SetupCanUseArchivedRosterWithoutPriorSnapshot(t *testing.T) {
	b, svc, source, actor, initialID := archiveFixture(t, 0, 10, "Second")
	ctx := context.Background()
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	initial, err := raid.NewRepo(b).Get(ctx, initialID)
	if err != nil {
		t.Fatal(err)
	}
	nextStart := initial.ScheduledStart.AddDate(0, 0, 7)
	next := &raid.Raid{
		Name: "ICC25-02", RaidLeaderID: actor, SeriesID: initial.SeriesID,
		ScheduledStart: &nextStart, Status: "PLANNED",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := raid.NewRepo(b).Create(ctx, next); err != nil {
		t.Fatal(err)
	}
	// Without a persisted setup baseline, the last archived raid itself is
	// sufficient to prove that an unchanged old roster must not be copied.
	report, err := svc.Preflight(ctx)
	if err != nil || !report.Ready || report.Setup.Added != 0 ||
		!strings.Contains(strings.Join(report.Setup.Warnings, " "), "identisch") {
		t.Fatalf("unchanged archived roster must be skipped: %+v %v", report, err)
	}
	source.data.Setup = []byte(strings.Replace(string(source.data.Setup), "Vergeltung", "Schutz", 1))
	source.data = historyHash(source.data)
	report, err = svc.Preflight(ctx)
	if err != nil || !report.Ready || report.Setup.Added != 1 ||
		!strings.Contains(strings.Join(report.Setup.Warnings, " "), "keine ältere Setup-Baseline") {
		t.Fatalf("changed roster should use archived predecessor as proof: %+v %v", report, err)
	}
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal("first successor sync after an upgrade", err)
	}
	rows, err := attendance.NewRepo(b).ListByRaid(ctx, next.ID)
	if err != nil || len(rows) != 1 || rows[0].Spec != "Schutz" {
		t.Fatal("next roster not imported after proven historical comparison", rows, err)
	}
}
