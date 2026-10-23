package raid

import (
	"time"

	"raid-website/internal/validation"
)

// Current picks a running raid first, otherwise the next upcoming raid.
// Completed, cancelled and archived raids are never current (ADR 094).
func Current(raids []Raid, now time.Time) *Raid {
	var running, next *Raid
	for i := range raids {
		rd := &raids[i]
		if rd.ArchivedAt != nil || (rd.Status != validation.RaidStatusOpen && rd.Status != validation.RaidStatusPlanned) {
			continue
		}
		at := beginning(rd)
		if at == nil {
			continue
		}
		if !at.After(now) {
			if running == nil || later(rd, running) {
				running = rd
			}
			continue
		}
		if next == nil || at.Before(*beginning(next)) || (at.Equal(*beginning(next)) && rd.ID < next.ID) {
			next = rd
		}
	}
	if running != nil {
		return running
	}
	return next
}

// beginning prefers the actual start over the scheduled start.
func beginning(rd *Raid) *time.Time {
	if rd.StartedAt != nil {
		return rd.StartedAt
	}
	return rd.ScheduledStart
}

func later(a, b *Raid) bool {
	at, bt := beginning(a), beginning(b)
	if !at.Equal(*bt) {
		return at.After(*bt)
	}
	return a.ID > b.ID
}
