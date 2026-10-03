package audit

import (
	"time"

	"github.com/uptrace/bun"
)

type Log struct {
	bun.BaseModel `bun:"table:audit_log"`

	ID         int64     `bun:"id,pk,autoincrement"`
	ActorID    *int64    `bun:"actor_id,nullzero"`
	EntityType string    `bun:"entity_type,notnull"`
	EntityID   string    `bun:"entity_id,notnull"`
	Action     string    `bun:"action,notnull"`
	BeforeJSON string    `bun:"before_json,nullzero"`
	AfterJSON  string    `bun:"after_json,nullzero"`
	Reason     string    `bun:"reason,notnull,default:''"`
	CreatedAt  time.Time `bun:"created_at,notnull"`
}
