package raid

import (
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/validation"
)

type Raid struct {
	bun.BaseModel `bun:"table:raids"`

	ID             int64                 `bun:"id,pk,autoincrement" json:"id"`
	SeriesID       *int64                `bun:"series_id" json:"series_id"`
	PreviousRaidID *int64                `bun:"previous_raid_id" json:"previous_raid_id"`
	Name           string                `bun:"name,notnull" json:"name"`
	RaidLeaderID   int64                 `bun:"raid_leader_id,notnull" json:"raid_leader_id"`
	Status         validation.RaidStatus `bun:"status,notnull" json:"status"`
	ScheduledStart *time.Time            `bun:"scheduled_start,nullzero" json:"scheduled_start"`
	ScheduledEnd   *time.Time            `bun:"scheduled_end,nullzero" json:"scheduled_end"`
	StartedAt      *time.Time            `bun:"started_at,nullzero" json:"started_at"`
	CompletedAt    *time.Time            `bun:"completed_at,nullzero" json:"completed_at"`
	CancelledAt    *time.Time            `bun:"cancelled_at,nullzero" json:"cancelled_at"`
	ArchivedAt     *time.Time            `bun:"archived_at,nullzero" json:"archived_at"`
	Notes          string                `bun:"notes,notnull,default:''" json:"notes"`
	CreatedAt      time.Time             `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt      time.Time             `bun:"updated_at,notnull" json:"updated_at"`
}
