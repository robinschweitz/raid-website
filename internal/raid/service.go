package raid

import (
	"context"
	"errors"
	"time"

	"raid-website/internal/validation"
)

var (
	ErrNotFound       = errors.New("raid not found")
	ErrActorNotFound  = errors.New("raid actor not found")
	ErrNotCancellable = errors.New("raid is not cancellable")
)

type Service struct {
	repo *Repo
}

func NewService(r *Repo) *Service {
	return &Service{repo: r}
}

func (s *Service) CancelRaid(ctx context.Context, raidID, actorID int64) (*Raid, error) {
	var rd *Raid
	err := s.repo.InTransaction(ctx, func(repo *Repo) error {
		if err := repo.RequireActor(ctx, actorID); err != nil {
			return err
		}
		var err error
		rd, err = repo.Get(ctx, raidID)
		if err != nil {
			return err
		}
		if rd.ArchivedAt != nil || IsCompletedOrCancelled(rd.Status) {
			return ErrNotCancellable
		}
		now := time.Now().UTC()
		rd.Status = validation.RaidStatusCancelled
		rd.CancelledAt = &now
		rd.UpdatedAt = now
		return repo.Update(ctx, rd)
	})
	return rd, err
}

func (s *Service) ArchiveRaid(ctx context.Context, raidID, actorID int64) (*Raid, error) {
	var rd *Raid
	err := s.repo.InTransaction(ctx, func(repo *Repo) error {
		if err := repo.RequireActor(ctx, actorID); err != nil {
			return err
		}
		var err error
		rd, err = repo.Get(ctx, raidID)
		if err != nil {
			return err
		}
		if rd.ArchivedAt != nil {
			return nil
		}
		now := time.Now().UTC()
		rd.ArchivedAt = &now
		rd.UpdatedAt = now
		return repo.Update(ctx, rd)
	})
	return rd, err
}

func IsCompletedOrCancelled(s validation.RaidStatus) bool {
	return s == validation.RaidStatusCompleted || s == validation.RaidStatusCancelled
}
