package enhancepoints

import (
	"time"

	"github.com/uptrace/bun"
)

type Transaction struct {
	bun.BaseModel `bun:"table:enhance_point_transactions"`

	ID        int64     `bun:"id,pk,autoincrement"`
	PlayerID  int64     `bun:"player_id,notnull"`
	RaidID    *int64    `bun:"raid_id,nullzero"`
	Amount    int       `bun:"amount,notnull"`
	Type      string    `bun:"type,notnull"`
	Reason    string    `bun:"reason,notnull"`
	RefType   string    `bun:"ref_type,notnull,default:''"`
	RefID     string    `bun:"ref_id,notnull,default:''"`
	ActorID   int64     `bun:"actor_id,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
}
