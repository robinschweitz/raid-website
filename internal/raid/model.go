package raid

import (
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/validation"
)

type Raid struct {
	bun.BaseModel `bun:"table:raids"`

	ID             int64                 `bun:"id,pk,autoincrement"`
	Name           string                `bun:"name,notnull"`
	RaidLeaderID   int64                 `bun:"raid_leader_id,notnull"`
	Status         validation.RaidStatus `bun:"status,notnull"`
	ScheduledStart *time.Time            `bun:"scheduled_start,nullzero"`
	ScheduledEnd   *time.Time            `bun:"scheduled_end,nullzero"`
	StartedAt      *time.Time            `bun:"started_at,nullzero"`
	CompletedAt    *time.Time            `bun:"completed_at,nullzero"`
	CancelledAt    *time.Time            `bun:"cancelled_at,nullzero"`
	ArchivedAt     *time.Time            `bun:"archived_at,nullzero"`
	Notes          string                `bun:"notes,notnull,default:''"`
	CreatedAt      time.Time             `bun:"created_at,notnull"`
	UpdatedAt      time.Time             `bun:"updated_at,notnull"`
}
