package legacyimport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

const authorizedSyncCopy = "10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8"
const originalImportSheet = "1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU"

// ErrArchiveActorForbidden rejects inactive or non-administrator sync actors.
var ErrArchiveActorForbidden = errors.New("sheet sync actor must be an active administrator")

type sheetArchivePlan struct {
	raids    map[string]RaidPlan
	points   map[string]PlayerPlan
	warnings []string
}

// ArchiveTransfer describes an authoritative Sheet archive adoption. It never
// invokes website completion rewards or invents a successor for imported history.
type ArchiveTransfer struct {
	RaidID     int64     `json:"raid_id"`
	RaidName   string    `json:"raid_name"`
	ArchivedAt time.Time `json:"archived_at"`
}

// PointTransfer reports compensating ledger entries, including zero openings.
type PointTransfer struct {
	Player     string `json:"player"`
	Before     int    `json:"before"`
	Target     int    `json:"target"`
	Adjustment int    `json:"adjustment"`
}

func (s *Service) prepareSheetArchives(ctx context.Context, actorID int64, source, sheetID, reason string, data SheetData) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("reason_required: Sheet-Archivübernahme braucht eine Begründung")
	}
	if source != "google_sheets" || sheetID != authorizedSyncCopy {
		return fmt.Errorf("sheet archival requires the authorized working copy")
	}
	actor, err := s.players.Get(ctx, actorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrActorNotFound
		}
		return err
	}
	if actor.Role != validation.RoleAdmin || !actor.Active {
		return ErrArchiveActorForbidden
	}
	p, _, err := planFromSheet(data)
	if err != nil {
		return err
	}
	if len(p.Errors) > 0 {
		return fmt.Errorf("sheet plan conflicts: %s", strings.Join(p.Errors, "; "))
	}
	syncPlan := &sheetArchivePlan{raids: map[string]RaidPlan{}, points: map[string]PlayerPlan{}}
	s.sheetArchives = syncPlan
	old, baselineErr := s.imports.sourceBaseline(ctx, source, sheetID)
	if baselineErr != nil && !errors.Is(baselineErr, sql.ErrNoRows) {
		return baselineErr
	}
	// Existing original-sheet provenance may be copied only when the configured
	// setup's exact raid appears in the authorized copy. Mappings are still fully
	// validated by review/bootstrap below, in the same rollbackable transaction.
	setup, setupErr := s.imports.setupMapping(ctx, source, sheetID)
	if errors.Is(setupErr, sql.ErrNoRows) {
		original, err := s.imports.setupMapping(ctx, source, originalImportSheet)
		if err == nil {
			configured, err := s.raids.Get(ctx, original.RaidID)
			if err != nil {
				return err
			}
			matched := false
			for _, rp := range p.Raids {
				if rp.Key == configured.Name {
					matched = true
				}
			}
			if matched {
				if err = s.imports.copyImportProvenance(ctx, source, originalImportSheet, sheetID); err != nil {
					return err
				}
				setup, setupErr = s.imports.setupMapping(ctx, source, sheetID)
				syncPlan.warnings = append(syncPlan.warnings, "Bestehende Import-Zuordnungen werden geprüft in die erlaubte Sheet-Kopie übernommen; das Original bleibt unverändert.")
				if err = s.imports.writeAudit(ctx, &audit.Log{ActorID: &actorID, EntityType: "sheet_sync", EntityID: sheetID, Action: "copy_import_provenance", BeforeJSON: strconv.Quote(originalImportSheet), AfterJSON: strconv.Quote(sheetID), Reason: reason, CreatedAt: time.Now().UTC()}); err != nil {
					return err
				}
				if originalBaseline, err := s.imports.sourceBaseline(ctx, source, originalImportSheet); err == nil {
					old = originalBaseline
					baselineErr = nil
					if err = s.imports.saveSourceBaseline(ctx, source, sheetID, old); err != nil {
						return err
					}
				} else if !errors.Is(err, sql.ErrNoRows) {
					return err
				}
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if setupErr != nil && !errors.Is(setupErr, sql.ErrNoRows) {
		return setupErr
	}
	oldRaids := map[string]bool{}
	if baselineErr == nil {
		previous, _, err := planFromSheet(old)
		if err != nil {
			return err
		}
		for _, rp := range previous.Raids {
			oldRaids[rp.Key] = true
		}
	}
	archivedRows, err := ParseRaidArchive(bytes.NewReader(data.Raid))
	if err != nil {
		return err
	}
	sourceRaids := map[string]bool{}
	for _, row := range archivedRows {
		if strings.TrimSpace(row.RaidID) != "" {
			sourceRaids[strings.TrimSpace(row.RaidID)] = true
		}
	}
	for _, rp := range p.Raids {
		mapping, err := s.imports.raidMapping(ctx, source, sheetID, rp.Key)
		if errors.Is(err, sql.ErrNoRows) {
			mapping = nil
		}
		if errors.Is(err, sql.ErrNoRows) && setupErr == nil {
			seriesRaids, loadErr := s.raids.ListBySeries(ctx, setup.SeriesID)
			if loadErr != nil {
				return loadErr
			}
			for _, configured := range seriesRaids {
				if configured.Name == rp.Key {
					mapping = &raidMapping{Source: source, SheetID: sheetID, RaidKey: rp.Key, RaidID: configured.ID}
					if err = s.imports.insertRaidMapping(ctx, mapping); err != nil {
						return err
					}
					break
				}
			}
		}
		var stored *raid.Raid
		if err == nil || mapping != nil {
			stored, err = s.raids.Get(ctx, mapping.RaidID)
			if err != nil {
				return err
			}
			rp.ExistingID = stored.ID
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		candidate := stored == nil || stored.ArchivedAt == nil || (baselineErr == nil && !oldRaids[rp.Key])
		if !candidate {
			continue
		}
		if !sourceRaids[rp.Key] {
			return fmt.Errorf("raid %q: Archivstatus ist nicht durch Raid Archive belegt", rp.Key)
		}
		if rp.Date == nil {
			return fmt.Errorf("raid %q: gültiger Sheet-Archivzeitpunkt erforderlich", rp.Key)
		}
		if stored != nil {
			if stored.Status == validation.RaidStatusCancelled || (stored.ArchivedAt != nil && stored.Status != validation.RaidStatusCompleted) {
				return fmt.Errorf("raid %q: abgesagter oder widersprüchlicher Website-Status", rp.Key)
			}
			if stored.Status != validation.RaidStatusPlanned && stored.Status != validation.RaidStatusOpen && stored.Status != validation.RaidStatusCompleted {
				return fmt.Errorf("raid %q: ungültiger Website-Status", rp.Key)
			}
			if err = s.validateArchiveRoster(ctx, stored.ID, rp); err != nil {
				return err
			}
			unmapped, err := s.imports.unmappedLootExists(ctx, source, sheetID, stored.ID)
			if err != nil {
				return err
			}
			if unmapped {
				return fmt.Errorf("raid %q: Website-Loot ohne Sheet-Zuordnung; keine doppelte Übernahme", rp.Key)
			}
		}
		for _, row := range archivedRows {
			if strings.TrimSpace(row.RaidID) != rp.Key {
				continue
			}
			timestamp, ok := parseSheetDate(row.TimestampRaw)
			if !ok || !timestamp.Equal(*rp.Date) {
				return fmt.Errorf("raid %q: widersprüchliche oder fehlende Archivzeitpunkte", rp.Key)
			}
		}
		for _, row := range rp.Entries {
			if stored == nil || stored.Status == validation.RaidStatusCompleted {
				continue
			}
			_, validClass := validation.ClassSpecs[row.Class]
			if row.Character == "" || row.Player == "" || row.Spec == "" || !validClass {
				return fmt.Errorf("raid %q: unvollständige Archivaufstellung für %q", rp.Key, row.Character)
			}
		}
		syncPlan.raids[rp.Key] = rp
	}
	if len(syncPlan.raids) == 0 {
		return nil
	}
	if err = s.validateMappedArchiveLoot(ctx, source, sheetID, p); err != nil {
		return err
	}
	pointPlayers := map[string]bool{}
	for _, rp := range syncPlan.raids {
		for _, row := range rp.Entries {
			pointPlayers[row.Player] = true
		}
	}
	for _, pl := range p.Players {
		if !pointPlayers[pl.Name] {
			continue
		}
		if !pl.HasPoints {
			// With MAIN/ENHANCED loot in every newly archived participation,
			// no completion award is due. An absent balance stays absent and
			// the website ledger is preserved; never infer an opening zero.
			noReward := true
			for key, rp := range syncPlan.raids {
				participates := false
				for _, row := range rp.Entries {
					if row.Player == pl.Name {
						participates = true
					}
				}
				if !participates {
					continue
				}
				hasMain := false
				for _, award := range p.Loot {
					if award.RaidKey == key && award.PlayerName == pl.Name && award.IsMainSpec() {
						hasMain = true
					}
				}
				if !hasMain {
					noReward = false
				}
			}
			if !noReward {
				return fmt.Errorf("player %q: expliziter Sheet-EP-Zielstand für Archivübernahme erforderlich", pl.Name)
			}
			syncPlan.warnings = append(syncPlan.warnings, fmt.Sprintf("%s: kein EP-Zielstand im Sheet; Main-Spec-Loot belegt, vorhandene Website-EP bleiben unverändert.", pl.Name))
			continue
		}
		syncPlan.points[pl.Name] = pl
	}
	for name, pl := range syncPlan.points {
		existing, err := s.players.FindByName(ctx, name)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		balance, err := s.points.Balance(ctx, existing.ID)
		if err != nil {
			return err
		}
		if balance == pl.Points {
			continue
		}
		version, err := s.imports.pointVersion(ctx, existing.ID)
		if err != nil {
			return err
		}
		baseline, err := s.imports.pointBaseline(ctx, source, sheetID, existing.ID)
		if errors.Is(err, sql.ErrNoRows) {
			websiteHistory, err := s.imports.hasWebsitePoints(ctx, existing.ID)
			if err != nil {
				return err
			}
			if websiteHistory {
				return fmt.Errorf("player %q: Website-EP-Historie ohne Abgleichbaseline; Sheet-Zielstand würde Website-Änderungen überschreiben", name)
			}
		} else if err != nil {
			return err
		} else if version != baseline {
			return fmt.Errorf("player %q: EP-Konflikt; Website und Sheet seit letztem Abgleich verändert", name)
		}
	}
	if errors.Is(baselineErr, sql.ErrNoRows) {
		imports, err := s.imports.List(ctx)
		if err != nil {
			return err
		}
		hasHistory := false
		for _, imp := range imports {
			if imp.Source == source && imp.Status == "MERGED" {
				hasHistory = true
			}
		}
		if hasHistory {
			if err = s.bootstrapArchiveBaseline(ctx, source, sheetID, data, p); err != nil {
				return err
			}
			syncPlan.warnings = append(syncPlan.warnings, "Altimport ohne Quellenbaseline: Charaktere, historische Raidaufstellungen und Loot wurden vor Archivübernahme gegen die Datenbank geprüft.")
		}
	}
	return nil
}

func (s *Service) validateArchiveRoster(ctx context.Context, raidID int64, planned RaidPlan) error {
	rows, err := s.att.ListByRaid(ctx, raidID)
	if err != nil {
		return err
	}
	for _, stored := range rows {
		person, err := s.players.Get(ctx, stored.PlayerID)
		if err != nil {
			return err
		}
		matched := false
		for _, row := range planned.Entries {
			if row.Player == person.Name && row.Character == stored.CharacterName && row.Class == stored.Class && row.Spec == stored.Spec && row.Role == stored.Role {
				matched = true
			}
		}
		if !matched {
			return fmt.Errorf("raid %q: Website-Aufstellung für %q widerspricht Sheet-Archiv", planned.Key, stored.CharacterName)
		}
	}
	return nil
}

// bootstrapArchiveBaseline proves the existing historical data instead of
// guessing a missing old snapshot from today's Sheet. Only the pending archive
// may add rows; old historical additions/removals/character edits still conflict.
func (s *Service) bootstrapArchiveBaseline(ctx context.Context, source, sheetID string, data SheetData, p Plan) error {
	mappings, err := s.imports.mappings(ctx, source, sheetID)
	if err != nil {
		return err
	}
	known := map[string]lootMapping{}
	for _, m := range mappings {
		known[m.Identity] = m
	}
	for _, row := range p.Loot {
		if old, ok := known[row.Identity]; ok {
			if old.Fingerprint != row.Fingerprint {
				return fmt.Errorf("legacy loot fingerprint conflict")
			}
			delete(known, row.Identity)
		} else if _, pending := s.sheetArchives.raids[row.RaidKey]; !pending {
			return fmt.Errorf("legacy baseline: unbekannter historischer Loot außerhalb neuer Archivierung")
		}
	}
	if len(known) > 0 {
		return fmt.Errorf("legacy baseline: historische Loot-Zeilen fehlen")
	}
	for _, c := range p.Characters {
		matches, err := s.imports.charactersByName(ctx, c.Name)
		if err != nil {
			return err
		}
		for _, stored := range matches {
			person, err := s.players.Get(ctx, stored.PlayerID)
			if err != nil {
				return err
			}
			if person.Name != c.PlayerName || stored.Class != c.Class || stored.MainSpec != c.MainSpec || stored.SecondSpec != c.SecondSpec {
				return fmt.Errorf("legacy baseline: character %q unterscheidet sich von Website", c.Name)
			}
		}
	}
	raidMappings, err := s.imports.raidMappings(ctx, source, sheetID)
	if err != nil {
		return err
	}
	present := map[string]bool{}
	for _, rp := range p.Raids {
		present[rp.Key] = true
	}
	for _, mapping := range raidMappings {
		if !present[mapping.RaidKey] {
			return fmt.Errorf("legacy baseline: historischer Raid %q fehlt", mapping.RaidKey)
		}
	}
	for _, rp := range p.Raids {
		if _, pending := s.sheetArchives.raids[rp.Key]; pending {
			continue
		}
		mapping, err := s.imports.raidMapping(ctx, source, sheetID, rp.Key)
		if err != nil {
			return fmt.Errorf("legacy baseline: raid %q ohne belegte Zuordnung: %w", rp.Key, err)
		}
		if err = s.validateArchiveRoster(ctx, mapping.RaidID, rp); err != nil {
			return err
		}
		rows, err := s.att.ListByRaid(ctx, mapping.RaidID)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, entry := range rp.Entries {
			seen[entry.Player+"\x00"+entry.Character] = true
		}
		if len(rows) != len(seen) {
			return fmt.Errorf("legacy baseline: historischer Raid %q hat neue Aufstellungszeilen", rp.Key)
		}
	}

	// Old imports without a source baseline may only reconcile participant EP.
	// Unrelated source balances must already match, rather than accepting an
	// invented old snapshot or silently resetting other website accounts.
	for _, pl := range p.Players {
		if _, pending := s.sheetArchives.points[pl.Name]; pending || !pl.HasPoints {
			continue
		}
		person, err := s.players.FindByName(ctx, pl.Name)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		balance, err := s.points.Balance(ctx, person.ID)
		if err != nil {
			return err
		}
		if balance != pl.Points {
			return fmt.Errorf("legacy baseline: EP-Konflikt außerhalb neuer Archivierung für %q", pl.Name)
		}
	}
	return s.imports.saveSourceBaseline(ctx, source, sheetID, data)
}

func (s *Service) describeSheetArchives(ctx context.Context, rep *Report) error {
	if s.sheetArchives == nil {
		return nil
	}
	rep.ArchiveTransfers = nil
	rep.PointTransfers = nil
	rep.Warnings = append(rep.Warnings, s.sheetArchives.warnings...)
	for _, rp := range s.sheetArchives.raids {
		rep.ArchiveTransfers = append(rep.ArchiveTransfers, ArchiveTransfer{RaidID: rp.ExistingID, RaidName: rp.Key, ArchivedAt: *rp.Date})
	}
	for name, pl := range s.sheetArchives.points {
		before := 0
		person, err := s.players.FindByName(ctx, name)
		if err == nil {
			before, err = s.points.Balance(ctx, person.ID)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		rep.PointTransfers = append(rep.PointTransfers, PointTransfer{Player: name, Before: before, Target: pl.Points, Adjustment: pl.Points - before})
	}
	sort.Slice(rep.ArchiveTransfers, func(i, j int) bool { return rep.ArchiveTransfers[i].RaidName < rep.ArchiveTransfers[j].RaidName })
	sort.Slice(rep.PointTransfers, func(i, j int) bool { return rep.PointTransfers[i].Player < rep.PointTransfers[j].Player })
	return nil
}

func (s *Service) applySheetArchives(ctx context.Context, actorID int64, source, sheetID, reason string, rep *Report) error {
	if s.sheetArchives == nil {
		return nil
	}
	now := time.Now().UTC()
	for name, pl := range s.sheetArchives.points {
		person, err := s.players.FindByName(ctx, name)
		if err != nil {
			return err
		}
		balance, err := s.points.Balance(ctx, person.ID)
		if err != nil {
			return err
		}
		hasOpening, err := s.points.HasTransactionType(ctx, person.ID, "legacy_open")
		if err != nil {
			return err
		}
		if !hasOpening {
			if err = s.points.InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: person.ID, Amount: 0, Type: "legacy_open", Reason: "Sheet-Archivabgleich: bestehender Eröffnungsstand erfasst", RefType: "sheet_sync", RefID: sheetID, ActorID: actorID, CreatedAt: now}); err != nil {
				return err
			}
			rep.Created.Points++
		}
		if delta := pl.Points - balance; delta != 0 {
			if err = s.points.InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: person.ID, Amount: delta, Type: "sheet_archive_reconciliation", Reason: reason, RefType: "sheet_sync", RefID: sheetID, ActorID: actorID, CreatedAt: now}); err != nil {
				return err
			}
			rep.Created.Points++
		}
		if err = s.auditSheetChange(ctx, actorID, "enhance_points", strconv.FormatInt(person.ID, 10), balance, pl.Points, reason, now); err != nil {
			return err
		}
	}
	for key, rp := range s.sheetArchives.raids {
		mapping, err := s.imports.raidMapping(ctx, source, sheetID, key)
		if err != nil {
			return err
		}
		stored, err := s.raids.Get(ctx, mapping.RaidID)
		if err != nil {
			return err
		}
		if stored.ArchivedAt != nil {
			continue
		}
		before := *stored
		stored.Status = validation.RaidStatusCompleted
		stored.CompletedAt = rp.Date
		stored.ArchivedAt = rp.Date
		stored.UpdatedAt = now
		if err = s.raids.Update(ctx, stored); err != nil {
			return err
		}
		locks, err := s.points.ListActiveLocksByRaid(ctx, stored.ID)
		if err != nil {
			return err
		}
		for _, lock := range locks {
			lock.Status = validation.LockStatusExpired
			lock.ExpiredAt = rp.Date
			lock.UpdatedAt = now
			if err = s.points.UpdateLock(ctx, &lock); err != nil {
				return err
			}
		}
		if err = s.auditSheetChange(ctx, actorID, "raid", strconv.FormatInt(stored.ID, 10), before, stored, reason, now); err != nil {
			return err
		}
		rep.ArchivedRaids++
	}
	return nil
}

func (s *Service) auditSheetChange(ctx context.Context, actorID int64, entity, id string, before, after any, reason string, now time.Time) error {
	old, err := json.Marshal(before)
	if err != nil {
		return err
	}
	current, err := json.Marshal(after)
	if err != nil {
		return err
	}
	if string(old) == string(current) {
		return nil
	}
	return s.imports.writeAudit(ctx, &audit.Log{ActorID: &actorID, EntityType: entity, EntityID: id, Action: "sheet_archive_reconciliation", BeforeJSON: string(old), AfterJSON: string(current), Reason: reason, CreatedAt: now})
}

func (s *Service) savePointBaselines(ctx context.Context, source, sheetID string, data SheetData) error {
	p, _, err := planFromSheet(data)
	if err != nil {
		return err
	}
	for _, pl := range p.Players {
		person, err := s.players.FindByName(ctx, pl.Name)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		balance, err := s.points.Balance(ctx, person.ID)
		if err != nil {
			return err
		}
		// A source-only sync must not acknowledge website EP that the source
		// has never contained. Keep its older version until values agree.
		if !pl.HasPoints || balance != pl.Points {
			continue
		}
		if err = s.imports.savePointBaseline(ctx, source, sheetID, person.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recordSheetArchiveReport(ctx context.Context, actorID int64, sheetID, reason string, rep Report) error {
	if len(rep.ArchiveTransfers) == 0 {
		return nil
	}
	raw, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.imports.Insert(ctx, &Import{ActorID: actorID, Source: rep.Source, SourceURL: sheetID, RawHash: rep.RawHash, DryRunReport: string(raw), Status: "MERGED", WarningsCount: len(rep.Warnings), CreatedCount: rep.Created.Players + rep.Created.Characters + rep.Created.Raids + rep.Created.Attendance + rep.Created.Loot + rep.Created.Points, UpdatedCount: rep.ArchivedRaids, StartedAt: &now, CompletedAt: &now, Notes: reason})
}

func (s *Service) validateMappedArchiveLoot(ctx context.Context, source, sheetID string, p Plan) error {
	mappings, err := s.imports.mappings(ctx, source, sheetID)
	if err != nil {
		return err
	}
	known := map[string]lootMapping{}
	for _, mapping := range mappings {
		known[mapping.Identity] = mapping
	}
	for _, row := range p.Loot {
		rp, pending := s.sheetArchives.raids[row.RaidKey]
		if !pending {
			continue
		}
		mapping, exists := known[row.Identity]
		if !exists {
			continue
		}
		stored, err := s.loot.Get(ctx, mapping.LootAwardID)
		if err != nil {
			return err
		}
		person, err := s.players.Get(ctx, stored.AwardedToPlayerID)
		if err != nil {
			return err
		}
		sameDate := row.Date == nil || stored.AwardedAt.Equal(*row.Date)
		if stored.RaidID != rp.ExistingID || stored.ItemName != row.Item || stored.ItemID != row.ItemID || stored.AwardedToCharacterName != row.CharacterName || person.Name != row.PlayerName || stored.AwardMethod != row.Method || !sameDate {
			return fmt.Errorf("raid %q: Website-Loot wurde seit Sheet-Import geändert", row.RaidKey)
		}
	}
	return nil
}
