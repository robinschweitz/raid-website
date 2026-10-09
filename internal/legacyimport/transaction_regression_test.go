package legacyimport

import (
	"context"
	"errors"
	"testing"
	"time"

	"raid-website/internal/player"
	"raid-website/internal/raidseries"
)

func TestImportSeriesUsesTransactionAndRollsBack(t *testing.T) {
	_, svc, players, _, _, _ := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	actor := &player.Player{Name: "Importer", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := players.Create(ctx, actor); err != nil {
		t.Fatal(err)
	}
	abort := errors.New("abort import")
	err := svc.transaction(ctx, true, func(bound *Service) error {
		series := &raidseries.Series{Name: "Test series", RaidLeaderID: actor.ID,
			Weekdays: []int{1}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin",
			CreatedAt: now, UpdatedAt: now}
		if err := bound.series.Save(ctx, series); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("expected rollback sentinel, got %v", err)
	}
	rows, err := svc.series.List(ctx)
	if err != nil || len(rows) != 0 {
		t.Fatalf("series escaped rollback: %v, %v", rows, err)
	}
}

func TestChangedSnapshotDoesNotBootstrapAnotherSeries(t *testing.T) {
	_, svc, players, _, _, raids := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Now().UTC()
	actor := &player.Player{Name: "Importer", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := players.Create(ctx, actor); err != nil {
		t.Fatal(err)
	}
	for _, hash := range []string{"initial", "reordered"} {
		data := sampleSheet()
		data.Hash = hash
		data.Setup = setupCSV()
		svc.fetcher = fakeFetcher{data: data}
		preview, err := svc.DryRunWithSetup(ctx, "test", "sheet", setupConfig())
		if err != nil || len(preview.Errors) != 0 {
			t.Fatalf("preview: %+v, %v", preview, err)
		}
		report, err := svc.MergeWithSetup(ctx, actor.ID, "test", "sheet", preview.RawHash, "", setupConfig())
		if err != nil || len(report.Errors) != 0 {
			t.Fatalf("merge: %+v, %v", report, err)
		}
		if hash == "reordered" && report.Created.Raids != 0 {
			t.Fatalf("reimport created raids: %+v", report)
		}
	}
	series, err := svc.series.List(ctx)
	if err != nil || len(series) != 1 {
		t.Fatalf("series: %+v, %v", series, err)
	}
	rows, err := raids.List(ctx, true)
	if err != nil || len(rows) != 2 {
		t.Fatalf("raids: %+v, %v", rows, err)
	}
}
