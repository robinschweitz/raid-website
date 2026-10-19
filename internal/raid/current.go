package raid

import (
	"time"

	"raid-website/internal/validation"
)

// Current picks the raid the "Aktueller Raid" link opens (ADR 088): the newest
// raid that has begun (started, or its scheduled start is not in the future),
// else the next raid to come. Cancelled raids and raids without any time are
// skipped. It returns nil when no raid qualifies.
func Current(raids []Raid, now time.Time) *Raid {
	var begun, next *Raid
	for i := range raids {
		rd := &raids[i]
		if rd.Status == validation.RaidStatusCancelled {
			continue
		}
		at := beginning(rd)
		if at == nil {
			continue
		}
		if !at.After(now) {
			if begun == nil || later(rd, begun) {
				begun = rd
			}
			continue
		}
		if next == nil || at.Before(*beginning(next)) || (at.Equal(*beginning(next)) && rd.ID < next.ID) {
			next = rd
		}
	}
	if begun != nil {
		return begun
	}
	return next
}

// beginning is when a raid began: its real start, else its scheduled start.
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
