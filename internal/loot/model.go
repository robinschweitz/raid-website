package loot

import (
	"time"

	"github.com/uptrace/bun"
)

type Award struct {
	bun.BaseModel `bun:"table:loot_awards"`

	ID                     int64     `bun:"id,pk,autoincrement" json:"id"`
	RaidID                 int64     `bun:"raid_id,notnull" json:"raid_id"`
	CatalogBossID          *string   `bun:"catalog_boss_id" json:"catalog_boss_id"`
	Boss                   string    `bun:"boss,notnull" json:"boss"`
	ItemID                 string    `bun:"item_id,notnull" json:"item_id"`
	ItemName               string    `bun:"item_name,notnull" json:"item_name"`
	AwardedToPlayerID      int64     `bun:"awarded_to_player_id,notnull" json:"awarded_to_player_id"`
	AwardedToCharacterID   *int64    `bun:"awarded_to_character_id,nullzero" json:"awarded_to_character_id"`
	AwardedToCharacterName string    `bun:"awarded_to_character_name,notnull,default:''" json:"awarded_to_character_name"`
	AwardedByPlayerID      int64     `bun:"awarded_by_player_id,notnull" json:"awarded_by_player_id"`
	Source                 string    `bun:"source,notnull" json:"source"`
	IsMainSpec             bool      `bun:"is_main_spec,notnull,default:false" json:"is_main_spec"`
	AwardMethod            Method    `bun:"award_method,notnull" json:"award_method"`
	EnhancedCost           int       `bun:"enhanced_cost,notnull" json:"enhanced_cost"`
	RequestID              *string   `bun:"request_id,nullzero" json:"request_id,omitempty"`
	RequestHash            string    `bun:"request_hash,notnull" json:"-"`
	ItemLockID             *int64    `bun:"item_lock_id,nullzero" json:"item_lock_id"`
	Notes                  string    `bun:"notes,notnull,default:''" json:"notes"`
	AwardedAt              time.Time `bun:"awarded_at,notnull" json:"awarded_at"`
	CreatedAt              time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt              time.Time `bun:"updated_at,notnull" json:"updated_at"`
}

// Method describes the recorded outside award decision.
type Method string

const (
	Main     Method = "MAIN"
	Second   Method = "SECOND"
	Enhanced Method = "ENHANCED"
	BOE      Method = "BOE"
)

// DeriveMainSpec validates the method and derives the persisted eligibility flag.
func (a *Award) DeriveMainSpec() error {
	switch a.AwardMethod {
	case Main, Enhanced:
		a.IsMainSpec = true
	case Second, BOE:
		a.IsMainSpec = false
	default:
		return ErrInvalidMethod
	}
	return nil
}
