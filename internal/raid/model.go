package raid

import (
	"time"

	"raid-website/internal/validation"
)

type Raid struct {
	ID             int64
	Name           string
	RaidLeaderID   int64
	Status         validation.RaidStatus
	ScheduledStart *time.Time
	ScheduledEnd   *time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
	CancelledAt    *time.Time
	ArchivedAt     *time.Time
	Notes          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
