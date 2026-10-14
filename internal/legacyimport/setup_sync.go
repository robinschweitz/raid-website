package legacyimport

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "strings"
    "time"

    "raid-website/internal/attendance"
    "raid-website/internal/audit"
    "raid-website/internal/validation"
)

// SetupSyncReport describes an additive synchronization of the current raid roster.
// A missing or changed source row never deletes or silently rewrites website data.
type SetupSyncReport struct {
    RaidID int64 `json:"raid_id"`
    Added int `json:"added"`
    Warnings []string `json:"warnings"`
    Conflicts []string `json:"conflicts"`
}

// SyncCurrentSetup reads the original ICC25 Raidsetup tab and adds known characters
// to the single active raid of the imported series. Conflicting edits are reported
// before any change is committed. Archived raids and unknown players are never
// modified automatically.
func (s *Service) SyncCurrentSetup(ctx context.Context, actorID int64, source, sheetID string) (SetupSyncReport, error) {
    var report SetupSyncReport
    data, err := s.fetcher.Fetch(ctx, sheetID)
    if err != nil { return report, err }
    plan := Plan{}
    entries, _, err := parseSetup(data.Setup, &plan)
    if err != nil { return report, err }
    report.Warnings = append(report.Warnings, plan.Warnings...)
    report.Conflicts = append(report.Conflicts, plan.Errors...)
    if len(report.Conflicts) > 0 { return report, nil }

    err = s.transaction(ctx, true, func(bound *Service) error {
        mapping, err := bound.imports.setupMapping(ctx, source, sheetID)
        if errors.Is(err, sql.ErrNoRows) {
            report.Warnings = append(report.Warnings, "Raidsetup noch nicht erstimportiert; zuerst die bestehende Import-Vorschau mit Raidsetup verwenden")
            return nil
        }
        if err != nil { return err }
        raids, err := bound.raids.ListBySeries(ctx, mapping.SeriesID)
        if err != nil { return err }
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
        current, err := bound.att.ListByRaid(ctx, report.RaidID)
        if err != nil { return err }
        byCharacter := make(map[int64]attendance.Attendance, len(current))
        for _, row := range current { byCharacter[row.CharacterID] = row }
        pending := make([]attendance.Attendance, 0)
        seen := make(map[int64]bool)
        for _, entry := range entries {
            person, err := bound.players.FindByName(ctx, entry.Player)
            if errors.Is(err, sql.ErrNoRows) {
                report.Conflicts = append(report.Conflicts, fmt.Sprintf("%s: Spieler %q unbekannt", entry.Character, entry.Player))
                continue
            }
            if err != nil { return err }
            characters, err := bound.imports.charactersByName(ctx, entry.Character)
            if err != nil { return err }
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
            if existing, ok := byCharacter[characterID]; ok {
                if existing.PlayerID != person.ID || existing.CharacterName != entry.Character ||
                    existing.Class != entry.Class || existing.Spec != entry.Spec || existing.Role != entry.Role {
                    report.Conflicts = append(report.Conflicts, fmt.Sprintf("%s: Website-Aufstellung weicht vom Google Sheet ab; keine automatische Überschreibung", entry.Character))
                }
                continue
            }
            pending = append(pending, attendance.Attendance{
                RaidID: report.RaidID, PlayerID: person.ID, CharacterID: characterID,
                CharacterName: entry.Character, Class: entry.Class, Spec: entry.Spec, Role: entry.Role,
            })
        }
        if len(report.Conflicts) > 0 { return nil }
        if _, err := bound.players.Get(ctx, actorID); err != nil { return err }
        now := time.Now().UTC()
        for i := range pending {
            pending[i].CreatedAt, pending[i].UpdatedAt = now, now
            if err := bound.att.Create(ctx, &pending[i]); err != nil { return err }
            actor := actorID
            if err := bound.imports.writeAudit(ctx, &audit.Log{
                ActorID: &actor, EntityType: "attendance", EntityID: fmt.Sprint(pending[i].ID),
                Action: "create", BeforeJSON: "null", AfterJSON: fmt.Sprintf(
                    "{\"raid_id\":%d,\"character_id\":%d}", report.RaidID, pending[i].CharacterID),
                Reason: "Automatischer Google-Sheet-Abgleich (nur neue Raidsetup-Einträge)",
                CreatedAt: now,
            }); err != nil { return err }
            report.Added++
        }
        if len(pending) == 0 && len(entries) > 0 {
            report.Warnings = append(report.Warnings, "Raidsetup unverändert; Website-exklusive Teilnehmer bleiben erhalten")
        }
        return nil
    })
    if err != nil { return SetupSyncReport{}, fmt.Errorf("sync current setup: %w", err) }
    for i := range report.Conflicts { report.Conflicts[i] = strings.TrimSpace(report.Conflicts[i]) }
    return report, nil
}
