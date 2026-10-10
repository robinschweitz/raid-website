package character

import (
	"time"

	"github.com/uptrace/bun"
)

type Character struct {
	bun.BaseModel `bun:"table:characters"`

	ID                  int64      `bun:"id,pk,autoincrement" json:"id"`
	PlayerID            int64      `bun:"player_id,notnull" json:"player_id"`
	Name                string     `bun:"name,notnull" json:"name"`
	NameChangedAt       *time.Time `bun:"name_changed_at,nullzero" json:"name_changed_at"`
	Class               string     `bun:"class,notnull" json:"class"`
	MainSpec            string     `bun:"main_spec,notnull" json:"main_spec"`
	SecondSpec          string     `bun:"second_spec,notnull" json:"second_spec"`
	IsMain              bool       `bun:"is_main,notnull,default:false" json:"is_main"`
	Active              bool       `bun:"active,notnull,default:true" json:"active"`
	ExternalSource      string     `bun:"external_source,notnull" json:"external_source"`
	ExternalProfileID   string     `bun:"external_profile_id,notnull" json:"external_profile_id"`
	ExternalProfileURL  string     `bun:"external_profile_url,notnull" json:"external_profile_url"`
	ExternalProfileJSON string     `bun:"external_profile_json,notnull" json:"-"`
	CreatedAt           time.Time  `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt           time.Time  `bun:"updated_at,notnull" json:"updated_at"`
}
