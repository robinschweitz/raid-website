package shadowmourne

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"raid-website/internal/character"
)

var (
	ErrInvalidProgress    = errors.New("invalid shard progress")
	ErrNotCurrent         = errors.New("entry is not the current collector")
	ErrNotQueued          = errors.New("entry is not queued")
	ErrQueueBoundary      = errors.New("entry cannot move further")
	ErrCharacterMissing   = errors.New("character not found")
	ErrProgressIncomplete = errors.New("shard progress is incomplete")
)

type Service struct {
	entries    *Repo
	characters *character.Repo
	now        func() time.Time
}

func NewService(entries *Repo, characters *character.Repo) *Service {
	return &Service{entries: entries, characters: characters, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) List(ctx context.Context) ([]Entry, error) {
	return s.entries.List(ctx)
}

func validProgress(progress ProgressInput) bool {
	return progress.ShardsCollected >= 0 && progress.ShardTarget > 0 && progress.ShardsCollected <= progress.ShardTarget
}

func (s *Service) Create(ctx context.Context, actorID int64, input CreateInput) (*Entry, error) {
	if input.CharacterID <= 0 || !validProgress(ProgressInput{ShardsCollected: input.ShardsCollected, ShardTarget: input.ShardTarget}) {
		return nil, ErrInvalidProgress
	}
	if _, err := s.characters.Get(ctx, input.CharacterID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCharacterMissing
		}
		return nil, err
	}
	var created *Entry
	err := s.entries.InTransaction(ctx, func(tx *Repo) error {
		position, err := tx.NextPosition(ctx)
		if err != nil {
			return err
		}
		active, err := tx.Active(ctx)
		if err != nil {
			return err
		}
		status := StatusQueued
		if active == nil {
			status = StatusCollecting
		}
		now := s.now()
		created = &Entry{CharacterID: input.CharacterID, QueuePosition: position, Status: status, ShardsCollected: input.ShardsCollected, ShardTarget: input.ShardTarget, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(ctx, created); err != nil {
			return err
		}
		created, err = tx.Get(ctx, created.ID)
		if err != nil {
			return err
		}
		return tx.Record(ctx, actorID, created.ID, "create", nil, created)
	})
	return created, err
}

func (s *Service) UpdateProgress(ctx context.Context, actorID, entryID int64, input ProgressInput) (*Entry, error) {
	if !validProgress(input) {
		return nil, ErrInvalidProgress
	}
	var updated *Entry
	err := s.entries.InTransaction(ctx, func(tx *Repo) error {
		before, err := tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		if before.Status != StatusCollecting {
			return ErrNotCurrent
		}
		copy := *before
		updated = &copy
		updated.ShardsCollected = input.ShardsCollected
		updated.ShardTarget = input.ShardTarget
		updated.UpdatedAt = s.now()
		if err := tx.SaveProgress(ctx, updated); err != nil {
			return err
		}
		updated, err = tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		return tx.Record(ctx, actorID, entryID, "update_progress", before, updated)
	})
	return updated, err
}

func (s *Service) Complete(ctx context.Context, actorID, entryID int64) (*Entry, error) {
	var completed *Entry
	err := s.entries.InTransaction(ctx, func(tx *Repo) error {
		before, err := tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		if before.Status != StatusCollecting {
			return ErrNotCurrent
		}
		if before.ShardsCollected != before.ShardTarget {
			return ErrProgressIncomplete
		}
		now := s.now()
		copy := *before
		completed = &copy
		completed.Status = StatusCompleted
		completed.CompletedAt = &now
		completed.UpdatedAt = now
		if err := tx.Complete(ctx, completed); err != nil {
			return err
		}
		next, err := tx.NextQueued(ctx)
		if err != nil {
			return err
		}
		if next != nil {
			next.Status = StatusCollecting
			next.UpdatedAt = now
			if err := tx.Activate(ctx, next); err != nil {
				return err
			}
			next, err = tx.Get(ctx, next.ID)
			if err != nil {
				return err
			}
		}
		completed, err = tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		return tx.Record(ctx, actorID, entryID, "complete", before, struct {
			*Entry
			NextCollector *Entry `json:"next_collector"`
		}{Entry: completed, NextCollector: next})
	})
	return completed, err
}

func (s *Service) Move(ctx context.Context, actorID, entryID int64, direction Direction) ([]Entry, error) {
	if direction != MoveUp && direction != MoveDown {
		return nil, ErrNotQueued
	}
	var entries []Entry
	err := s.entries.InTransaction(ctx, func(tx *Repo) error {
		entry, err := tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		if entry.Status != StatusQueued {
			return ErrNotQueued
		}
		neighbor, err := tx.Neighbor(ctx, entry, direction)
		if err != nil {
			return err
		}
		if neighbor == nil {
			return ErrQueueBoundary
		}
		before := struct {
			Entry    *Entry `json:"entry"`
			Neighbor *Entry `json:"neighbor"`
		}{Entry: entry, Neighbor: neighbor}
		if err := tx.SwapQueuePositions(ctx, entry, neighbor, s.now()); err != nil {
			return err
		}
		entries, err = tx.List(ctx)
		if err != nil {
			return err
		}
		moved, err := tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		movedNeighbor, err := tx.Get(ctx, neighbor.ID)
		if err != nil {
			return err
		}
		return tx.Record(ctx, actorID, entryID, "move", before, struct {
			*Entry
			Neighbor *Entry `json:"neighbor"`
		}{Entry: moved, Neighbor: movedNeighbor})
	})
	return entries, err
}
