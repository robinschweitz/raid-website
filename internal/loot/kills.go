package loot

import (
	"context"
	"errors"
	"time"

	"raid-website/internal/bosskill"
)

var ErrDifficultyRequired = errors.New("boss difficulty required")
var ErrKillDifficultyConflict = errors.New("boss kill difficulty changed")

// EnsureBossKill joins the caller's transaction: new loot and its kill commit or
// roll back together. The writer must already be reserved before reading kills.
func EnsureBossKill(ctx context.Context, repo *Repo, a *Award, reason string) error {
	validated := *a
	if err := ValidateCatalog(ctx, repo, &validated, nil); err != nil {
		return err
	}
	kills, err := repo.Kills().ListByRaid(ctx, a.RaidID)
	if err != nil {
		return err
	}
	for _, kill := range kills {
		if a.CatalogBossID != nil && kill.CatalogBossID == *a.CatalogBossID {
			if a.Difficulty != "" && a.Difficulty != kill.Difficulty {
				return ErrKillDifficultyConflict
			}
			a.Difficulty = kill.Difficulty
			return nil
		}
	}
	if a.Difficulty == "" {
		return ErrDifficultyRequired
	}
	rd, err := repo.awardRepositories().raids.Get(ctx, a.RaidID)
	if err != nil {
		return err
	}
	boss, modes, err := repo.Catalog().BossModes(ctx, *rd.CatalogInstanceID, *a.CatalogBossID, *rd.RaidSize)
	if err != nil {
		return err
	}
	allowed := false
	for _, mode := range modes {
		if string(mode) == string(a.Difficulty) {
			allowed = true
		}
	}
	if !allowed {
		return bosskill.ErrInvalidDifficulty
	}
	now := time.Now().UTC()
	kill := &bosskill.Kill{RaidID: a.RaidID, CatalogBossID: boss.ID, Boss: boss.Name, Difficulty: a.Difficulty, CreatedByPlayerID: a.AwardedByPlayerID, CreatedAt: now, UpdatedAt: now}
	if err := repo.Kills().Create(ctx, kill); err != nil {
		return err
	}
	return repo.awardRepositories().audit.Record(ctx, a.AwardedByPlayerID, "raid_boss_kill", kill.ID, "create", nil, kill, reason)
}
