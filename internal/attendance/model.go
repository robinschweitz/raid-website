package attendance

import (
	"time"

	"raid-website/internal/validation"
)

type Attendance struct {
	ID           int64
	RaidID       int64
	PlayerID     int64
	CharacterID  int64
	Role         validation.AttendanceRole
	IsSubstitute bool
	IsBench      bool
	LateJoin     bool
	EarlyLeave   bool
	Notes        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
