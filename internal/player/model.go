package player

import (
	"time"

	"github.com/uptrace/bun"
)

type Player struct {
	bun.BaseModel `bun:"table:players"`

	ID          int64     `bun:"id,pk,autoincrement" json:"id"`
	Name        string    `bun:"name,notnull" json:"name"`
	GuildRankID *int64    `bun:"guild_rank_id,nullzero" json:"guild_rank_id"`
	Active      bool      `bun:"active,notnull,default:true" json:"active"`
	CreatedAt   time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt   time.Time `bun:"updated_at,notnull" json:"updated_at"`
}
