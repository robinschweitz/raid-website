package audit

import "time"

type Log struct {
	ID         int64
	ActorID    *int64
	EntityType string
	EntityID   string
	Action     string
	BeforeJSON string
	AfterJSON  string
	Reason     string
	CreatedAt  time.Time
}
