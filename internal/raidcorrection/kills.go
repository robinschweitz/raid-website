package raidcorrection

import (
	"context"
	"errors"
	"strings"
	"time"

	"raid-website/internal/bosskill"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
)

// RecordKill stores a killed boss of a raid with one catalog difficulty
// (FR-29.5, FR-29.6). The raid follows the correction rules of FR-13.11 and
// FR-26.5; the kill and its audit entry share one transaction.
func (s *Service) RecordKill(ctx context.Context, raidID, actor int64, catalogBossID string, difficulty bosskill.Difficulty, reason string) (*bosskill.Kill, error) {
	kill := &bosskill.Kill{}
	err := s.repo.InTransaction(ctx, func(r *Repo) error {
		if _, err := s.check(ctx, r, raidID, actor, reason); err != nil {
			return err
		}
		boss, err := s.killDifficulty(ctx, r, raidID, strings.TrimSpace(catalogBossID), difficulty)
		if err != nil {
			return err
		}
		exists, err := r.kills.ExistsActive(ctx, raidID, boss.ID)
		if err != nil {
			return err
		}
		if exists {
			return bosskill.ErrExists
		}
		now := time.Now().UTC()
		*kill = bosskill.Kill{RaidID: raidID, CatalogBossID: boss.ID, Boss: boss.Name, Difficulty: difficulty, CreatedByPlayerID: actor, CreatedAt: now, UpdatedAt: now}
		if err := r.kills.Create(ctx, kill); err != nil {
			return err
		}
		return r.audit.Record(ctx, actor, "raid_boss_kill", kill.ID, "create", nil, kill, reason)
	})
	return kill, err
}

// ChangeKillDifficulty corrects the difficulty of a kill (FR-29.14). It is
// rejected while the raid has unwithdrawn loot of that boss, because that loot
// was recorded under the old difficulty.
func (s *Service) ChangeKillDifficulty(ctx context.Context, raidID, killID, actor int64, difficulty bosskill.Difficulty, reason string) (*bosskill.Kill, error) {
	kill := &bosskill.Kill{}
	err := s.repo.InTransaction(ctx, func(r *Repo) error {
		old, err := s.activeKill(ctx, r, raidID, killID)
		if err != nil {
			return err
		}
		if _, err := s.check(ctx, r, raidID, actor, reason); err != nil {
			return err
		}
		if _, err := s.killDifficulty(ctx, r, raidID, old.CatalogBossID, difficulty); err != nil {
			return err
		}
		*kill = *old
		if old.Difficulty == difficulty {
			return nil
		}
		if err := s.killWithoutLoot(ctx, r, old); err != nil {
			return err
		}
		kill.Difficulty = difficulty
		kill.UpdatedAt = time.Now().UTC()
		if err := r.kills.Update(ctx, kill); err != nil {
			return err
		}
		return r.audit.Record(ctx, actor, "raid_boss_kill", kill.ID, "update", old, kill, reason)
	})
	return kill, err
}

// RemoveKill archives a wrongly recorded kill (FR-29.5, FR-29.9). The row stays
// as history; the boss can be recorded again afterwards.
func (s *Service) RemoveKill(ctx context.Context, raidID, killID, actor int64, reason string) error {
	return s.repo.InTransaction(ctx, func(r *Repo) error {
		old, err := s.activeKill(ctx, r, raidID, killID)
		if err != nil {
			return err
		}
		if _, err := s.check(ctx, r, raidID, actor, reason); err != nil {
			return err
		}
		if err := s.killWithoutLoot(ctx, r, old); err != nil {
			return err
		}
		kill := *old
		now := time.Now().UTC()
		kill.RemovedAt, kill.RemovedByPlayerID, kill.RemoveReason, kill.UpdatedAt = &now, &actor, strings.TrimSpace(reason), now
		if err := r.kills.Update(ctx, &kill); err != nil {
			return err
		}
		return r.audit.Record(ctx, actor, "raid_boss_kill", kill.ID, "remove", old, &kill, reason)
	})
}

func (s *Service) activeKill(ctx context.Context, r *Repo, raidID, killID int64) (*bosskill.Kill, error) {
	kill, err := r.kills.Get(ctx, killID)
	if err != nil {
		return nil, err
	}
	if kill.RaidID != raidID || kill.RemovedAt != nil {
		return nil, bosskill.ErrNotFound
	}
	return kill, nil
}

// killDifficulty accepts only a boss of the raid's instance and a difficulty
// that the catalog has for that boss at the raid's size (FR-29.6).
func (s *Service) killDifficulty(ctx context.Context, r *Repo, raidID int64, catalogBossID string, difficulty bosskill.Difficulty) (lootcatalog.Boss, error) {
	rd, err := r.raids.Get(ctx, raidID)
	if err != nil {
		return lootcatalog.Boss{}, referenceError(err)
	}
	if rd.CatalogInstanceID == nil || rd.RaidSize == nil {
		return lootcatalog.Boss{}, loot.ErrCatalogContextRequired
	}
	boss, modes, err := r.raids.Catalog().BossModes(ctx, *rd.CatalogInstanceID, catalogBossID, *rd.RaidSize)
	if errors.Is(err, lootcatalog.ErrInvalid) {
		return lootcatalog.Boss{}, bosskill.ErrInvalidBoss
	}
	if err != nil {
		return lootcatalog.Boss{}, err
	}
	for _, mode := range modes {
		if string(mode) == string(difficulty) {
			return boss, nil
		}
	}
	return lootcatalog.Boss{}, bosskill.ErrInvalidDifficulty
}

// killWithoutLoot rejects a change while the raid has unwithdrawn loot of the
// kill's boss (FR-29.9, FR-29.14).
func (s *Service) killWithoutLoot(ctx context.Context, r *Repo, kill *bosskill.Kill) error {
	awards, err := r.loot.ListByRaid(ctx, kill.RaidID)
	if err != nil {
		return err
	}
	for _, award := range awards {
		if award.WithdrawnAt == nil && award.CatalogBossID != nil && *award.CatalogBossID == kill.CatalogBossID {
			return bosskill.ErrHasLoot
		}
	}
	return nil
}
