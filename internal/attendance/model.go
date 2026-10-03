package attendance

import (
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/validation"
)

type Attendance struct {
	bun.BaseModel `bun:"table:raid_attendance"`

	ID           int64                     `bun:"id,pk,autoincrement"`
	RaidID       int64                     `bun:"raid_id,notnull"`
	PlayerID     int64                     `bun:"player_id,notnull"`
	CharacterID  int64                     `bun:"character_id,notnull"`
	Role         validation.AttendanceRole `bun:"role,notnull"`
	IsSubstitute bool                      `bun:"is_substitute,notnull,default:false"`
	IsBench      bool                      `bun:"is_bench,notnull,default:false"`
	LateJoin     bool                      `bun:"late_join,notnull,default:false"`
	EarlyLeave   bool                      `bun:"early_leave,notnull,default:false"`
	Notes        string                    `bun:"notes,notnull,default:''"`
	CreatedAt    time.Time                 `bun:"created_at,notnull"`
	UpdatedAt    time.Time                 `bun:"updated_at,notnull"`
}
