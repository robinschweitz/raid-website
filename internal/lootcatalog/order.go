package lootcatalog

import (
	"context"
	"errors"
	"sort"
)

// ErrBossOrderInvalid rejects a boss order that is not exactly the bosses of
// one instance, each once.
var ErrBossOrderInvalid = errors.New("boss order must list every boss of the instance once")

// Boss order (ADR 088): an Admin sets the order of an instance's bosses for the
// boss pickers and the loot lists. It is guild configuration stored beside the
// bundled catalog, never part of it; bosses without a position follow the
// ordered ones in catalog order.

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
