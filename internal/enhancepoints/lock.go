package enhancepoints

import (
	"time"

	"raid-website/internal/validation"
)

type ItemLock struct {
	ID          int64
	PlayerID    int64
	RaidID      int64
	ItemID      string
	ItemName    string
	Boss        string
	Status      validation.LockStatus
	LockedAt    time.Time
	UsedAt      *time.Time
	ExpiredAt   *time.Time
	CancelledAt *time.Time
	ActorID     int64
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
