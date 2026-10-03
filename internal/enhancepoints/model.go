package enhancepoints

import "time"

type Transaction struct {
	ID        int64
	PlayerID  int64
	RaidID    *int64
	Amount    int
	Type      string
	Reason    string
	RefType   string
	RefID     string
	ActorID   int64
	CreatedAt time.Time
}
