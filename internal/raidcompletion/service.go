package raidcompletion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
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
	ErrInvalidExclusion  = errors.New("only rostered players who declined can be excluded from points")
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
	return s.ArchiveRaidWithout(ctx, raidID, actorID, nil)
}

// ArchiveRaidWithout archives like ArchiveRaid but awards no participation points
// to the listed players. Each must be on the roster and have declined the raid;
// the decision is written to the audit log.
func (s *Service) ArchiveRaidWithout(ctx context.Context, raidID, actorID int64, withoutPoints []int64) (*raid.Raid, *raid.Raid, error) {
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
		before := *rd
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
				if err := raids.Audit(ctx, actorID, rd.ID, "archive", &before, rd); err != nil {
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
		// A cancelled future raid (for example a holiday week planned ahead) is
		// skipped; the next occurrence in the same rhythm becomes the successor (ADR 078).
		for skipped := 0; next != nil && next.Status == validation.RaidStatusCancelled && next.ArchivedAt == nil; skipped++ {
			if skipped >= 7*raidseries.MaxPlanWeeks {
				return raid.ErrOccurrenceConflict
			}
			scheduled, err = raidseries.NextOccurrence(series, scheduled, now)
			if err != nil {
				return err
			}
			next, err = raids.FindOccurrence(ctx, series.ID, scheduled)
			if err != nil {
				return fmt.Errorf("find successor occurrence: %w", err)
			}
		}
		if next == nil {
			if err := raids.Catalog().ValidateAssociation(ctx, series.CatalogInstanceID, series.RaidSize); err != nil {
				return err
			}
			successorName := series.Name
			if series.NameTemplate != "" {
				successorName = raidseries.EvaluateName(series.NameTemplate, series.NextRaidNumber, series.Name, scheduled)
				series.NextRaidNumber++
			}
			next = &raid.Raid{Name: successorName, RaidLeaderID: series.RaidLeaderID, SeriesID: rd.SeriesID, PreviousRaidID: &rd.ID, Status: validation.RaidStatusPlanned, ScheduledStart: &scheduled, CreatedAt: now, UpdatedAt: now}
			next.CatalogInstanceID, next.RaidSize = series.CatalogInstanceID, series.RaidSize
			if err := raids.Create(ctx, next); err != nil {
				return fmt.Errorf("create successor: %w", err)
			}
			if err := raids.Audit(ctx, actorID, next.ID, "create_successor", nil, next); err != nil {
				return err
			}
			if err := raids.OpenSignups(ctx, next.ID); err != nil {
				return fmt.Errorf("open successor signups: %w", err)
			}
			if series.NameTemplate != "" {
				if err := repo.series.Save(ctx, series); err != nil {
					return fmt.Errorf("increment series counter: %w", err)
				}
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
		if err := raids.Audit(ctx, actorID, rd.ID, "archive", &before, rd); err != nil {
			return err
		}
		if _, err := raid.PlanAheadInTransaction(ctx, raids, series.ID, actorID, now); err != nil {
			return fmt.Errorf("plan series ahead: %w", err)
		}

		skip := make(map[int64]bool, len(withoutPoints))
		if len(withoutPoints) > 0 {
			declined, err := repo.declinedPlayers(ctx, raidID)
			if err != nil {
				return fmt.Errorf("load absences for raid %d: %w", raidID, err)
			}
			rostered := make(map[int64]bool, len(rows))
			for _, a := range rows {
				rostered[a.PlayerID] = true
			}
			for _, id := range withoutPoints {
				if !declined[id] || !rostered[id] {
					return ErrInvalidExclusion
				}
				skip[id] = true
			}
			ids := make([]int64, 0, len(skip))
			for id := range skip {
				ids = append(ids, id)
			}
			slices.Sort(ids)
			after, err := json.Marshal(map[string]any{"without_points": ids})
			if err != nil {
				return err
			}
			if err := repo.audit.Insert(ctx, &audit.Log{ActorID: &actorID, EntityType: "raid", EntityID: strconv.FormatInt(raidID, 10), Action: "archive_without_points", BeforeJSON: "null", AfterJSON: string(after), CreatedAt: now}); err != nil {
				return err
			}
		}

		awardedPlayers := make(map[int64]struct{}, len(rows))
		for _, a := range rows {
			if _, done := awardedPlayers[a.PlayerID]; done || skip[a.PlayerID] {
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

// Participation reasons reported by PreviewArchive.
const (
	PreviewAward          = "award"
	PreviewMainSpecLoot   = "main_spec_loot"
	PreviewAlreadyAwarded = "already_awarded"
)

// PreviewRow tells what archiving would do for one roster player.
type PreviewRow struct {
	PlayerID int64  `json:"player_id"`
	Balance  int    `json:"balance"`
	Award    int    `json:"award"`
	Reason   string `json:"reason"`
}

// PreviewArchive reports, without changing anything, who would receive the
// participation points when the raid is archived and each player's balance.
func (s *Service) PreviewArchive(ctx context.Context, raidID int64) ([]PreviewRow, error) {
	rd, err := s.repo.raids.Get(ctx, raidID)
	if err != nil {
		return nil, fmt.Errorf("load raid %d: %w", raidID, err)
	}
	if rd.Status != validation.RaidStatusPlanned && rd.Status != validation.RaidStatusOpen {
		return nil, ErrRaidNotArchivable
	}
	rows, err := s.repo.attendance.ListByRaid(ctx, raidID)
	if err != nil {
		return nil, err
	}
	out := []PreviewRow{}
	seen := map[int64]bool{}
	for _, a := range rows {
		if seen[a.PlayerID] {
			continue
		}
		seen[a.PlayerID] = true
		row := PreviewRow{PlayerID: a.PlayerID, Reason: PreviewAward, Award: noMainSpecAward}
		if row.Balance, err = s.repo.points.Balance(ctx, a.PlayerID); err != nil {
			return nil, err
		}
		hasMain, err := s.repo.loot.PlayerHasMainSpecAward(ctx, raidID, a.PlayerID)
		if err != nil {
			return nil, err
		}
		if hasMain {
			row.Award, row.Reason = 0, PreviewMainSpecLoot
		} else if done, err := s.repo.points.HasNoMainSpecAward(ctx, raidID, a.PlayerID); err != nil {
			return nil, err
		} else if done {
			row.Award, row.Reason = 0, PreviewAlreadyAwarded
		}
		out = append(out, row)
	}
	return out, nil
}
