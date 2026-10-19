package raid

import (
	"testing"
	"time"

	"raid-website/internal/validation"
)

func at(t time.Time) *time.Time { return &t }

func TestCurrentPicksTheNewestRaidThatHasBegun(t *testing.T) {
	now := time.Date(2026, 10, 19, 21, 0, 0, 0, time.UTC)
	raids := []Raid{
		{ID: 1, Status: validation.RaidStatusCompleted, ScheduledStart: at(now.AddDate(0, 0, -7))},
		{ID: 2, Status: validation.RaidStatusOpen, ScheduledStart: at(now.Add(-time.Hour))},
		{ID: 3, Status: validation.RaidStatusOpen, ScheduledStart: at(now.AddDate(0, 0, 7))},
		{ID: 4, Status: validation.RaidStatusCancelled, ScheduledStart: at(now.Add(-time.Minute))},
	}
	if got := Current(raids, now); got == nil || got.ID != 2 {
		t.Fatalf("want tonight's raid 2, got %+v", got)
	}
}

func TestCurrentKeepsTheLastRaidUntilTheNextBegins(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	raids := []Raid{
		{ID: 2, Status: validation.RaidStatusCompleted, ScheduledStart: at(now.AddDate(0, 0, -1))},
		{ID: 3, Status: validation.RaidStatusOpen, ScheduledStart: at(now.AddDate(0, 0, 6))},
	}
	if got := Current(raids, now); got == nil || got.ID != 2 {
		t.Fatalf("want yesterday's raid 2, got %+v", got)
	}
}

func TestCurrentUsesTheRealStartAndFallsBackToTheNextRaid(t *testing.T) {
	now := time.Date(2026, 10, 19, 19, 0, 0, 0, time.UTC)
	// Started early: counts although its scheduled start is still ahead.
	early := []Raid{
		{ID: 5, Status: validation.RaidStatusOpen, ScheduledStart: at(now.Add(time.Hour)), StartedAt: at(now.Add(-10 * time.Minute))},
		{ID: 6, Status: validation.RaidStatusPlanned, ScheduledStart: at(now.AddDate(0, 0, 7))},
	}
	if got := Current(early, now); got == nil || got.ID != 5 {
		t.Fatalf("want the started raid 5, got %+v", got)
	}
	// Nothing has begun yet: the next raid to come.
	ahead := []Raid{
		{ID: 8, Status: validation.RaidStatusPlanned, ScheduledStart: at(now.AddDate(0, 0, 14))},
		{ID: 7, Status: validation.RaidStatusOpen, ScheduledStart: at(now.AddDate(0, 0, 7))},
		{ID: 9, Status: validation.RaidStatusOpen},
	}
	if got := Current(ahead, now); got == nil || got.ID != 7 {
		t.Fatalf("want the next raid 7, got %+v", got)
	}
	if got := Current([]Raid{{ID: 9, Status: validation.RaidStatusOpen}}, now); got != nil {
		t.Fatalf("a raid without any time is not current, got %+v", got)
	}
	// Imported history has neither start: its completion counts.
	imported := []Raid{{ID: 10, Status: validation.RaidStatusCompleted, CompletedAt: at(now.AddDate(0, 0, -3))}, ahead[1]}
	if got := Current(imported, now); got == nil || got.ID != 10 {
		t.Fatalf("want the imported raid 10, got %+v", got)
	}
}
