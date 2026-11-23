package raid

import (
	"context"
	"fmt"

	"raid-website/internal/enhancepoints"
	"raid-website/internal/raidseries"
)

// RaidRef names a raid in an answer.
type RaidRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ConfigureSeries saves a series and, in the same transaction, gives its
// planned or open, unarchived raids the series' EP pool and award when those
// changed (ADR 125). A raid that already has EP transactions or item locks
// keeps its setting and is returned, so staff can see it.
func (s *Service) ConfigureSeries(ctx context.Context, id int64, changes raidseries.Changes) (*raidseries.Series, []RaidRef, error) {
	var saved *raidseries.Series
	kept := []RaidRef{}
	err := s.repo.InTransaction(ctx, func(repo *Repo) error {
		seriesRepo := raidseries.NewRepo(repo.db)
		var before *raidseries.Series
		if id != 0 {
			current, err := seriesRepo.Get(ctx, id)
			if err != nil {
				return err
			}
			before = current
		}
		var err error
		saved, err = raidseries.ConfigureIn(ctx, seriesRepo, id, changes)
		if err != nil {
			return err
		}
		if before == nil || sameSeriesEP(before, saved) {
			return nil
		}
		kept, err = applySeriesEP(ctx, repo, saved, changes.ActorID)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return saved, kept, nil
}

func applySeriesEP(ctx context.Context, repo *Repo, series *raidseries.Series, actorID int64) ([]RaidRef, error) {
	raids, err := repo.ListOpenBySeries(ctx, series.ID)
	if err != nil {
		return nil, fmt.Errorf("list open raids of series %d: %w", series.ID, err)
	}
	points := enhancepoints.NewRepo(repo.db)
	kept := []RaidRef{}
	for i := range raids {
		rd := &raids[i]
		if sameEP(rd, series) {
			continue
		}
		hasEP, err := points.RaidHasEP(ctx, rd.ID)
		if err != nil {
			return nil, err
		}
		if hasEP {
			kept = append(kept, RaidRef{ID: rd.ID, Name: rd.Name})
			continue
		}
		before := *rd
		if err := repo.CopySeriesEP(ctx, rd, series); err != nil {
			return nil, err
		}
		if err := repo.Audit(ctx, actorID, rd.ID, "series_ep", &before, rd); err != nil {
			return nil, err
		}
	}
	return kept, nil
}

func sameSeriesEP(a, b *raidseries.Series) bool {
	if a.EPAward != b.EPAward || (a.EPPoolID == nil) != (b.EPPoolID == nil) {
		return false
	}
	return a.EPPoolID == nil || *a.EPPoolID == *b.EPPoolID
}
