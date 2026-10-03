package loot

import (
	"time"

	"github.com/uptrace/bun"
)

type Award struct {
	bun.BaseModel `bun:"table:loot_awards"`

	ID                   int64     `bun:"id,pk,autoincrement" json:"id"`
	RaidID               int64     `bun:"raid_id,notnull" json:"raid_id"`
	Boss                 string    `bun:"boss,notnull" json:"boss"`
	ItemID               string    `bun:"item_id,notnull" json:"item_id"`
	ItemName             string    `bun:"item_name,notnull" json:"item_name"`
	AwardedToPlayerID    int64     `bun:"awarded_to_player_id,notnull" json:"awarded_to_player_id"`
	AwardedToCharacterID *int64    `bun:"awarded_to_character_id,nullzero" json:"awarded_to_character_id"`
	AwardedByPlayerID    int64     `bun:"awarded_by_player_id,notnull" json:"awarded_by_player_id"`
	Source               string    `bun:"source,notnull" json:"source"`
	IsMainSpec           bool      `bun:"is_main_spec,notnull,default:false" json:"is_main_spec"`
	ItemLockID           *int64    `bun:"item_lock_id,nullzero" json:"item_lock_id"`
	Notes                string    `bun:"notes,notnull,default:''" json:"notes"`
	AwardedAt            time.Time `bun:"awarded_at,notnull" json:"awarded_at"`
	CreatedAt            time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt            time.Time `bun:"updated_at,notnull" json:"updated_at"`
}
