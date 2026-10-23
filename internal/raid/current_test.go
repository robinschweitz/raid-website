package raid

import (
	"testing"
	"time"

	"raid-website/internal/validation"
)

func at(t time.Time) *time.Time { return &t }

func TestCurrentPicksRunningRaid(t *testing.T) {
	now := time.Date(2026, 10, 19, 21, 0, 0, 0, time.UTC)
	raids := []Raid{
		{ID: 1, Status: validation.RaidStatusCompleted, ScheduledStart: at(now.AddDate(0, 0, -7))},
		{ID: 2, Status: validation.RaidStatusOpen, ScheduledStart: at(now.Add(-time.Hour))},
		{ID: 3, Status: validation.RaidStatusOpen, ScheduledStart: at(now.AddDate(0, 0, 7))},
		{ID: 4, Status: validation.RaidStatusCancelled, ScheduledStart: at(now.Add(-time.Minute))},
	}
	if got := Current(raids, now); got == nil || got.ID != 2 {
		t.Fatalf("want running raid 2, got %+v", got)
	}
}

func TestCurrentPicksNextAfterPreviousRaidCompleted(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	raids := []Raid{
		{ID: 1, Status: validation.RaidStatusCompleted, ScheduledStart: at(now.AddDate(0, 0, -1))},
		{ID: 2, Status: validation.RaidStatusOpen, ScheduledStart: at(now.AddDate(0, 0, 6))},
		{ID: 3, Status: validation.RaidStatusPlanned, ScheduledStart: at(now.AddDate(0, 0, 2))},
		{ID: 4, Status: validation.RaidStatusOpen, ScheduledStart: at(now.AddDate(0, 0, 2))},
	}
	if got := Current(raids, now); got == nil || got.ID != 3 {
		t.Fatalf("want earliest future raid 3 (lowest ID on tie), got %+v", got)
	}
}

func TestCurrentUsesRealStartAndSkipsArchived(t *testing.T) {
	now := time.Date(2026, 10, 19, 19, 0, 0, 0, time.UTC)
	raids := []Raid{
		{ID: 5, Status: validation.RaidStatusOpen, ScheduledStart: at(now.Add(time.Hour)), StartedAt: at(now.Add(-10 * time.Minute))},
		{ID: 6, Status: validation.RaidStatusPlanned, ScheduledStart: at(now.AddDate(0, 0, 7))},
		{ID: 7, Status: validation.RaidStatusOpen, ScheduledStart: at(now.Add(-time.Hour)), ArchivedAt: at(now)},
	}
	if got := Current(raids, now); got == nil || got.ID != 5 {
		t.Fatalf("want early-started raid 5, got %+v", got)
	}
	raids[0].Status = validation.RaidStatusCompleted
	if got := Current(raids, now); got == nil || got.ID != 6 {
		t.Fatalf("want upcoming raid 6 after completion, got %+v", got)
	}
}

func TestCurrentReturnsNilWithoutActiveOrUpcomingRaid(t *testing.T) {
	now := time.Date(2026, 10, 19, 19, 0, 0, 0, time.UTC)
	raids := []Raid{
		{ID: 1, Status: validation.RaidStatusCompleted, CompletedAt: at(now.Add(-time.Hour))},
		{ID: 2, Status: validation.RaidStatusCancelled, ScheduledStart: at(now.Add(time.Hour))},
		{ID: 3, Status: validation.RaidStatusOpen},
		{ID: 4, Status: validation.RaidStatusPlanned, ScheduledStart: at(now.Add(time.Hour)), ArchivedAt: at(now)},
	}
	if got := Current(raids, now); got != nil {
		t.Fatalf("want no current raid, got %+v", got)
	}
}
