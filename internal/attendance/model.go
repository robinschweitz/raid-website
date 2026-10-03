package attendance

import (
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/validation"
)

type Attendance struct {
	bun.BaseModel `bun:"table:raid_attendance"`

	ID           int64                     `bun:"id,pk,autoincrement" json:"id"`
	RaidID       int64                     `bun:"raid_id,notnull" json:"raid_id"`
	PlayerID     int64                     `bun:"player_id,notnull" json:"player_id"`
	CharacterID  int64                     `bun:"character_id,notnull" json:"character_id"`
	Role         validation.AttendanceRole `bun:"role,notnull" json:"role"`
	IsSubstitute bool                      `bun:"is_substitute,notnull,default:false" json:"is_substitute"`
	IsBench      bool                      `bun:"is_bench,notnull,default:false" json:"is_bench"`
	LateJoin     bool                      `bun:"late_join,notnull,default:false" json:"late_join"`
	EarlyLeave   bool                      `bun:"early_leave,notnull,default:false" json:"early_leave"`
	Notes        string                    `bun:"notes,notnull,default:''" json:"notes"`
	CreatedAt    time.Time                 `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt    time.Time                 `bun:"updated_at,notnull" json:"updated_at"`
}
