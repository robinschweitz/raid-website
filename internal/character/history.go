package character

import (
	"time"

	"github.com/uptrace/bun"
)

type NameHistory struct {
	bun.BaseModel `bun:"table:character_name_history"`

	ID          int64     `bun:"id,pk,autoincrement" json:"id"`
	CharacterID int64     `bun:"character_id,notnull" json:"character_id"`
	OldName     string    `bun:"old_name,notnull" json:"old_name"`
	NewName     string    `bun:"new_name,notnull" json:"new_name"`
	ActorID     int64     `bun:"actor_id,notnull" json:"actor_id"`
	Reason      string    `bun:"reason,notnull" json:"reason"`
	CreatedAt   time.Time `bun:"created_at,notnull" json:"created_at"`
}
