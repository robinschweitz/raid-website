package raidcompletion

import (
	"context"
	"testing"
	"time"

	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

// ADR 078: archiving tops up the series' planning window and every raid it
// creates opens its signup right away.
func TestArchiveTopsUpPlanningWindowWithOpenSignups(t *testing.T) {
	ctx := context.Background()
	database := setupDB(t)
	actor := seedPlayer(t, ctx, player.NewRepo(database), "Actor")
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	series := &raidseries.Series{Name: "Mittwoch", RaidLeaderID: actor, Weekdays: []int{3}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin", PlanWeeksAhead: 3}
	if err := raidseries.NewService(raidseries.NewRepo(database)).Save(ctx, series); err != nil {
		t.Fatal(err)
	}
	stored, err := raidseries.NewRepo(database).Get(ctx, series.ID)
	if err != nil || stored.PlanWeeksAhead != 3 {
		t.Fatalf("plan weeks not stored: %+v %v", stored, err)
	}
	start := now.AddDate(0, 0, -1)
	current := &raid.Raid{Name: "Current", RaidLeaderID: actor, SeriesID: &series.ID, ScheduledStart: &start, Status: validation.RaidStatusOpen, CreatedAt: now, UpdatedAt: now}
	raids := raid.NewRepo(database)
	if err := raids.Create(ctx, current); err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewRepo(database))
	svc.now = func() time.Time { return now }
	_, next, err := svc.ArchiveRaid(ctx, current.ID, actor)
	if err != nil || next == nil {
		t.Fatalf("archive: %+v %v", next, err)
	}
	all, err := raids.ListBySeries(ctx, series.ID)
	if err != nil {
		t.Fatal(err)
	}
	future := 0
	for _, rd := range all {
		if rd.ID == current.ID {
			continue
		}
		future++
		if rd.Status != validation.RaidStatusPlanned || rd.ScheduledStart == nil || rd.ScheduledStart.After(now.AddDate(0, 0, 21)) {
			t.Fatalf("unexpected planned raid: %+v", rd)
		}
		var enabled bool
		if err := database.NewSelect().TableExpr("raid_signup_settings").Column("is_enabled").Where("raid_id = ?", rd.ID).Scan(ctx, &enabled); err != nil || !enabled {
			t.Fatalf("signups of raid %d not open: %v %v", rd.ID, enabled, err)
		}
	}
	if future != 3 {
		t.Fatalf("want three wednesdays planned, got %d: %+v", future, all)
	}
}
