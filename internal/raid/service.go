package raid

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

var (
	ErrNotFound            = errors.New("raid not found")
	ErrActorNotFound       = errors.New("raid actor not found")
	ErrNameRequired        = errors.New("raid name is required")
	ErrNotCancellable      = errors.New("raid is not cancellable")
	ErrNotCancelled        = errors.New("raid is not cancelled")
	ErrScheduleNotEditable = errors.New("finalized, cancelled or archived raid schedule cannot be changed")
	// ErrEPLocked: the raid has EP rows or locks, so its pool and award stay (FR-16.11).
	ErrEPLocked           = errors.New("raid ep pool is fixed by existing points or locks")
	ErrOccurrenceConflict = errors.New("raid series occurrence or successor already exists")
	ErrLeadersInvalid     = errors.New("raid leaders must be active players, each once, at least one")
	ErrReasonRequired     = errors.New("changes to an archived raid need a reason")
	// ErrEPInvalid: an EP award outside 0..100 (FR-16.11).
	ErrEPInvalid = errors.New("raid ep award is out of range")
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
		before := *rd
		now := time.Now().UTC()
		rd.Status = validation.RaidStatusCancelled
		rd.CancelledAt = &now
		rd.UpdatedAt = now
		if err := repo.Update(ctx, rd); err != nil {
			return err
		}
		if err := repo.Audit(ctx, actorID, rd.ID, "cancel", &before, rd); err != nil {
			return err
		}
		return enhancepoints.NewRepo(repo.db).ExpireRaidLocks(ctx, actorID, rd.ID, now)
	})
	return rd, err
}

// UncancelRaid takes back the cancellation of an unarchived raid (ADR 115). The raid
// gets the status it had before the cancellation, read from the cancel audit entry;
// without one it becomes PLANNED. Locks that the cancellation expired stay expired.
func (s *Service) UncancelRaid(ctx context.Context, raidID, actorID int64) (*Raid, error) {
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
		if rd.ArchivedAt != nil || rd.Status != validation.RaidStatusCancelled {
			return ErrNotCancelled
		}
		previous, err := statusBeforeCancel(ctx, repo, raidID)
		if err != nil {
			return err
		}
		before := *rd
		rd.Status = previous
		rd.CancelledAt = nil
		rd.UpdatedAt = time.Now().UTC()
		if err := repo.Update(ctx, rd); err != nil {
			return err
		}
		return repo.Audit(ctx, actorID, rd.ID, "uncancel", &before, rd)
	})
	if err != nil {
		return nil, err
	}
	return rd, nil
}

// statusBeforeCancel reads the status from the newest cancel audit entry of a raid.
func statusBeforeCancel(ctx context.Context, repo *Repo, raidID int64) (validation.RaidStatus, error) {
	logs, err := audit.NewRepo(repo.db).ListByEntity(ctx, "raid", strconv.FormatInt(raidID, 10))
	if err != nil {
		return "", err
	}
	for i := len(logs) - 1; i >= 0; i-- {
		if logs[i].Action != "cancel" {
			continue
		}
		var snapshot struct {
			Status validation.RaidStatus `json:"status"`
		}
		if json.Unmarshal([]byte(logs[i].BeforeJSON), &snapshot) == nil && snapshot.Status == validation.RaidStatusOpen {
			return validation.RaidStatusOpen, nil
		}
		break
	}
	return validation.RaidStatusPlanned, nil
}

// sameEP reports whether a raid already has the series' EP pool and award.
func sameEP(rd *Raid, series *raidseries.Series) bool {
	if rd.EPAward != series.EPAward || (rd.EPPoolID == nil) != (series.EPPoolID == nil) {
		return false
	}
	return rd.EPPoolID == nil || *rd.EPPoolID == *series.EPPoolID
}

// ScheduleChanges contains only explicitly supplied scheduling prerequisites.
type ScheduleChanges struct {
	Name              *string
	CatalogInstanceID *string
	RaidSize          *int
	SeriesID          *int64
	ScheduledStart    *time.Time
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
		before := *rd
		var newSeries *raidseries.Series
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
			newSeries = series
		}
		if changes.ScheduledStart != nil {
			start := changes.ScheduledStart.UTC()
			rd.ScheduledStart = &start
		}
		if changes.Name != nil {
			name := strings.TrimSpace(*changes.Name)
			if name == "" {
				return ErrNameRequired
			}
			rd.Name = name
		}
		if changes.CatalogInstanceID != nil {
			rd.CatalogInstanceID = changes.CatalogInstanceID
		}
		if changes.RaidSize != nil {
			rd.RaidSize = changes.RaidSize
		}
		if err := repo.Catalog().ValidateAssociation(ctx, rd.CatalogInstanceID, rd.RaidSize); err != nil {
			return err
		}
		if changes.Name == nil && changes.SeriesID == nil && changes.ScheduledStart == nil && changes.CatalogInstanceID == nil && changes.RaidSize == nil {
			return nil
		}
		now := time.Now().UTC()
		rd.UpdatedAt = now
		if err := repo.Update(ctx, rd); err != nil {
			return err
		}
		if newSeries != nil && !sameEP(rd, newSeries) {
			// Points already booked or locked in the raid's pool fix it (FR-16.11).
			hasEP, err := enhancepoints.NewRepo(repo.db).RaidHasEP(ctx, rd.ID)
			if err != nil {
				return err
			}
			if hasEP {
				return ErrEPLocked
			}
			if err := repo.CopySeriesEP(ctx, rd, newSeries); err != nil {
				return err
			}
		}
		return repo.Audit(ctx, actorID, rd.ID, "update", &before, rd)
	})
	if err != nil {
		return nil, err
	}
	return rd, nil
}

// CreateOptions are the choices staff make on the new-raid form beyond the raid row.
type CreateOptions struct {
	// LeaderIDs replaces the leader list in order and wins over raid_leader_id; nil
	// keeps raid_leader_id first, followed by the series' other leaders (FR-10.17).
	LeaderIDs []int64
	// EPPoolID sets the EP pool; a pointer to 0 means no pool. Nil takes the
	// series' pool, or the default pool without a series (FR-16.11).
	EPPoolID *int64
	// EPAward sets the EP award; nil takes the series' award or the default.
	EPAward *int
}

// CreateRaid accepts concrete legacy manual paths, while validating any selected series.
// A sent name wins; without one the selected series derives it: the naming template with
// the next counter, or the series name when the template is empty. A series raid always
// consumes the counter (incremented in the same transaction), so numbering stays in step.
func (s *Service) CreateRaid(ctx context.Context, rd *Raid, actorID int64, options CreateOptions) error {
	rd.Name = strings.TrimSpace(rd.Name)
	var leaderIDs []int64
	if options.LeaderIDs != nil {
		if err := checkLeaderList(options.LeaderIDs); err != nil {
			return err
		}
		leaderIDs = options.LeaderIDs
		rd.RaidLeaderID = leaderIDs[0]
	}
	if options.EPAward != nil && !enhancepoints.ValidAward(*options.EPAward) {
		return ErrEPInvalid
	}
	return s.repo.InTransaction(ctx, func(repo *Repo) error {
		var seriesLeaderIDs []int64
		var fromSeries *raidseries.Series
		if rd.SeriesID != nil {
			seriesRepo := raidseries.NewRepo(repo.db)
			series, err := seriesRepo.Get(ctx, *rd.SeriesID)
			if err != nil {
				return err
			}
			seriesLeaderIDs = series.LeaderIDs
			fromSeries = series
			if err := raidseries.Validate(series); err != nil {
				return err
			}
			if err := seriesRepo.RequireLeader(ctx, series.RaidLeaderID); err != nil {
				return err
			}
			reference := time.Now().UTC()
			if rd.ScheduledStart != nil {
				reference = rd.ScheduledStart.UTC()
			}
			derived := series.Name
			if series.NameTemplate != "" {
				derived = raidseries.EvaluateName(series.NameTemplate, series.NextRaidNumber, series.Name, reference)
				series.NextRaidNumber++
				if err := seriesRepo.Save(ctx, series); err != nil {
					return err
				}
			}
			if rd.Name == "" {
				rd.Name = derived
			}
			if rd.CatalogInstanceID == nil {
				rd.CatalogInstanceID = series.CatalogInstanceID
			}
			if rd.RaidSize == nil {
				rd.RaidSize = series.RaidSize
			}
		}
		if rd.Name == "" {
			return ErrNameRequired
		}
		if err := repo.Catalog().ValidateAssociation(ctx, rd.CatalogInstanceID, rd.RaidSize); err != nil {
			return err
		}
		for _, id := range leaderIDs {
			exists, err := repo.ActivePlayerExists(ctx, id)
			if err != nil {
				return err
			}
			if !exists {
				return ErrLeadersInvalid
			}
		}
		if err := repo.Create(ctx, rd); err != nil {
			return err
		}
		if leaderIDs != nil {
			if err := repo.ReplaceExtraLeaders(ctx, rd.ID, leaderIDs[1:]); err != nil {
				return err
			}
		} else if err := repo.CopySeriesLeaders(ctx, rd, seriesLeaderIDs); err != nil {
			return err
		}
		if fromSeries != nil {
			if err := repo.CopySeriesEP(ctx, rd, fromSeries); err != nil {
				return err
			}
		} else if err := repo.LoadEP(ctx, rd); err != nil {
			return err
		}
		if options.EPPoolID != nil || options.EPAward != nil {
			poolID, award := rd.EPPoolID, rd.EPAward
			if options.EPPoolID != nil {
				poolID = nil
				if *options.EPPoolID != 0 {
					if _, err := enhancepoints.NewRepo(repo.db).GetPool(ctx, *options.EPPoolID); err != nil {
						if errors.Is(err, enhancepoints.ErrPoolNotFound) {
							return raidseries.ErrPoolNotFound
						}
						return err
					}
					id := *options.EPPoolID
					poolID = &id
				}
			}
			if options.EPAward != nil {
				award = *options.EPAward
			}
			if err := repo.SetEP(ctx, rd, poolID, award); err != nil {
				return err
			}
		}
		if err := repo.Audit(ctx, actorID, rd.ID, "create", nil, rd); err != nil {
			return err
		}
		if leaderIDs == nil {
			return nil
		}
		return audit.NewRepo(repo.db).Record(ctx, actorID, "raid", rd.ID, "set_leaders", nil, map[string]any{"leader_ids": leaderIDs}, "")
	})
}

// checkLeaderList rejects an empty list, nonpositive IDs, and repeated players.
func checkLeaderList(playerIDs []int64) error {
	if len(playerIDs) == 0 {
		return ErrLeadersInvalid
	}
	seen := map[int64]bool{}
	for _, id := range playerIDs {
		if id <= 0 || seen[id] {
			return ErrLeadersInvalid
		}
		seen[id] = true
	}
	return nil
}

// Leaders lists all raid leaders of a raid; the first is raids.raid_leader_id (FR-10.17).
func (s *Service) Leaders(ctx context.Context, raidID int64) ([]int64, error) {
	rd, err := s.repo.Get(ctx, raidID)
	if err != nil {
		return nil, err
	}
	extra, err := s.repo.ExtraLeaders(ctx, raidID)
	if err != nil {
		return nil, err
	}
	return append([]int64{rd.RaidLeaderID}, extra...), nil
}

// SetLeaders replaces the raid leaders of a planned, open, or completed raid, also
// an archived one, which needs a reason (FR-13.11, FR-26.5). Leaders are shown
// only; they grant no permissions (ADR 109).
func (s *Service) SetLeaders(ctx context.Context, raidID, actorID int64, playerIDs []int64, reason string) ([]int64, error) {
	reason = strings.TrimSpace(reason)
	if err := checkLeaderList(playerIDs); err != nil {
		return nil, err
	}
	err := s.repo.InTransaction(ctx, func(repo *Repo) error {
		if err := repo.RequireActor(ctx, actorID); err != nil {
			return err
		}
		rd, err := repo.Get(ctx, raidID)
		if err != nil {
			return err
		}
		if rd.Status == validation.RaidStatusCancelled || (rd.ArchivedAt != nil && rd.Status != validation.RaidStatusCompleted) {
			return ErrScheduleNotEditable
		}
		if rd.ArchivedAt != nil && reason == "" {
			return ErrReasonRequired
		}
		for _, id := range playerIDs {
			exists, err := repo.ActivePlayerExists(ctx, id)
			if err != nil {
				return err
			}
			if !exists {
				return ErrLeadersInvalid
			}
		}
		extra, err := repo.ExtraLeaders(ctx, raidID)
		if err != nil {
			return err
		}
		before := append([]int64{rd.RaidLeaderID}, extra...)
		rd.RaidLeaderID = playerIDs[0]
		rd.UpdatedAt = time.Now().UTC()
		if err := repo.Update(ctx, rd); err != nil {
			return err
		}
		if err := repo.ReplaceExtraLeaders(ctx, raidID, playerIDs[1:]); err != nil {
			return err
		}
		return audit.NewRepo(repo.db).Record(ctx, actorID, "raid", raidID, "set_leaders", map[string]any{"leader_ids": before}, map[string]any{"leader_ids": playerIDs}, reason)
	})
	if err != nil {
		return nil, err
	}
	return playerIDs, nil
}

func IsCompletedOrCancelled(s validation.RaidStatus) bool {
	return s == validation.RaidStatusCompleted || s == validation.RaidStatusCancelled
}
