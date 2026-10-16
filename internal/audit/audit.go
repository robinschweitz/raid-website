package audit

import (
	"time"

	"github.com/uptrace/bun"
)

type Log struct {
	bun.BaseModel `bun:"table:audit_log"`

	ID         int64     `bun:"id,pk,autoincrement" json:"id"`
	ActorID    *int64    `bun:"actor_id,nullzero" json:"actor_id"`
	EntityType string    `bun:"entity_type,notnull" json:"entity_type"`
	EntityID   string    `bun:"entity_id,notnull" json:"entity_id"`
	Action     string    `bun:"action,notnull" json:"action"`
	BeforeJSON string    `bun:"before_json,nullzero" json:"before_json"`
	AfterJSON  string    `bun:"after_json,nullzero" json:"after_json"`
	Reason     string    `bun:"reason,notnull,default:''" json:"reason"`
	CreatedAt  time.Time `bun:"created_at,notnull" json:"created_at"`
}

// Entry is the admin-visible form of an audit log entry. ActorName remains
// empty when the actor account was deleted after the action.
type Entry struct {
	ID         int64     `json:"id"`
	ActorID    *int64    `json:"actor_id"`
	ActorName  *string   `json:"actor_name"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Action     string    `json:"action"`
	BeforeJSON string    `json:"before_json"`
	AfterJSON  string    `json:"after_json"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}
