package legacyimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/validation"
)

// SetupSyncReport describes an additive synchronization of the current raid roster.
// A missing or changed source row never deletes or silently rewrites website data.
type SetupSyncReport struct {
	RaidID    int64    `json:"raid_id"`
	RaidName  string   `json:"raid_name,omitempty"`
	Added     int      `json:"added"`
	Warnings  []string `json:"warnings"`
	Conflicts []string `json:"conflicts"`
}

// SyncCurrentSetup reads the live ICC25 Raidsetup tab and adds known characters
// to the single active raid of the imported series. On a new successor it first
// requires a previously synchronized archived roster and a changed source block,
// so an old setup cannot be silently copied into the next raid. Conflicting edits
// are reported before changes are committed; archived raids are never modified.
func (s *Service) SyncCurrentSetup(ctx context.Context, actorID int64, source, sheetID, expectedHash string) (SetupSyncReport, error) {
	report := SetupSyncReport{Warnings: []string{}, Conflicts: []string{}}
	data, err := s.fetcher.Fetch(ctx, sheetID)
	if err != nil {
		return report, err
	}
	if data.Hash != expectedHash {
		report.Conflicts = append(report.Conflicts, "Google Sheet wurde während des laufenden Abgleichs verändert; nächster Lauf erforderlich")
		return report, nil
	}
	plan := Plan{}
	entries, _, err := parseSetup(data.Setup, &plan)
	if err != nil {
		return report, err
	}
	report.Warnings = append(report.Warnings, plan.Warnings...)
	report.Conflicts = append(report.Conflicts, plan.Errors...)
	if len(report.Conflicts) > 0 {
		return report, nil
	}

	err = s.transaction(ctx, true, func(bound *Service) error {
		mapping, err := bound.imports.setupMapping(ctx, source, sheetID)
		if errors.Is(err, sql.ErrNoRows) {
			report.Warnings = append(report.Warnings, "Raidsetup noch nicht erstimportiert; zuerst die bestehende Import-Vorschau mit Raidsetup verwenden")
			return nil
		}
		if err != nil {
			return err
		}
		raids, err := bound.raids.ListBySeries(ctx, mapping.SeriesID)
		if err != nil {
			return err
		}
		var active []int64
		for _, rd := range raids {
			if rd.ArchivedAt == nil && (rd.Status == validation.RaidStatusPlanned || rd.Status == validation.RaidStatusOpen) {
				active = append(active, rd.ID)
			}
		}
		if len(active) == 0 {
			report.Warnings = append(report.Warnings, "Kein aktiver Raid in der importierten Serie; Raidsetup wird nicht verändert")
			return nil
		}
		if len(active) != 1 {
			report.Conflicts = append(report.Conflicts, fmt.Sprintf("%d aktive Raids in der importierten Serie: Zuordnung des Sheets nicht eindeutig", len(active)))
			return nil
		}
		report.RaidID = active[0]
		targetRaid, err := bound.raids.Get(ctx, report.RaidID)
		if err != nil {
			return err
		}
		report.RaidName = targetRaid.Name
		previous, previousErr := bound.imports.setupBaseline(ctx, source, sheetID, report.RaidID)
		if previousErr != nil && !errors.Is(previousErr, sql.ErrNoRows) {
			return previousErr
		}
		// A new successor may only receive a changed main roster block.
		// Notes/secondary tables and subsequent edits to archived raids do not
		// prove that the sheet belongs to the next occurrence.
		if report.RaidID != mapping.RaidID && errors.Is(previousErr, sql.ErrNoRows) {
			imported, err := bound.raids.Get(ctx, mapping.RaidID)
			if err != nil {
				return err
			}
			if imported.ArchivedAt == nil || imported.Status != validation.RaidStatusCompleted ||
				imported.ScheduledStart == nil || targetRaid.ScheduledStart == nil ||
				!targetRaid.ScheduledStart.After(*imported.ScheduledStart) {
				report.Conflicts = append(report.Conflicts, "ICC25 Raidsetup: bisheriger Import-Raid nicht abgeschlossen oder nächster Termin nicht eindeutig nachgewiesen")
				return nil
			}
			last, err := bound.imports.latestArchivedSetupBaseline(ctx, source, sheetID, mapping.SeriesID, *targetRaid.ScheduledStart)
			if errors.Is(err, sql.ErrNoRows) {
				report.Warnings = append(report.Warnings, "ICC25 Raidsetup: keine belegte frühere Aufstellung gespeichert; neue Raid-Zuordnung ohne Vergleich nicht automatisch möglich")
				return nil
			}
			if err != nil {
				return err
			}
			previousPlan := Plan{}
			lastEntries, _, err := parseSetup([]byte(last), &previousPlan)
			if err != nil {
				return err
			}
			if len(previousPlan.Errors) > 0 {
				report.Conflicts = append(report.Conflicts, "ICC25 Raidsetup: gespeicherte frühere Aufstellung ist ungültig")
				return nil
			}
			if reflect.DeepEqual(lastEntries, entries) {
				report.Warnings = append(report.Warnings, "ICC25 Raidsetup: Aufstellung identisch zum letzten archivierten Raid; keine automatische Übernahme in "+targetRaid.Name+" ohne belegten Wechsel")
				return nil
			}
		}
		if previousErr == nil {
			oldPlan := Plan{}
			oldEntries, _, parseErr := parseSetup([]byte(previous), &oldPlan)
			if parseErr != nil {
				return parseErr
			}
			currentSource := map[string]bool{}
			for _, entry := range entries {
				currentSource[entry.Character] = true
			}
			for _, entry := range oldEntries {
				if !currentSource[entry.Character] {
					report.Conflicts = append(report.Conflicts, fmt.Sprintf("%s: synchronisierter Sheet-Teilnehmer entfernt; keine automatische Löschung", entry.Character))
				}
			}
		}
		current, err := bound.att.ListByRaid(ctx, report.RaidID)
		if err != nil {
			return err
		}
		byCharacter := make(map[int64]attendance.Attendance, len(current))
		byPlayer := make(map[int64]int64, len(current))
		for _, row := range current {
			byCharacter[row.CharacterID] = row
			byPlayer[row.PlayerID] = row.CharacterID
		}
		pending := make([]attendance.Attendance, 0)
		seen := make(map[int64]bool)
		seenPlayers := make(map[int64]bool)
		for _, entry := range entries {
			person, err := bound.players.FindByName(ctx, entry.Player)
			if errors.Is(err, sql.ErrNoRows) {
				report.Conflicts = append(report.Conflicts, fmt.Sprintf("%s: Spieler %q unbekannt", entry.Character, entry.Player))
				continue
			}
			if err != nil {
				return err
			}
			characters, err := bound.imports.charactersByName(ctx, entry.Character)
			if err != nil {
				return err
			}
			var characterID int64
			for _, ch := range characters {
				if ch.PlayerID == person.ID {
					if characterID != 0 && characterID != ch.ID {
						characterID = 0
						break
					}
					characterID = ch.ID
				}
			}
			if characterID == 0 {
				report.Conflicts = append(report.Conflicts, fmt.Sprintf("%s: Charakter nicht eindeutig Spieler %q zugeordnet", entry.Character, entry.Player))
				continue
			}
			if seen[characterID] {
				report.Conflicts = append(report.Conflicts, fmt.Sprintf("%s: Charakter mehrfach im Sheet", entry.Character))
				continue
			}
			seen[characterID] = true
			if seenPlayers[person.ID] {
				report.Conflicts = append(report.Conflicts, fmt.Sprintf("%s: Spieler mehrfach mit unterschiedlichen Charakteren im Sheet", entry.Character))
				continue
			}
			seenPlayers[person.ID] = true
			if existing, ok := byCharacter[characterID]; ok {
				if existing.PlayerID != person.ID || existing.CharacterName != entry.Character ||
					existing.Class != entry.Class || existing.Spec != entry.Spec || existing.Role != entry.Role {
					report.Conflicts = append(report.Conflicts, fmt.Sprintf("%s: Website-Aufstellung weicht vom Google Sheet ab; keine automatische Überschreibung", entry.Character))
				}
				continue
			}
			if former, ok := byPlayer[person.ID]; ok && former != characterID {
				report.Conflicts = append(report.Conflicts, fmt.Sprintf("%s: Spieler hat bereits einen anderen Charakter im Raid; kein automatischer Austausch", entry.Character))
				continue
			}
			pending = append(pending, attendance.Attendance{
				RaidID: report.RaidID, PlayerID: person.ID, CharacterID: characterID,
				CharacterName: entry.Character, Class: entry.Class, Spec: entry.Spec, Role: entry.Role,
			})
		}
		if len(current)+len(pending) > 25 {
			report.Conflicts = append(report.Conflicts, "ICC25 Raidsetup: mehr als 25 Website-Teilnahmen nach Ergänzung; kein automatisches Entfernen")
		}
		if len(report.Conflicts) > 0 {
			return nil
		}
		if _, err := bound.players.Get(ctx, actorID); err != nil {
			return err
		}
		now := time.Now().UTC()
		for i := range pending {
			pending[i].CreatedAt, pending[i].UpdatedAt = now, now
			if err := bound.att.Create(ctx, &pending[i]); err != nil {
				return err
			}
			actor := actorID
			after, err := json.Marshal(&pending[i])
			if err != nil {
				return err
			}
			if err := bound.imports.writeAudit(ctx, &audit.Log{
				ActorID: &actor, EntityType: "attendance", EntityID: fmt.Sprint(pending[i].ID),
				Action: "create", BeforeJSON: "null", AfterJSON: string(after),
				Reason:    "Automatischer Google-Sheet-Abgleich (nur neue Raidsetup-Einträge)",
				CreatedAt: now,
			}); err != nil {
				return err
			}
			report.Added++
		}
		if len(pending) == 0 && len(entries) > 0 {
			report.Warnings = append(report.Warnings, "Raidsetup unverändert; Website-exklusive Teilnehmer bleiben erhalten")
		}
		return bound.imports.saveSetupBaseline(ctx, source, sheetID, report.RaidID, data.Setup)
	})
	if err != nil {
		return SetupSyncReport{}, fmt.Errorf("sync current setup: %w", err)
	}
	for i := range report.Conflicts {
		report.Conflicts[i] = strings.TrimSpace(report.Conflicts[i])
	}
	return report, nil
}
