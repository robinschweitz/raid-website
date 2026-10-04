package enhancepoints

import (
	"time"

	"github.com/uptrace/bun"
)

type Transaction struct {
	bun.BaseModel `bun:"table:enhance_point_transactions"`

	ID          int64     `bun:"id,pk,autoincrement" json:"id"`
	PlayerID    int64     `bun:"player_id,notnull" json:"player_id"`
	RaidID      *int64    `bun:"raid_id,nullzero" json:"raid_id"`
	LootAwardID *int64    `bun:"loot_award_id,nullzero" json:"loot_award_id"`
	Amount      int       `bun:"amount,notnull" json:"amount"`
	Type        string    `bun:"type,notnull" json:"type"`
	Reason      string    `bun:"reason,notnull" json:"reason"`
	RefType     string    `bun:"ref_type,notnull,default:''" json:"ref_type"`
	RefID       string    `bun:"ref_id,notnull,default:''" json:"ref_id"`
	ActorID     int64     `bun:"actor_id,notnull" json:"actor_id"`
	CreatedAt   time.Time `bun:"created_at,notnull" json:"created_at"`
}
