package loot

import "time"

type Award struct {
	ID                   int64
	RaidID               int64
	Boss                 string
	ItemID               string
	ItemName             string
	AwardedToPlayerID    int64
	AwardedToCharacterID *int64
	AwardedByPlayerID    int64
	Source               string
	IsMainSpec           bool
	ItemLockID           *int64
	Notes                string
	AwardedAt            time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
