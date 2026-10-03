package roster

import "time"

type GuildRank struct {
	ID        int64     `bun:"id,pk,autoincrement" json:"id"`
	Name      string    `bun:"name,notnull" json:"name"`
	SortOrder int       `bun:"sort_order,notnull,default:0" json:"sort_order"`
	Active    bool      `bun:"active,notnull,default:true" json:"active"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull" json:"updated_at"`
}

type RaidGroup struct {
	ID        int64     `bun:"id,pk,autoincrement" json:"id"`
	Name      string    `bun:"name,notnull" json:"name"`
	Color     string    `bun:"color,notnull,default:''" json:"color"`
	SortOrder int       `bun:"sort_order,notnull,default:0" json:"sort_order"`
	Active    bool      `bun:"active,notnull,default:true" json:"active"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull" json:"updated_at"`
}
