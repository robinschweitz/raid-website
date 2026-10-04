package raidcompletion

import (
	"context"
	"errors"
	"testing"
	"time"

	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

func TestConcurrentScheduleAndArchiveUseOneConsistentAnchor(t *testing.T) {
	ctx := context.Background()
	database := setupDB(t)
	database.SetMaxOpenConns(4)
	actor := seedPlayer(t, ctx, player.NewRepo(database), "Actor")
	raids := raid.NewRepo(database)
	archive := NewService(NewRepo(database))
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	archive.now = func() time.Time { return now }
	for i := 0; i < 20; i++ {
		oldSeries := &raidseries.Series{Name: "Old series", RaidLeaderID: actor, Weekdays: []int{1}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin"}
		newSeries := &raidseries.Series{Name: "New series", RaidLeaderID: actor, Weekdays: []int{3}, IntervalWeeks: 2, StartTime: "21:00", Timezone: "Europe/Berlin"}
		seriesService := raidseries.NewService(raidseries.NewRepo(database))
		if err := seriesService.Save(ctx, oldSeries); err != nil {
			t.Fatal(err)
		}
		if err := seriesService.Save(ctx, newSeries); err != nil {
			t.Fatal(err)
		}
		oldStart := time.Date(2026, 10, 5, 18, 0, 0, 0, time.UTC)
		newStart := time.Date(2026, 10, 19, 18, 0, 0, 0, time.UTC)
		rd := &raid.Raid{Name: "Current", RaidLeaderID: actor, SeriesID: &oldSeries.ID, ScheduledStart: &oldStart, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
		if err := raids.Create(ctx, rd); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		archived := make(chan error, 1)
		configured := make(chan error, 1)
		go func() { <-start; _, _, err := archive.ArchiveRaid(ctx, rd.ID, actor); archived <- err }()
		go func() {
			<-start
			_, err := raid.NewService(raids).ConfigureSchedule(ctx, rd.ID, actor, raid.ScheduleChanges{SeriesID: &newSeries.ID, ScheduledStart: &newStart})
			configured <- err
		}()
		close(start)
		if err := <-archived; err != nil {
			t.Fatal(err)
		}
		configuredError := <-configured
		if configuredError != nil && !errors.Is(configuredError, raid.ErrScheduleNotEditable) {
			t.Fatal(configuredError)
		}
		stored, err := raids.Get(ctx, rd.ID)
		if err != nil {
			t.Fatal(err)
		}
		expectedSeries := oldSeries
		expectedStart := oldStart
		if configuredError == nil {
			expectedSeries = newSeries
			expectedStart = newStart
		}
		if stored.Status != validation.RaidStatusCompleted || stored.ArchivedAt == nil || *stored.SeriesID != expectedSeries.ID || !stored.ScheduledStart.Equal(expectedStart) {
			t.Fatalf("schedule/archive race lost lifecycle or partially applied patch: %+v %v", stored, configuredError)
		}
		next, err := raids.Successor(ctx, rd.ID)
		if err != nil {
			t.Fatal(err)
		}
		expectedOccurrence, err := raidseries.NextOccurrence(expectedSeries, expectedStart, now)
		if err != nil {
			t.Fatal(err)
		}
		if next == nil || *next.SeriesID != expectedSeries.ID || next.Name != expectedSeries.Name || !next.ScheduledStart.Equal(expectedOccurrence) {
			t.Fatalf("inconsistent successor anchor: %+v", next)
		}
	}
}
