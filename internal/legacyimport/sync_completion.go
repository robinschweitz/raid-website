package legacyimport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/raid"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/validation"
)

type syncFinalization struct {
	raidID  int64
	raidKey string
	nextKey string
	entries []AttendancePlan
}

func syncSuccessorKey(key string) (string, error) {
	cut := strings.LastIndex(key, "-")
	if cut < 1 || cut == len(key)-1 {
		return "", fmt.Errorf("sync raid key %q has no numeric successor", key)
	}
	number, err := strconv.Atoi(key[cut+1:])
	if err != nil || number < 1 {
		return "", fmt.Errorf("sync raid key %q has no numeric successor", key)
	}
	return key[:cut+1] + fmt.Sprintf("%0*d", len(key[cut+1:]), number+1), nil
}

func (s *Service) syncFinalizations(ctx context.Context, source, sheetID string, p Plan, setup []byte, rep *Report) ([]syncFinalization, error) {
	finalizations := []syncFinalization{}
	for _, planned := range p.Raids {
		if planned.ExistingID == 0 {
			continue
		}
		rd, err := s.raids.Get(ctx, planned.ExistingID)
		if err != nil {
			return nil, err
		}
		if rd.Status != validation.RaidStatusPlanned && rd.Status != validation.RaidStatusOpen {
			continue
		}
		if rd.SeriesID == nil || rd.ScheduledStart == nil {
			rep.Errors = append(rep.Errors, "sync_raid_schedule_required: "+planned.Key)
			continue
		}
		if _, err := s.series.Get(ctx, *rd.SeriesID); err != nil {
			return nil, err
		}
		nextKey, err := syncSuccessorKey(planned.Key)
		if err != nil {
			rep.Errors = append(rep.Errors, err.Error())
			continue
		}
		setupPlan := Plan{}
		entries, _, err := parseSetup(setup, &setupPlan)
		if err != nil {
			return nil, fmt.Errorf("parse ICC25 Raidsetup: %w", err)
		}
		rep.Errors = append(rep.Errors, setupPlan.Errors...)
		rep.Warnings = append(rep.Warnings, setupPlan.Warnings...)
		for _, entry := range entries {
			if _, err := s.imports.syncPlayerID(ctx, source, sheetID, entry.Player); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					rep.Errors = append(rep.Errors, "sync_unmapped_player: "+entry.Player)
					continue
				}
				return nil, err
			}
			if _, err := s.imports.syncCharacterID(ctx, source, sheetID, entry.Character, entry.Player); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					rep.Errors = append(rep.Errors, "sync_unmapped_character: "+entry.Character)
					continue
				}
				return nil, err
			}
		}
		if mapped, err := s.imports.syncRaidMapping(ctx, source, sheetID, nextKey); err == nil && mapped.RaidID == rd.ID {
			rep.Errors = append(rep.Errors, "sync_successor_mapping_conflict: "+nextKey)
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		finalizations = append(finalizations, syncFinalization{raidID: rd.ID, raidKey: planned.Key, nextKey: nextKey, entries: entries})
		rep.FinalizedRaids++
		rep.Setup = &SetupReport{Name: nextKey, Attendance: len(entries)}
	}
	if len(rep.Errors) > 0 {
		rep.FinalizedRaids = 0
	}
	return finalizations, nil
}

func (s *Service) applySyncFinalizations(ctx context.Context, actorID int64, source, sheetID, reason string, finalizations []syncFinalization, rep *Report) error {
	if len(finalizations) > 0 && s.completion == nil {
		return fmt.Errorf("sync completion service is not configured")
	}
	for _, planned := range finalizations {
		_, successor, err := s.completion.ArchiveRaid(ctx, planned.raidID, actorID)
		if err != nil {
			return fmt.Errorf("finalize synced raid %q: %w", planned.raidKey, err)
		}
		if successor == nil || successor.Status != validation.RaidStatusPlanned {
			return fmt.Errorf("finalize synced raid %q: planned successor missing", planned.raidKey)
		}
		if err := s.reconcileSyncSetup(ctx, actorID, source, sheetID, reason, successor, planned.nextKey, planned.entries); err != nil {
			return err
		}
		rep.Setup = &SetupReport{Name: planned.nextKey, ScheduledStart: dereferenceTime(successor.ScheduledStart), Attendance: len(planned.entries), AlreadyImported: false}
	}
	return nil
}

func dereferenceTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func (s *Service) reconcileSyncSetup(ctx context.Context, actorID int64, source, sheetID, reason string, successor *raid.Raid, key string, entries []AttendancePlan) error {
	corrections := raidcorrection.NewService(raidcorrection.NewRepo(s.imports.db))
	existing, err := s.att.ListByRaid(ctx, successor.ID)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		playerID, err := s.imports.syncPlayerID(ctx, source, sheetID, entry.Player)
		if err != nil {
			return err
		}
		characterID, err := s.imports.syncCharacterID(ctx, source, sheetID, entry.Character, entry.Player)
		if err != nil {
			return err
		}
		var stored *attendance.Attendance
		for i := range existing {
			if existing[i].PlayerID == playerID && existing[i].CharacterID == characterID {
				stored = &existing[i]
				break
			}
		}
		row := &attendance.Attendance{RaidID: successor.ID, PlayerID: playerID, CharacterID: characterID, CharacterName: entry.Character, Class: entry.Class, Spec: entry.Spec, Role: entry.Role}
		if stored == nil {
			if err := corrections.SaveAttendance(ctx, row, actorID, reason); err != nil {
				return fmt.Errorf("add synced setup attendance %q: %w", entry.Character, err)
			}
			continue
		}
		if stored.CharacterName == row.CharacterName && stored.Class == row.Class && stored.Spec == row.Spec && stored.Role == row.Role {
			continue
		}
		if _, err := corrections.CorrectAttendance(ctx, successor.ID, stored.ID, actorID, reason, func(current *attendance.Attendance) {
			current.CharacterName, current.Class, current.Spec, current.Role = row.CharacterName, row.Class, row.Spec, row.Role
		}); err != nil {
			return fmt.Errorf("correct synced setup attendance %q: %w", entry.Character, err)
		}
	}
	return s.imports.ensureSyncRaidMapping(ctx, source, sheetID, key, successor.ID, actorID, time.Now().UTC())
}
