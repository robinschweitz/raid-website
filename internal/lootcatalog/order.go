package lootcatalog

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
)

// ErrBossOrderInvalid rejects a boss order that is not exactly the bosses of
// one instance, each once.
var ErrBossOrderInvalid = errors.New("boss order must list every boss of the instance once")

// Boss order (ADR 088): an Admin sets the order of an instance's bosses for the
// boss pickers and the loot lists. It is guild configuration stored beside the
// bundled catalog, never part of it; bosses without a position follow the
// ordered ones in catalog order.

type bossPosition struct {
	bun.BaseModel `bun:"table:loot_catalog_boss_order"`

	BossID            string    `bun:"boss_id,pk"`
	Position          int       `bun:"position,notnull"`
	UpdatedByPlayerID int64     `bun:"updated_by_player_id,notnull"`
	UpdatedAt         time.Time `bun:"updated_at,notnull"`
}

// bossPositions returns the stored position of every ordered boss.
func (r *Repo) bossPositions(ctx context.Context) (map[string]int, error) {
	if r.db == nil { // a catalog without a database (exporter, tests) has no order
		return map[string]int{}, nil
	}
	var rows []bossPosition
	if err := r.db.NewSelect().Model(&rows).Scan(ctx); err != nil {
		return nil, err
	}
	positions := make(map[string]int, len(rows))
	for _, row := range rows {
		positions[row.BossID] = row.Position
	}
	return positions, nil
}

// replaceBossOrder stores `order` for one instance's bosses and audits it.
func (r *Repo) replaceBossOrder(ctx context.Context, actorID int64, instanceID string, before, order []string) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if len(before) > 0 {
			if _, err := tx.NewDelete().Model((*bossPosition)(nil)).Where("boss_id IN (?)", bun.In(before)).Exec(ctx); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		rows := make([]bossPosition, len(order))
		for i, id := range order {
			rows[i] = bossPosition{BossID: id, Position: i, UpdatedByPlayerID: actorID, UpdatedAt: now}
		}
		if _, err := tx.NewInsert().Model(&rows).Exec(ctx); err != nil {
			return err
		}
		return audit.NewRepo(tx).Record(ctx, actorID, "boss_order", 0, "update",
			map[string]any{"instance_id": instanceID, "boss_ids": before},
			map[string]any{"instance_id": instanceID, "boss_ids": order}, "")
	})
}

// sortBosses orders bosses by their stored position; the rest keep their
// catalog order after them.
func sortBosses(bosses []Boss, positions map[string]int) {
	sort.SliceStable(bosses, func(i, j int) bool {
		pi, iOK := positions[bosses[i].ID]
		pj, jOK := positions[bosses[j].ID]
		if iOK != jOK {
			return iOK
		}
		return iOK && pi < pj
	})
}

// SetBossOrder stores the order of an instance's bosses. `bossIDs` must list
// every boss of the instance exactly once.
func (s *Service) SetBossOrder(ctx context.Context, actorID int64, instanceID string, bossIDs []string) error {
	bosses, err := s.Bosses(ctx, instanceID)
	if err != nil {
		return err
	}
	if instanceID == "" || len(bosses) == 0 || len(bossIDs) != len(bosses) {
		return ErrBossOrderInvalid
	}
	known := make(map[string]bool, len(bosses))
	before := make([]string, len(bosses))
	for i, boss := range bosses {
		known[boss.ID] = true
		before[i] = boss.ID
	}
	seen := make(map[string]bool, len(bossIDs))
	for _, id := range bossIDs {
		if !known[id] || seen[id] {
			return ErrBossOrderInvalid
		}
		seen[id] = true
	}
	return s.repo.replaceBossOrder(ctx, actorID, instanceID, before, bossIDs)
}

// BossRanks numbers every catalog boss in display order: the Admin-set
// positions first, then catalog order. Lists sort by it (ADR 088).
func (s *Service) BossRanks(ctx context.Context) (map[string]int, error) {
	bosses, err := s.Bosses(ctx, "")
	if err != nil {
		return nil, err
	}
	ranks := make(map[string]int, len(bosses))
	for i, boss := range bosses {
		ranks[boss.ID] = i
	}
	return ranks, nil
}
