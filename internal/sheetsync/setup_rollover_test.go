package sheetsync

import (
	"context"
	"strings"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/raid"
)

// An existing successor receives the current live setup even if its roster
// happens to be identical to the previous archived raid. Historical edits
// and changes to already-matched website attendance still conflict.
func TestCurrentICC25SetupMovesToUpcomingRaidAfterSheetArchive(t *testing.T) {
	db, svc, source, actor, firstID := archiveFixture(t, 0, 10, "Second")
	ctx := context.Background()
	archivedSource := source.data
	source.data.Raid = []byte("Raid ID,Timestamp,Position,Charakter,Rolle,Klasse,Spec\n")
	source.data.Loot = []byte("Raid ID,Timestamp,Spieler,Loot,Priority,ItemId,Import ID\n")
	source.data.Points = []byte("Spieler,Punkte,Inaktiv\nHero,0,Nein\n")
	source.data = historyHash(source.data)
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal("initial setup baseline", err)
	}
	source.data = historyHash(archivedSource)
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal("archive adoption", err)
	}
	first, err := raid.NewRepo(db).Get(ctx, firstID)
	if err != nil || first.ArchivedAt == nil {
		t.Fatal("first raid not archived", err)
	}
	scheduled := first.ScheduledStart.AddDate(0, 0, 7)
	next := &raid.Raid{
		Name: "ICC25-02", RaidLeaderID: actor, SeriesID: first.SeriesID,
		ScheduledStart: &scheduled, Status: "PLANNED",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := raid.NewRepo(db).Create(ctx, next); err != nil {
		t.Fatal(err)
	}
	before := archiveSnapshot(t, db)
	preview, err := svc.Preflight(ctx)
	if err != nil || !preview.Ready || preview.Setup.Added != 1 ||
		preview.Setup.RaidID != next.ID || preview.Setup.RaidName != next.Name {
		t.Fatalf("current setup must fill next planned raid even unchanged: %+v %v", preview, err)
	}
	if !historyUnchanged(before, archiveSnapshot(t, db)) {
		t.Fatal("preview mutated history")
	}
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal("next raid setup", err)
	}
	rows, err := attendance.NewRepo(db).ListByRaid(ctx, next.ID)
	if err != nil || len(rows) != 1 || rows[0].Spec != "Vergeltung" {
		t.Fatal("current live roster not imported", rows, err)
	}
	original, err := attendance.NewRepo(db).ListByRaid(ctx, firstID)
	if err != nil || len(original) != 1 || original[0].Spec != "Vergeltung" {
		t.Fatal("old raid was changed", original, err)
	}
	stable := archiveSnapshot(t, db)
	if _, err := svc.RunOnce(ctx); err != nil || !historyUnchanged(stable, archiveSnapshot(t, db)) {
		t.Fatal("idempotent setup repeat", err)
	}
	// Editing an already synchronized participant's spec is a conflict.
	source.data.Setup = []byte(strings.Replace(string(source.data.Setup), "Vergeltung", "Schutz", 1))
	source.data = historyHash(source.data)
	conflict, err := svc.Preflight(ctx)
	if err != nil || conflict.Ready {
		t.Fatalf("existing website attendance must not be overwritten: %+v %v", conflict, err)
	}
}

func TestCurrentICC25SetupUsesLiveRosterWithoutOldBaseline(t *testing.T) {
	db, svc, _, actor, firstID := archiveFixture(t, 0, 10, "Second")
	ctx := context.Background()
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := raid.NewRepo(db).Get(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	scheduled := first.ScheduledStart.AddDate(0, 0, 7)
	next := &raid.Raid{
		Name: "ICC25-02", RaidLeaderID: actor, SeriesID: first.SeriesID,
		ScheduledStart: &scheduled, Status: "PLANNED",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := raid.NewRepo(db).Create(ctx, next); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.Preflight(ctx)
	if err != nil || !preview.Ready || preview.Setup.Added != 1 {
		t.Fatalf("current setup needs no previous archived snapshot: %+v %v", preview, err)
	}
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal("first successor setup", err)
	}
	rows, err := attendance.NewRepo(db).ListByRaid(ctx, next.ID)
	if err != nil || len(rows) != 1 || rows[0].Spec != "Vergeltung" {
		t.Fatal("live roster not imported after upgrade", rows, err)
	}
}
