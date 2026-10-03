package character

import "time"

type Character struct {
	ID            int64
	PlayerID      int64
	Name          string
	FormerNames   []string
	NameChangedAt *time.Time
	Class         string
	MainSpec      string
	SecondSpec    string
	IsMain        bool
	Active        bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
