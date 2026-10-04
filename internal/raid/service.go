package raid

import (
	"context"
	"errors"
	"time"

	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

var (
	ErrNotFound            = errors.New("raid not found")
	ErrActorNotFound       = errors.New("raid actor not found")
	ErrNotCancellable      = errors.New("raid is not cancellable")
	ErrScheduleNotEditable = errors.New("finalized, cancelled or archived raid schedule cannot be changed")
	ErrOccurrenceConflict  = errors.New("raid series occurrence or successor already exists")
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

// ScheduleChanges contains only explicitly supplied scheduling prerequisites.
type ScheduleChanges struct {
	SeriesID       *int64
	ScheduledStart *time.Time
}

// ConfigureSchedule changes active scheduling prerequisites without rewriting raid history.
func (s *Service) ConfigureSchedule(ctx context.Context, raidID, actorID int64, changes ScheduleChanges) (*Raid, error) {
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
		if rd.ArchivedAt != nil || (rd.Status != validation.RaidStatusPlanned && rd.Status != validation.RaidStatusOpen) {
			return ErrScheduleNotEditable
		}
		if changes.SeriesID != nil {
			if *changes.SeriesID <= 0 {
				return raidseries.ErrSeriesRequired
			}
			seriesRepo := raidseries.NewRepo(repo.db)
			series, err := seriesRepo.Get(ctx, *changes.SeriesID)
			if err != nil {
				return err
			}
			if err := raidseries.Validate(series); err != nil {
				return err
			}
			if err := seriesRepo.RequireLeader(ctx, series.RaidLeaderID); err != nil {
				return err
			}
			rd.SeriesID = changes.SeriesID
		}
		if changes.ScheduledStart != nil {
			start := changes.ScheduledStart.UTC()
			rd.ScheduledStart = &start
		}
		if changes.SeriesID == nil && changes.ScheduledStart == nil {
			return nil
		}
		now := time.Now().UTC()
		rd.UpdatedAt = now
		return repo.Update(ctx, rd)
	})
	if err != nil {
		return nil, err
	}
	return rd, nil
}

// CreateRaid accepts concrete legacy manual paths, while validating any selected series.
func (s *Service) CreateRaid(ctx context.Context, rd *Raid) error {
	return s.repo.InTransaction(ctx, func(repo *Repo) error {
		if rd.SeriesID != nil {
			seriesRepo := raidseries.NewRepo(repo.db)
			series, err := seriesRepo.Get(ctx, *rd.SeriesID)
			if err != nil {
				return err
			}
			if err := raidseries.Validate(series); err != nil {
				return err
			}
			if err := seriesRepo.RequireLeader(ctx, series.RaidLeaderID); err != nil {
				return err
			}
		}
		return repo.Create(ctx, rd)
	})
}

func IsCompletedOrCancelled(s validation.RaidStatus) bool {
	return s == validation.RaidStatusCompleted || s == validation.RaidStatusCancelled
}
