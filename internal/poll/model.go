package poll

import (
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/validation"
)

type Poll struct {
	bun.BaseModel `bun:"table:polls"`

	ID            int64                 `bun:"id,pk,autoincrement" json:"id"`
	Title         string                `bun:"title,notnull" json:"title"`
	Description   string                `bun:"description,notnull,default:''" json:"description"`
	PollType      string                `bun:"poll_type,notnull" json:"poll_type"`
	AllowMultiple bool                  `bun:"allow_multiple,notnull" json:"allow_multiple"`
	Status        validation.PollStatus `bun:"status,notnull" json:"status"`
	ClosesAt      *time.Time            `bun:"closes_at,nullzero" json:"closes_at"`
	CreatedBy     int64                 `bun:"created_by,notnull" json:"created_by"`
	CreatedAt     time.Time             `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt     time.Time             `bun:"updated_at,notnull" json:"updated_at"`
	Options       []Option              `bun:"-" json:"options"`
}

type Option struct {
	bun.BaseModel `bun:"table:poll_options"`

	ID        int64  `bun:"id,pk,autoincrement" json:"id"`
	PollID    int64  `bun:"poll_id,notnull" json:"poll_id"`
	Label     string `bun:"label,notnull" json:"label"`
	SortOrder int    `bun:"sort_order,notnull" json:"sort_order"`
}

type Vote struct {
	bun.BaseModel `bun:"table:poll_votes"`

	ID       int64     `bun:"id,pk,autoincrement" json:"id"`
	PollID   int64     `bun:"poll_id,notnull" json:"poll_id"`
	PlayerID int64     `bun:"player_id,notnull" json:"player_id"`
	Choice   string    `bun:"choice,notnull" json:"choice"`
	VotedAt  time.Time `bun:"voted_at,notnull" json:"voted_at"`
	Choices  []string  `bun:"-" json:"choices"`
}

type VoteChoice struct {
	bun.BaseModel `bun:"table:poll_vote_choices"`

	ID        int64  `bun:"id,pk,autoincrement" json:"id"`
	VoteID    int64  `bun:"vote_id,notnull" json:"vote_id"`
	OptionID  int64  `bun:"option_id,notnull" json:"option_id"`
	Label     string `bun:"label,notnull" json:"label"`
	SortOrder int    `bun:"sort_order,notnull" json:"sort_order"`
}
