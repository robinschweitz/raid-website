package enhancepoints

import (
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/validation"
)

type ItemLock struct {
	bun.BaseModel `bun:"table:item_locks"`

	ID          int64                 `bun:"id,pk,autoincrement" json:"id"`
	PlayerID    int64                 `bun:"player_id,notnull" json:"player_id"`
	RaidID      int64                 `bun:"raid_id,notnull" json:"raid_id"`
	ItemID      string                `bun:"item_id,notnull,default:''" json:"item_id"`
	ItemName    string                `bun:"item_name,notnull,default:''" json:"item_name"`
	Boss        string                `bun:"boss,notnull,default:''" json:"boss"`
	Status      validation.LockStatus `bun:"status,notnull" json:"status"`
	LockedAt    time.Time             `bun:"locked_at,notnull" json:"locked_at"`
	UsedAt      *time.Time            `bun:"used_at,nullzero" json:"used_at"`
	ExpiredAt   *time.Time            `bun:"expired_at,nullzero" json:"expired_at"`
	CancelledAt *time.Time            `bun:"cancelled_at,nullzero" json:"cancelled_at"`
	ActorID     int64                 `bun:"actor_id,notnull" json:"actor_id"`
	Notes       string                `bun:"notes,notnull,default:''" json:"notes"`
	CreatedAt   time.Time             `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt   time.Time             `bun:"updated_at,notnull" json:"updated_at"`
}
