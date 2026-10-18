// Package shadowmourne manages the guild's serial Shadowmourne collection order.
package shadowmourne

import "time"

type Status string

const (
	StatusQueued     Status = "QUEUED"
	StatusCollecting Status = "COLLECTING"
	StatusCompleted  Status = "COMPLETED"
)

type Entry struct {
	ID              int64      `json:"id"`
	CharacterID     int64      `json:"character_id"`
	CharacterName   string     `json:"character_name"`
	PlayerID        int64      `json:"player_id"`
	PlayerName      string     `json:"player_name"`
	QueuePosition   int        `json:"queue_position"`
	Status          Status     `json:"status"`
	ShardsCollected int        `json:"shards_collected"`
	ShardTarget     int        `json:"shard_target"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type CreateInput struct {
	CharacterID     int64
	ShardsCollected int
	ShardTarget     int
}

type ProgressInput struct {
	ShardsCollected int
	ShardTarget     int
}

type Direction string

const (
	MoveUp   Direction = "UP"
	MoveDown Direction = "DOWN"
)
