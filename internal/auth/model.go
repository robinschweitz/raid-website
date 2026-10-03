package auth

import (
	"time"

	"github.com/uptrace/bun"
)

type Session struct {
	bun.BaseModel `bun:"table:sessions"`

	ID         int64     `bun:"id,pk,autoincrement" json:"id"`
	TokenHash  string    `bun:"token_hash,notnull,unique" json:"-"`
	PlayerID   int64     `bun:"player_id,notnull" json:"player_id"`
	CreatedAt  time.Time `bun:"created_at,notnull" json:"created_at"`
	ExpiresAt  time.Time `bun:"expires_at,notnull" json:"expires_at"`
	LastSeenAt time.Time `bun:"last_seen_at,notnull" json:"last_seen_at"`
}
