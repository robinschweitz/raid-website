package poll

import (
	"time"

	"raid-website/internal/validation"
)

type Poll struct {
	ID          int64
	Title       string
	Description string
	Status      validation.PollStatus
	ClosesAt    *time.Time
	CreatedBy   int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Vote struct {
	ID       int64
	PollID   int64
	PlayerID int64
	Choice   string
	VotedAt  time.Time
}
