package enhancepoints

import (
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/validation"
)

type ItemLock struct {
	bun.BaseModel `bun:"table:item_locks"`

	ID          int64                 `bun:"id,pk,autoincrement"`
	PlayerID    int64                 `bun:"player_id,notnull"`
	RaidID      int64                 `bun:"raid_id,notnull"`
	ItemID      string                `bun:"item_id,notnull,default:''"`
	ItemName    string                `bun:"item_name,notnull,default:''"`
	Boss        string                `bun:"boss,notnull,default:''"`
	Status      validation.LockStatus `bun:"status,notnull"`
	LockedAt    time.Time             `bun:"locked_at,notnull"`
	UsedAt      *time.Time            `bun:"used_at,nullzero"`
	ExpiredAt   *time.Time            `bun:"expired_at,nullzero"`
	CancelledAt *time.Time            `bun:"cancelled_at,nullzero"`
	ActorID     int64                 `bun:"actor_id,notnull"`
	Notes       string                `bun:"notes,notnull,default:''"`
	CreatedAt   time.Time             `bun:"created_at,notnull"`
	UpdatedAt   time.Time             `bun:"updated_at,notnull"`
}
