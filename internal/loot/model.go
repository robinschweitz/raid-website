package loot

import (
	"time"

	"github.com/uptrace/bun"
)

type Award struct {
	bun.BaseModel `bun:"table:loot_awards"`

	ID                   int64     `bun:"id,pk,autoincrement"`
	RaidID               int64     `bun:"raid_id,notnull"`
	Boss                 string    `bun:"boss,notnull"`
	ItemID               string    `bun:"item_id,notnull"`
	ItemName             string    `bun:"item_name,notnull"`
	AwardedToPlayerID    int64     `bun:"awarded_to_player_id,notnull"`
	AwardedToCharacterID *int64    `bun:"awarded_to_character_id,nullzero"`
	AwardedByPlayerID    int64     `bun:"awarded_by_player_id,notnull"`
	Source               string    `bun:"source,notnull"`
	IsMainSpec           bool      `bun:"is_main_spec,notnull,default:false"`
	ItemLockID           *int64    `bun:"item_lock_id,nullzero"`
	Notes                string    `bun:"notes,notnull,default:''"`
	AwardedAt            time.Time `bun:"awarded_at,notnull"`
	CreatedAt            time.Time `bun:"created_at,notnull"`
	UpdatedAt            time.Time `bun:"updated_at,notnull"`
}
