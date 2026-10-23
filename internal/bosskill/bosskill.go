// Package bosskill owns the killed bosses of a raid and the difficulty of each
// kill (FR-29). Writes go through raidcorrection, which applies the raid-state,
// reason and audit rules in one transaction (ARCHITECTURE "Boss kills").
package bosskill

import (
	"context"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

// Difficulty is the mode a boss was killed in (FR-29.6).
type Difficulty string

const (
	Normal   Difficulty = "NORMAL"
	Heroic   Difficulty = "HEROIC"
	Hardmode Difficulty = "HARDMODE"
)

// Valid reports whether the difficulty is one of the three known ones.
func (d Difficulty) Valid() bool { return d == Normal || d == Heroic || d == Hardmode }

var (
	ErrNotFound          = errors.New("boss kill not found")
	ErrExists            = errors.New("raid already has a kill of this boss")
	ErrInvalidBoss       = errors.New("boss does not belong to the raid's instance")
	ErrInvalidDifficulty = errors.New("catalog has no such difficulty for this boss and raid size")
	ErrHasLoot           = errors.New("raid still has loot of this boss")
)

// Kill is one killed boss of a raid. A removed kill keeps its row (FR-31.1).
type Kill struct {
	bun.BaseModel `bun:"table:raid_boss_kills"`

	ID                int64      `bun:"id,pk,autoincrement" json:"id"`
	RaidID            int64      `bun:"raid_id,notnull" json:"raid_id"`
	CatalogBossID     string     `bun:"catalog_boss_id,notnull" json:"catalog_boss_id"`
	Boss              string     `bun:"boss,notnull" json:"boss"`
	Difficulty        Difficulty `bun:"difficulty,notnull" json:"difficulty"`
	CreatedByPlayerID int64      `bun:"created_by_player_id,notnull" json:"created_by_player_id"`
	CreatedAt         time.Time  `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt         time.Time  `bun:"updated_at,notnull" json:"updated_at"`
	RemovedAt         *time.Time `bun:"removed_at,nullzero" json:"removed_at,omitempty"`
	RemovedByPlayerID *int64     `bun:"removed_by_player_id,nullzero" json:"removed_by_player_id,omitempty"`
	RemoveReason      string     `bun:"remove_reason,notnull" json:"remove_reason,omitempty"`
}

// Service reads kills; writes belong to raidcorrection.
type Service struct{ repo *Repo }

// NewService wires the service.
func NewService(repo *Repo) *Service { return &Service{repo: repo} }

// List returns the active kills of a raid.
func (s *Service) List(ctx context.Context, raidID int64) ([]Kill, error) {
	return s.repo.ListByRaid(ctx, raidID)
}
