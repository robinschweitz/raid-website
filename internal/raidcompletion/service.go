package raidcompletion

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"raid-website/internal/enhancepoints"
	"raid-website/internal/validation"
)

const noMainSpecAward = 10

var (
	ErrRaidCancelled      = errors.New("raid is cancelled")
	ErrRaidNotCompletable = errors.New("raid is not completable")
)

// Service completes raids and applies the cross-domain consequences:
// enhance point awards for players without main-spec loot, and lock expiry.
type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service {
	return &Service{repo: repo}
}

// CompleteRaid marks the raid completed and awards enhance points to each
// participant who did not receive a main-spec item. It is idempotent: an
// already-completed raid is a no-op, and duplicate point awards are skipped.
func (s *Service) CompleteRaid(ctx context.Context, raidID, actorID int64) error {
	return s.repo.InTransaction(ctx, func(repo *Repo) error {
		raids := repo.raids
		att := repo.attendance
		lo := repo.loot
		ep := repo.points
		if err := raids.RequireActor(ctx, actorID); err != nil {
			return err
		}

		rd, err := raids.Get(ctx, raidID)
		if err != nil {
			return fmt.Errorf("load raid %d: %w", raidID, err)
		}

		switch rd.Status {
		case validation.RaidStatusCancelled:
			return ErrRaidCancelled
		case validation.RaidStatusCompleted:
			return nil
		case validation.RaidStatusPlanned, validation.RaidStatusOpen:
			if rd.ArchivedAt != nil {
				return ErrRaidNotCompletable
			}
		default:
			return ErrRaidNotCompletable
		}

		now := time.Now().UTC()
		rd.Status = validation.RaidStatusCompleted
		rd.CompletedAt = &now
		rd.UpdatedAt = now
		if err := raids.Update(ctx, rd); err != nil {
			return fmt.Errorf("complete raid %d: %w", raidID, err)
		}

		rows, err := att.ListByRaid(ctx, raidID)
		if err != nil {
			return fmt.Errorf("load attendance for raid %d: %w", raidID, err)
		}

		awardedPlayers := make(map[int64]struct{}, len(rows))
		for _, a := range rows {
			if _, done := awardedPlayers[a.PlayerID]; done {
				continue
			}
			awardedPlayers[a.PlayerID] = struct{}{}

			hasMainSpec, err := lo.PlayerHasMainSpecAward(ctx, raidID, a.PlayerID)
			if err != nil {
				return fmt.Errorf("check main-spec loot for player %d: %w", a.PlayerID, err)
			}
			if hasMainSpec {
				continue
			}

			alreadyAwarded, err := ep.HasNoMainSpecAward(ctx, raidID, a.PlayerID)
			if err != nil {
				return fmt.Errorf("check existing award for player %d: %w", a.PlayerID, err)
			}
			if alreadyAwarded {
				continue
			}

			raidIDCopy := raidID
			t := &enhancepoints.Transaction{
				PlayerID:  a.PlayerID,
				RaidID:    &raidIDCopy,
				Amount:    noMainSpecAward,
				Type:      "award_no_mainspec_loot",
				Reason:    fmt.Sprintf("raid %d completed without main-spec loot", raidID),
				RefType:   "raid",
				RefID:     strconv.FormatInt(raidID, 10),
				ActorID:   actorID,
				CreatedAt: now,
			}
			if err := ep.InsertTransaction(ctx, t); err != nil {
				return fmt.Errorf("award points to player %d: %w", a.PlayerID, err)
			}
		}

		locks, err := ep.ListActiveLocksByRaid(ctx, raidID)
		if err != nil {
			return fmt.Errorf("load active locks for raid %d: %w", raidID, err)
		}
		for i := range locks {
			locks[i].Status = validation.LockStatusExpired
			locks[i].ExpiredAt = &now
			locks[i].UpdatedAt = now
			if err := ep.UpdateLock(ctx, &locks[i]); err != nil {
				return fmt.Errorf("expire lock %d: %w", locks[i].ID, err)
			}
		}

		return nil
	})
}
