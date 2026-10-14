package sheetsync

import (
	"context"
	"strings"
	"testing"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/legacyimport"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
)

// Existing deployments may have every ICC25 archive and the current roster
// but no imported recurring series at all. Sync must explain that missing
// prerequisite, then the existing explicit GUI setup import must bootstrap
// ICC25-36 without replaying historic loot or EP.
func TestHistoricalOnlySheetBootstrapThenSyncICC25Raid36(t *testing.T) {
	db, imports, source, actor := validationDB(t)
	ctx := context.Background()
	source.data.Raid = []byte("Raid ID,Timestamp,Position,Charakter,Rolle,Klasse,Spec\nICC25-35,12.10.2026,1,Hero,Mdd,Paladin,Vergeltung\n")
	source.data = historyHash(source.data)

	previous, err := imports.DryRun(ctx, Source, validationCopy)
	if err != nil || len(previous.Errors) != 0 {
		t.Fatalf("historical preview: %+v %v", previous, err)
	}
	if _, err := imports.Merge(ctx, actor, Source, validationCopy, previous.RawHash, "Archive"); err != nil {
		t.Fatal("historic import", err)
	}
	raids, err := raid.NewRepo(db).List(ctx, true)
	if err != nil || len(raids) != 1 {
		t.Fatal("historic raid not imported", raids, err)
	}
	series, err := raidseries.NewRepo(db).List(ctx)
	if err != nil || len(series) != 0 {
		t.Fatal("fixture must start without raid series", err)
	}

	sync, err := New(db, imports, &validationWriter{}, validationCopy, actor, "Archivabgleich", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := sync.Preflight(ctx)
	if err != nil || !preflight.Ready ||
		!strings.Contains(strings.Join(preflight.Setup.Warnings, " "), "raidsetup_initialization_required") {
		t.Fatalf("missing series must be explained in preflight: %+v %v", preflight, err)
	}
	status, err := sync.RunOnce(ctx)
	if err != nil || status.CurrentSetup == nil ||
		!strings.Contains(strings.Join(status.CurrentSetup.Warnings, " "), "raidsetup_initialization_required") {
		t.Fatalf("missing series must be explained in status: %+v %v", status, err)
	}
	before := archiveSnapshot(t, db)
	cfg := &legacyimport.SetupConfig{
		Name: "ICC25", Leader: "Hero", FirstDate: "2026-10-19",
		StartTime: "19:00", Timezone: "Europe/Berlin",
		Weekdays: []int{1}, IntervalWeeks: 1,
	}
	preview, err := imports.DryRunWithSetup(ctx, Source, validationCopy, cfg)
	if err != nil || len(preview.Errors) != 0 || preview.Setup == nil ||
		preview.Setup.ScheduledStart.IsZero() || preview.Setup.AlreadyImported {
		t.Fatalf("first setup preview: %+v %v", preview, err)
	}
	// The GUI preview must not have created the series or next raid.
	if !historyUnchanged(before, archiveSnapshot(t, db)) {
		t.Fatal("one-time setup preview mutated existing history")
	}
	if _, err := imports.MergeWithSetup(ctx, actor, Source, validationCopy, preview.RawHash, "ICC25 Serienplan", cfg); err != nil {
		t.Fatal("create future series and first raid", err)
	}
	next, err := raid.NewRepo(db).FindByName(ctx, "ICC25-36")
	if err != nil || next.ArchivedAt != nil || next.Status != "PLANNED" {
		t.Fatalf("next planned raid not created: %+v %v", next, err)
	}
	participants, err := attendance.NewRepo(db).ListByRaid(ctx, next.ID)
	if err != nil || len(participants) != 1 || participants[0].CharacterName != "Hero" {
		t.Fatal("current sheet roster was not imported", participants, err)
	}
	if _, err := sync.RunOnce(ctx); err != nil {
		t.Fatal("continuous sync must accept bootstrapped series", err)
	}
	got, err := raid.NewRepo(db).List(ctx, true)
	if err != nil || len(got) != 2 {
		t.Fatal("repeated sync must not create duplicate raids", got, err)
	}
}
