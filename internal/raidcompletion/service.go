package raidcompletion

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

const noMainSpecAward = 10

var (
	ErrRaidCancelled     = errors.New("raid is cancelled")
	ErrRaidNotArchivable = errors.New("raid is not archivable")
	ErrRosterIncomplete  = errors.New("raid roster is incomplete")
)

// Service archives raids with point awards, lock expiry and successor scheduling.
type Service struct {
	repo *Repo
	now  func() time.Time
}

func NewService(repo *Repo) *Service {
	return &Service{repo: repo, now: time.Now}
}

// ArchiveRaid finalizes rewards, locks and the next occurrence atomically.
// Stored COMPLETED raids are historical: never recreate their rewards or cadence.
func (s *Service) ArchiveRaid(ctx context.Context, raidID, actorID int64) (*raid.Raid, *raid.Raid, error) {
	var archived, next *raid.Raid
	err := s.repo.InTransaction(ctx, func(repo *Repo) error {
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
		archived = rd
		now := s.now().UTC().Truncate(time.Microsecond)

		switch rd.Status {
		case validation.RaidStatusCancelled:
			return ErrRaidCancelled
		case validation.RaidStatusCompleted:
			if rd.ArchivedAt == nil {
				rd.ArchivedAt = &now
				rd.UpdatedAt = now
				if err := raids.Update(ctx, rd); err != nil {
					return err
				}
			}
			next, err = raids.Successor(ctx, raidID)
			return err
		case validation.RaidStatusPlanned, validation.RaidStatusOpen:
			// Old archived active raids require this explicit finalization request.
		default:
			return ErrRaidNotArchivable
		}

		if rd.SeriesID == nil {
			return raidseries.ErrSeriesRequired
		}
		if rd.ScheduledStart == nil {
			return raidseries.ErrScheduledStartRequired
		}
		rows, err := att.ListByRaid(ctx, raidID)
		if err != nil {
			return fmt.Errorf("load attendance for raid %d: %w", raidID, err)
		}
		for _, a := range rows {
			_, validClass := validation.ClassSpecs[attendance.NormalizeClass(a.Class)]
			role := validation.AttendanceRole(strings.ToUpper(strings.TrimSpace(string(a.Role))))
			validRole := role == validation.RoleTank || role == validation.RoleHeal || role == validation.RoleDD
			if strings.TrimSpace(a.CharacterName) == "" || strings.TrimSpace(a.Spec) == "" || !validClass || !validRole {
				return fmt.Errorf("%w: attendance %d requires character_name, valid class, spec and role", ErrRosterIncomplete, a.ID)
			}
		}
		series, err := repo.series.Get(ctx, *rd.SeriesID)
		if err != nil {
			return err
		}
		if err := repo.series.RequireLeader(ctx, series.RaidLeaderID); err != nil {
			return err
		}
		scheduled, err := raidseries.NextOccurrence(series, *rd.ScheduledStart, now)
		if err != nil {
			return err
		}
		next, err = raids.FindOccurrence(ctx, series.ID, scheduled)
		if err != nil {
			return fmt.Errorf("find successor occurrence: %w", err)
		}
		if next == nil {
			next = &raid.Raid{Name: series.Name, RaidLeaderID: series.RaidLeaderID, SeriesID: rd.SeriesID, PreviousRaidID: &rd.ID, Status: validation.RaidStatusPlanned, ScheduledStart: &scheduled, CreatedAt: now, UpdatedAt: now}
			if err := raids.Create(ctx, next); err != nil {
				return fmt.Errorf("create successor: %w", err)
			}
		} else {
			if next.ArchivedAt != nil || (next.Status != validation.RaidStatusPlanned && next.Status != validation.RaidStatusOpen) || (next.PreviousRaidID != nil && *next.PreviousRaidID != rd.ID) {
				return raid.ErrOccurrenceConflict
			}
			if next.PreviousRaidID == nil {
				if err := raids.LinkSuccessor(ctx, next.ID, rd.ID); err != nil {
					return fmt.Errorf("link successor: %w", err)
				}
				next.PreviousRaidID = &rd.ID
			}
		}
		rd.Status = validation.RaidStatusCompleted
		rd.CompletedAt = &now
		rd.ArchivedAt = &now
		rd.UpdatedAt = now
		if err := raids.Update(ctx, rd); err != nil {
			return fmt.Errorf("complete raid %d: %w", raidID, err)
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
	if err != nil {
		return nil, nil, err
	}
	return archived, next, nil
}
