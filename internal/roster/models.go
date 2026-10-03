package roster

import "time"

type GuildRank struct {
	ID        int64
	Name      string
	SortOrder int
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type RaidGroup struct {
	ID        int64
	Name      string
	Color     string
	SortOrder int
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
