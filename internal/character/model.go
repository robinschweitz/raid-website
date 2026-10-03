package character

import (
	"time"

	"github.com/uptrace/bun"
)

type Character struct {
	bun.BaseModel `bun:"table:characters"`

	ID            int64      `bun:"id,pk,autoincrement"`
	PlayerID      int64      `bun:"player_id,notnull"`
	Name          string     `bun:"name,notnull"`
	NameChangedAt *time.Time `bun:"name_changed_at,nullzero"`
	Class         string     `bun:"class,notnull"`
	MainSpec      string     `bun:"main_spec,notnull"`
	SecondSpec    string     `bun:"second_spec,notnull"`
	IsMain        bool       `bun:"is_main,notnull,default:false"`
	Active        bool       `bun:"active,notnull,default:true"`
	CreatedAt     time.Time  `bun:"created_at,notnull"`
	UpdatedAt     time.Time  `bun:"updated_at,notnull"`
}
