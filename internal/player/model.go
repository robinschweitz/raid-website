package player

import (
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/validation"
)

type Player struct {
	bun.BaseModel `bun:"table:players"`

	ID           int64           `bun:"id,pk,autoincrement" json:"id"`
	Name         string          `bun:"name,notnull" json:"name"`
	GuildRankID  *int64          `bun:"guild_rank_id,nullzero" json:"guild_rank_id"`
	Username     *string         `bun:"username,nullzero" json:"username"`
	PasswordHash string          `bun:"password_hash,notnull,default:''" json:"-"`
	Role         validation.Role `bun:"role,notnull,default:'GUEST'" json:"role"`
	Active       bool            `bun:"active,notnull,default:true" json:"active"`
	CreatedAt    time.Time       `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt    time.Time       `bun:"updated_at,notnull" json:"updated_at"`
}
