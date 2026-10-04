package loot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"raid-website/internal/enhancepoints"
	"raid-website/internal/tokens"
	"raid-website/internal/validation"
)

var (
	ErrRaidState    = errors.New("raid does not allow ordinary loot awards")
	ErrNotFound     = errors.New("referenced record not found")
	ErrOwnership    = errors.New("character does not belong to recipient")
	ErrLockConflict = errors.New("multiple active locks match item")
	ErrInvalidLock  = errors.New("item lock must match an active lock for this raid and item")
)

// Service coordinates loot awards with derived token transactions.
type Service struct {
	awards *Repo
}

// NewService constructs a loot service using the repository transaction boundary.
func NewService(repo *Repo) *Service {
	return &Service{awards: repo}
}

// Award persists a loot award and, when the item is an armor token,
// records the derived token transaction in the same database transaction.
func (s *Service) Award(ctx context.Context, a *Award) error {
	return s.awards.InTransaction(ctx, func(awards *Repo) error { return AwardInTransaction(ctx, awards, a) })
}

// AwardInTransaction applies ordinary loot consequences using the caller's repository transaction.
func AwardInTransaction(ctx context.Context, awards *Repo, a *Award) error {
	repositories := awards.awardRepositories()
	rd, err := repositories.raids.Get(ctx, a.RaidID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if rd.ArchivedAt != nil || (rd.Status != validation.RaidStatusPlanned && rd.Status != validation.RaidStatusOpen) {
		return ErrRaidState
	}
	for _, id := range []int64{a.AwardedToPlayerID, a.AwardedByPlayerID} {
		if _, err := repositories.players.Get(ctx, id); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
	}
	if a.AwardedToCharacterID != nil {
		c, err := repositories.characters.Get(ctx, *a.AwardedToCharacterID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if c.PlayerID != a.AwardedToPlayerID {
			return ErrOwnership
		}
	}
	points := repositories.points
	locks, err := points.ListActiveLocksByRaid(ctx, a.RaidID)
	if err != nil {
		return err
	}
	matching, err := enhancepoints.ResolveActiveLock(locks, a.ItemID, a.ItemName)
	if errors.Is(err, enhancepoints.ErrLockConflict) {
		return ErrLockConflict
	}
	if err != nil {
		return err
	}
	if a.ItemLockID != nil {
		if _, err := points.GetLock(ctx, *a.ItemLockID); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if matching == nil || matching.ID != *a.ItemLockID {
			return ErrInvalidLock
		}
	}
	if matching != nil {
		if a.AwardedToPlayerID != matching.PlayerID {
			a.AwardedToCharacterID = nil
		}
		a.AwardedToPlayerID = matching.PlayerID
		a.IsMainSpec = true
		a.ItemLockID = &matching.ID
		matching.Status = validation.LockStatusUsed
		now := time.Now().UTC()
		matching.UsedAt = &now
		matching.UpdatedAt = now
		if err := points.UpdateLock(ctx, matching); err != nil {
			return err
		}
	}

	if err := awards.CreateAward(ctx, a); err != nil {
		return fmt.Errorf("create award: %w", err)
	}

	if tokens.IsTokenAward(a.ItemName) {
		raidID := a.RaidID
		awardID := a.ID
		t := &tokens.Transaction{
			PlayerID:    a.AwardedToPlayerID,
			RaidID:      &raidID,
			LootAwardID: &awardID,
			Amount:      tokens.DeriveTokenAmount(1),
			Type:        "award",
			Reason:      a.ItemName,
			RefType:     "loot_award",
			RefID:       fmt.Sprintf("%d", a.ID),
			ActorID:     a.AwardedByPlayerID,
			CreatedAt:   a.AwardedAt,
		}
		if err := repositories.tokens.InsertTransaction(ctx, t); err != nil {
			return fmt.Errorf("derive token: %w", err)
		}
	}

	return nil
}
