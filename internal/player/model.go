package player

import (
	"time"

	"github.com/uptrace/bun"
)

type Player struct {
	bun.BaseModel `bun:"table:players"`

	ID          int64     `bun:"id,pk,autoincrement"`
	Name        string    `bun:"name,notnull"`
	GuildRankID *int64    `bun:"guild_rank_id,nullzero"`
	Active      bool      `bun:"active,notnull,default:true"`
	CreatedAt   time.Time `bun:"created_at,notnull"`
	UpdatedAt   time.Time `bun:"updated_at,notnull"`
}
