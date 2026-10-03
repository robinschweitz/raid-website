package character

import (
	"time"

	"github.com/uptrace/bun"
)

type NameHistory struct {
	bun.BaseModel `bun:"table:character_name_history"`

	ID          int64     `bun:"id,pk,autoincrement"`
	CharacterID int64     `bun:"character_id,notnull"`
	OldName     string    `bun:"old_name,notnull"`
	NewName     string    `bun:"new_name,notnull"`
	ActorID     int64     `bun:"actor_id,notnull"`
	Reason      string    `bun:"reason,notnull"`
	CreatedAt   time.Time `bun:"created_at,notnull"`
}
