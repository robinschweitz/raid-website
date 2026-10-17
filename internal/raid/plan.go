package raid

import (
	"context"
	"errors"
	"time"

	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

var (
	// ErrPlanUntilPast rejects a planning date that is not in the future.
	ErrPlanUntilPast = errors.New("planning date must be in the future")
	// ErrPlanTooFar rejects planning beyond raidseries.MaxPlanWeeks.
	ErrPlanTooFar = errors.New("planning date is too far in the future")
)

// PlanSeriesUntil creates every missing occurrence of the series up to and
// including the given local calendar day (ADR 078). Only the year, month and
// day of untilDate are used; the day ends at 23:59 in the series timezone.
func (s *Service) PlanSeriesUntil(ctx context.Context, seriesID, actorID int64, untilDate time.Time) ([]Raid, error) {
	var created []Raid
	err := s.repo.InTransaction(ctx, func(repo *Repo) error {
		if err := repo.RequireActor(ctx, actorID); err != nil {
			return err
		}
		series, err := raidseries.NewRepo(repo.db).Get(ctx, seriesID)
		if err != nil {
			return err
		}
		location, err := time.LoadLocation(series.Timezone)
		if err != nil {
			return errors.Join(raidseries.ErrInvalid, err)
		}
		until := time.Date(untilDate.Year(), untilDate.Month(), untilDate.Day(), 23, 59, 59, 0, location)
		created, err = PlanOccurrences(ctx, repo, series, actorID, until, time.Now().UTC())
		return err
	})
	return created, err
}

// PlanAhead tops up the series' automatic planning window, if one is set.
func (s *Service) PlanAhead(ctx context.Context, seriesID, actorID int64) ([]Raid, error) {
	var created []Raid
	err := s.repo.InTransaction(ctx, func(repo *Repo) error {
		var err error
		created, err = PlanAheadInTransaction(ctx, repo, seriesID, actorID, time.Now().UTC())
		return err
	})
	return created, err
}

// PlanAheadInTransaction is PlanAhead for callers that already hold a
// lifecycle transaction, such as archiving.
func PlanAheadInTransaction(ctx context.Context, repo *Repo, seriesID, actorID int64, now time.Time) ([]Raid, error) {
	series, err := raidseries.NewRepo(repo.db).Get(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	if series.PlanWeeksAhead <= 0 {
		return nil, nil
	}
	return PlanOccurrences(ctx, repo, series, actorID, now.AddDate(0, 0, 7*series.PlanWeeksAhead), now)
}

// PlanOccurrences creates the series' occurrences after the latest existing
// raid of the series (or after now, if it has none) up to until. Elapsed
// occurrences are skipped, existing ones are never touched, and every created
// raid opens its signup right away. The schedule continues from the latest
// raid so that multi-week intervals keep their rhythm.
func PlanOccurrences(ctx context.Context, repo *Repo, series *raidseries.Series, actorID int64, until, now time.Time) ([]Raid, error) {
	created := []Raid{}
	if !until.After(now) {
		return nil, ErrPlanUntilPast
	}
	if until.After(now.AddDate(0, 0, 7*raidseries.MaxPlanWeeks)) {
		return nil, ErrPlanTooFar
	}
	if err := raidseries.Validate(series); err != nil {
		return nil, err
	}
	seriesRepo := raidseries.NewRepo(repo.db)
	if err := seriesRepo.RequireLeader(ctx, series.RaidLeaderID); err != nil {
		return nil, err
	}
	if err := repo.Catalog().ValidateAssociation(ctx, series.CatalogInstanceID, series.RaidSize); err != nil {
		return nil, err
	}
	existing, err := repo.ListBySeries(ctx, series.ID)
	if err != nil {
		return nil, err
	}
	// Continue from the latest raid of the series, even a past one; NextOccurrence
	// skips elapsed occurrences and keeps the multi-week rhythm of that raid.
	cursor := now
	var latest time.Time
	for _, rd := range existing {
		if rd.ScheduledStart != nil && rd.ScheduledStart.After(latest) {
			latest = *rd.ScheduledStart
		}
	}
	if !latest.IsZero() {
		cursor = latest
	}
	counterChanged := false
	for {
		next, err := raidseries.NextOccurrence(series, cursor, now)
		if err != nil {
			return nil, err
		}
		if next.After(until) {
			break
		}
		cursor = next
		found, err := repo.FindOccurrence(ctx, series.ID, next)
		if err != nil {
			return nil, err
		}
		if found != nil {
			continue
		}
		start := next
		rd := &Raid{Name: series.Name, RaidLeaderID: series.RaidLeaderID, SeriesID: &series.ID, Status: validation.RaidStatusPlanned, ScheduledStart: &start, CreatedAt: now, UpdatedAt: now}
		rd.CatalogInstanceID, rd.RaidSize = series.CatalogInstanceID, series.RaidSize
		if series.NameTemplate != "" {
			rd.Name = raidseries.EvaluateName(series.NameTemplate, series.NextRaidNumber, series.Name, start)
			series.NextRaidNumber++
			counterChanged = true
		}
		if err := repo.Create(ctx, rd); err != nil {
			return nil, err
		}
		if err := repo.OpenSignups(ctx, rd.ID); err != nil {
			return nil, err
		}
		if err := repo.Audit(ctx, actorID, rd.ID, "plan", nil, rd); err != nil {
			return nil, err
		}
		created = append(created, *rd)
	}
	if counterChanged {
		if err := seriesRepo.Save(ctx, series); err != nil {
			return nil, err
		}
	}
	return created, nil
}
