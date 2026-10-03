package raidcompletion

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/raid"
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
	db         *bun.DB
	raids      *raid.Repo
	attendance *attendance.Repo
	loot       *loot.Repo
	points     *enhancepoints.Repo
}

func New(
	db *bun.DB,
	raids *raid.Repo,
	att *attendance.Repo,
	lo *loot.Repo,
	ep *enhancepoints.Repo,
) *Service {
	return &Service{
		db:         db,
		raids:      raids,
		attendance: att,
		loot:       lo,
		points:     ep,
	}
}

// CompleteRaid marks the raid completed and awards enhance points to each
// participant who did not receive a main-spec item. It is idempotent: an
// already-completed raid is a no-op, and duplicate point awards are skipped.
func (s *Service) CompleteRaid(ctx context.Context, raidID, actorID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	raids := s.raids.WithTx(tx)
	att := s.attendance.WithTx(tx)
	lo := s.loot.WithTx(tx)
	ep := s.points.WithTx(tx)

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
		// completable
	default:
		return ErrRaidNotCompletable
	}

	now := time.Now().UTC()
	rd.Status = validation.RaidStatusCompleted
	rd.CompletedAt = &now
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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit raid completion: %w", err)
	}
	return nil
}
