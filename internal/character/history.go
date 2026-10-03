package character

import "time"

type NameHistory struct {
	ID          int64
	CharacterID int64
	OldName     string
	NewName     string
	ActorID     int64
	Reason      string
	CreatedAt   time.Time
}
