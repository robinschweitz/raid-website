// Package shadowmourne tracks who is doing the Shadowmourne questline, in which
// priority, and who does the steps that only one raid member can do per raid.
package shadowmourne

import "time"

// Stage is the questline step a character is currently on. The order of
// Stages is the questline order; staff may still set any stage.
type Stage string

const (
	StagePrequest Stage = "PREQUEST"
	StageSouls    Stage = "SOULS"
	StageUnholy   Stage = "UNHOLY"
	StageBlood    Stage = "BLOOD"
	StageFrost    Stage = "FROST"
	StageShards   Stage = "SHARDS"
	StageFinal    Stage = "FINAL"
)

var Stages = []Stage{StagePrequest, StageSouls, StageUnholy, StageBlood, StageFrost, StageShards, StageFinal}

// RaidTasks are the stages only one raid member can work on per raid.
var RaidTasks = []Stage{StageUnholy, StageBlood, StageFrost, StageShards}

// DefaultShardTarget is the Blizzlike number of Shadowfrost Shards. The target
// stays editable per entry (owner decision, ADR 086).
const DefaultShardTarget = 50

// EligibleClasses are the classes that can use Shadowmourne.
var EligibleClasses = []string{"Krieger", "Paladin", "Todesritter"}

func validStage(stage Stage) bool {
	for _, known := range Stages {
		if stage == known {
			return true
		}
	}
	return false
}

func validRaidTask(task Stage) bool {
	for _, known := range RaidTasks {
		if task == known {
			return true
		}
	}
	return false
}

func eligibleClass(class string) bool {
	for _, known := range EligibleClasses {
		if class == known {
			return true
		}
	}
	return false
}

type Status string

const (
	StatusActive    Status = "ACTIVE"
	StatusCompleted Status = "COMPLETED"
)

type Entry struct {
	ID              int64      `json:"id"`
	CharacterID     int64      `json:"character_id"`
	CharacterName   string     `json:"character_name"`
	CharacterClass  string     `json:"character_class"`
	PlayerID        int64      `json:"player_id"`
	PlayerName      string     `json:"player_name"`
	Priority        int        `json:"priority"`
	Status          Status     `json:"status"`
	Stage           Stage      `json:"stage"`
	ShardsCollected int        `json:"shards_collected"`
	ShardTarget     int        `json:"shard_target"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type CreateInput struct {
	CharacterID int64
	// Position is the 1-based place in the active list; 0 appends.
	Position        int
	Stage           Stage
	ShardsCollected int
	ShardTarget     int
}

type ProgressInput struct {
	Stage           Stage
	ShardsCollected int
	ShardTarget     int
}

// RaidTask says who does one single-person questline step in a raid.
// Assigned is the staff choice; Suggested is the first character on the list
// who is in the roster and on that stage.
type RaidTask struct {
	Task      Stage  `json:"task"`
	Assigned  *Entry `json:"assigned"`
	Suggested *Entry `json:"suggested"`
}

type RaidPlan struct {
	RaidID     int64      `json:"raid_id"`
	Editable   bool       `json:"editable"`
	Tasks      []RaidTask `json:"tasks"`
	Candidates []Entry    `json:"candidates"`
}
