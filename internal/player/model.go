package player

import "time"

type Player struct {
	ID          int64
	Name        string
	GuildRankID *int64
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
