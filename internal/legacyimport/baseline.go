package legacyimport

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"

	"raid-website/internal/validation"
)

// sourceConflicts compares source versions, independently of later website edits.
// A missing legacy baseline is never inferred from a current balance or a name.
func (s *Service) sourceConflicts(ctx context.Context, source, sheetID string, data SheetData) ([]string, error) {
	old, err := s.imports.sourceBaseline(ctx, source, sheetID)
	if errors.Is(err, sql.ErrNoRows) {
		imports, err := s.imports.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, imp := range imports {
			if imp.Source == source && (imp.SourceURL == sheetID || source == "google_sheets") && imp.Status == "MERGED" {
				return []string{"source_baseline_required: Alter Import ohne Quellenbaseline; Altbestand vor Sync-Aktivierung ausdrücklich abgleichen"}, nil
			}
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	conflicts := []string{}
	oldChars, err := ParseCharaktere(bytes.NewReader(old.Character))
	if err != nil {
		return nil, err
	}
	newChars, err := ParseCharaktere(bytes.NewReader(data.Character))
	if err != nil {
		return nil, err
	}
	chars := map[CharacterRow]int{}
	for _, row := range newChars {
		chars[row]++
	}
	for _, row := range oldChars {
		if row.Character == "" {
			continue
		}
		if chars[row] == 0 {
			conflicts = append(conflicts, fmt.Sprintf("character source conflict: %q geändert oder entfernt; keine automatische Überschreibung", row.Character))
		} else {
			chars[row]--
		}
	}
	oldPlan, _, err := planFromSheet(old)
	if err != nil {
		return nil, err
	}
	newPlan, _, err := planFromSheet(data)
	if err != nil {
		return nil, err
	}
	points := map[string]PlayerPlan{}
	for _, row := range newPlan.Players {
		points[row.Name] = row
	}
	for _, row := range oldPlan.Players {
		_, archivePoints := PlayerPlan{}, false
		if s.sheetArchives != nil {
			_, archivePoints = s.sheetArchives.points[row.Name]
		}
		if !row.HasPoints {
			continue
		}
		if archivePoints {
			if current, ok := points[row.Name]; !ok || current.Active != row.Active {
				conflicts = append(conflicts, fmt.Sprintf("points source conflict: %q Aktivstatus geändert", row.Name))
			}
			continue
		}
		current, ok := points[row.Name]
		if !ok || !current.HasPoints || current.Points != row.Points || current.Active != row.Active {
			conflicts = append(conflicts, fmt.Sprintf("points source conflict: %q geändert oder entfernt; EP-Historie bleibt unverändert", row.Name))
		}
	}
	oldPlayers := map[string]PlayerPlan{}
	for _, row := range oldPlan.Players {
		oldPlayers[row.Name] = row
	}
	for _, row := range newPlan.Players {
		if s.sheetArchives != nil {
			if _, pending := s.sheetArchives.points[row.Name]; pending {
				continue
			}
		}
		if previous, exists := oldPlayers[row.Name]; exists && !previous.HasPoints && row.HasPoints && row.Points != 0 {
			conflicts = append(conflicts, fmt.Sprintf("points source conflict: %q hatte keinen belegten Eröffnungsstand; kein nachträglicher Saldenimport", row.Name))
		}
	}
	oldRaids, err := ParseRaidArchive(bytes.NewReader(old.Raid))
	if err != nil {
		return nil, err
	}
	newRaids, err := ParseRaidArchive(bytes.NewReader(data.Raid))
	if err != nil {
		return nil, err
	}
	raids := map[RaidArchiveRow]int{}
	for _, row := range newRaids {
		raids[row]++
	}
	for _, row := range oldRaids {
		if row.RaidID == "" {
			continue
		}
		if raids[row] == 0 {
			conflicts = append(conflicts, fmt.Sprintf("raid source conflict: %s / %s geändert oder entfernt", row.RaidID, row.Character))
		} else {
			raids[row]--
		}
	}
	oldLoot, err := ParseLootArchive(bytes.NewReader(old.Loot))
	if err != nil {
		return nil, err
	}
	newLoot, err := ParseLootArchive(bytes.NewReader(data.Loot))
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, row := range newLoot {
		ids[row.ImportID] = true
	}
	for _, row := range oldLoot {
		if row.ImportID != "" && !ids[row.ImportID] {
			conflicts = append(conflicts, fmt.Sprintf("loot source conflict: Import ID %q entfernt", row.ImportID))
		}
	}
	archiveSetup := false
	if s.sheetArchives != nil {
		for _, rp := range s.sheetArchives.raids {
			mapping, err := s.imports.setupMapping(ctx, source, sheetID)
			if err == nil && rp.ExistingID == mapping.RaidID {
				archiveSetup = true
			} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}
	}
	// ICC25 Raidsetup is the live roster of the next occurrence once the
	// imported predecessor is archived. It may change on this first handoff
	// (including the run that creates the next raid). After the target has
	// a source baseline, changes to established participation still conflict.
	if !archiveSetup {
		pending, err := s.pendingCurrentSetupHandoff(ctx, source, sheetID)
		if err != nil {
			return nil, err
		}
		archiveSetup = pending
	}
	if len(old.Setup) > 0 && !archiveSetup {
		oldSetupPlan, newSetupPlan := Plan{}, Plan{}
		before, _, err := parseSetup(old.Setup, &oldSetupPlan)
		if err != nil {
			return nil, err
		}
		after, _, err := parseSetup(data.Setup, &newSetupPlan)
		if err != nil {
			return nil, err
		}
		setup := map[string]AttendancePlan{}
		for _, row := range after {
			setup[row.Character] = row
		}
		for _, row := range before {
			current, ok := setup[row.Character]
			row.Position = 0
			current.Position = 0
			if !ok || !reflect.DeepEqual(row, current) {
				conflicts = append(conflicts, fmt.Sprintf("raidsetup source conflict: %q geändert oder entfernt", row.Character))
			}
		}
		conflicts = append(conflicts, newSetupPlan.Errors...)
	}
	return conflicts, nil
}

// pendingCurrentSetupHandoff lifts old source immutability only when
// the imported predecessor has been archived and the next ICC25 occurrence
// has not yet received its own setup baseline. It performs no writes.
func (s *Service) pendingCurrentSetupHandoff(ctx context.Context, source, sheetID string) (bool, error) {
	mapping, err := s.imports.setupMapping(ctx, source, sheetID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	imported, err := s.raids.Get(ctx, mapping.RaidID)
	if err != nil {
		return false, err
	}
	if imported.ArchivedAt == nil || imported.Status != validation.RaidStatusCompleted || imported.ScheduledStart == nil {
		return false, nil
	}
	seriesRaids, err := s.raids.ListBySeries(ctx, mapping.SeriesID)
	if err != nil {
		return false, err
	}
	var targetID int64
	for _, rd := range seriesRaids {
		if rd.ArchivedAt != nil || (rd.Status != validation.RaidStatusPlanned && rd.Status != validation.RaidStatusOpen) {
			continue
		}
		if targetID != 0 || rd.ScheduledStart == nil || !rd.ScheduledStart.After(*imported.ScheduledStart) {
			return false, nil
		}
		targetID = rd.ID
	}
	if targetID == 0 {
		series, err := s.series.Get(ctx, mapping.SeriesID)
		if err != nil {
			return false, err
		}
		// An absent future raid is now creatable from the proven ICC25
		// archive plus the explicitly configured recurrence.
		return series.Name == "ICC25" && series.NameTemplate == "ICC25-{number}", nil
	}
	if targetID == mapping.RaidID {
		return false, nil
	}
	_, err = s.imports.setupBaseline(ctx, source, sheetID, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return false, err
}
