package legacyimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/audit"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

// createUpcomingSetupRaid creates the next scheduled occurrence only when an
// existing, explicitly imported ICC25 series can be matched to a proven archived
// ICC25-N entry in the authorized source. An absent/ambiguous series or historical
// entry is never a reason to invent a schedule. Called inside Reconcile's single
// rollbackable transaction, before applying the live setup roster.
func (s *Service) createUpcomingSetupRaid(ctx context.Context, actorID int64, source, sheetID string, mapping *setupMapping, data SheetData) (*raid.Raid, string, error) {
	if source != "google_sheets" || sheetID != authorizedSyncCopy {
		return nil, "", errors.New("current raid creation requires authorized copy")
	}
	series, err := s.series.Get(ctx, mapping.SeriesID)
	if err != nil {
		return nil, "", err
	}
	// The explicit numeric ICC25 naming rule is necessary to prove ICC25-36
	// from ICC25-35; never guess from unrelated historical rows.
	const prefix = "ICC25-"
	if series.Name != "ICC25" || series.NameTemplate != prefix+"{number}" {
		return nil, "Raidsetup: ICC25-Serienvorlage fehlt oder ist abweichend; keinen Folge-Raid geraten", nil
	}
	if err := raidseries.Validate(series); err != nil {
		return nil, "", fmt.Errorf("raidsetup Serienplan ungültig: %w", err)
	}
	if err := s.series.RequireLeader(ctx, series.RaidLeaderID); err != nil {
		return nil, "", err
	}
	if err := s.raids.Catalog().ValidateAssociation(ctx, series.CatalogInstanceID, series.RaidSize); err != nil {
		return nil, "", err
	}
	if series.RaidSize == nil || *series.RaidSize != 25 || series.CatalogInstanceID == nil || *series.CatalogInstanceID != icecrownCitadelInstanceID {
		return nil, "Raidsetup: Serie ist nicht eindeutig als ICC25 konfiguriert", nil
	}

	archivedRows, err := ParseRaidArchive(strings.NewReader(string(data.Raid)))
	if err != nil {
		return nil, "", err
	}
	lastNumber := 0
	for _, row := range archivedRows {
		name := strings.TrimSpace(row.RaidID)
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		number, numberErr := strconv.Atoi(strings.TrimPrefix(name, prefix))
		if numberErr == nil && number > lastNumber {
			lastNumber = number
		}
	}
	if lastNumber == 0 {
		return nil, "Raidsetup: kein nummerierter ICC25-Raid im freigegebenen Archiv belegt", nil
	}
	lastName := fmt.Sprintf("%s%d", prefix, lastNumber)
	provenance, err := s.imports.raidMapping(ctx, source, sheetID, lastName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "Raidsetup: letzter Sheet-Raid "+lastName+" ohne überprüfte Importzuordnung", nil
	}
	if err != nil {
		return nil, "", err
	}
	lastRaid, err := s.raids.Get(ctx, provenance.RaidID)
	if err != nil {
		return nil, "", err
	}
	if lastRaid.Name != lastName || lastRaid.ArchivedAt == nil || lastRaid.Status != validation.RaidStatusCompleted || lastRaid.ScheduledStart == nil {
		return nil, "Raidsetup: "+lastName+" ist auf der Website noch nicht eindeutig archiviert", nil
	}
	if lastRaid.SeriesID != nil && *lastRaid.SeriesID != series.ID {
		return nil, "", fmt.Errorf("raidsetup: archivierter Raid %s gehört zu einer anderen Serie", lastName)
	}
	if previous, err := s.raids.Successor(ctx, lastRaid.ID); err != nil {
		return nil, "", err
	} else if previous != nil {
		return nil, "", fmt.Errorf("raidsetup: %s hat bereits Nachfolger %s; keine doppelte Planung", lastName, previous.Name)
	}

	// Source archive names are authoritative for the next ordinal. An advanced
	// server counter would mean a skipped/removed occurrence: stop rather than
	// silently creating ICC25-37 when the source proves ICC25-36.
	nextNumber := lastNumber + 1
	if series.NextRaidNumber > nextNumber {
		return nil, "", fmt.Errorf("raidsetup: Serienzähler %d liegt hinter Sheet-Nachfolger %s%d; Planung manuell prüfen", series.NextRaidNumber, prefix, nextNumber)
	}
	nextName := raidseries.EvaluateName(series.NameTemplate, nextNumber, series.Name, time.Time{})
	if found, err := s.raids.FindByName(ctx, nextName); err == nil {
		return nil, "", fmt.Errorf("raidsetup: %s existiert bereits als Raid %d ohne aktive Zuordnung", nextName, found.ID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, "", err
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	scheduled, err := raidseries.NextOccurrence(series, *lastRaid.ScheduledStart, now)
	if err != nil {
		return nil, "", fmt.Errorf("raidsetup: nächster Termin aus Serienplan konnte nicht bestimmt werden: %w", err)
	}
	if occurrence, err := s.raids.FindOccurrence(ctx, series.ID, scheduled); err != nil {
		return nil, "", err
	} else if occurrence != nil {
		return nil, "", fmt.Errorf("raidsetup: Termin bereits als %s vorhanden; automatische Erstellung blockiert", occurrence.Name)
	}
	rd := &raid.Raid{
		Name:              nextName,
		SeriesID:          &series.ID,
		PreviousRaidID:    &lastRaid.ID,
		RaidLeaderID:      series.RaidLeaderID,
		Status:            validation.RaidStatusPlanned,
		CatalogInstanceID: series.CatalogInstanceID,
		RaidSize:          series.RaidSize,
		ScheduledStart:    &scheduled,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.raids.Create(ctx, rd); err != nil {
		return nil, "", fmt.Errorf("raidsetup: %s erstellen: %w", nextName, err)
	}
	series.NextRaidNumber = nextNumber + 1
	series.UpdatedAt = now
	if err := s.series.Save(ctx, series); err != nil {
		return nil, "", fmt.Errorf("raidsetup: Serienzähler speichern: %w", err)
	}
	after, err := json.Marshal(rd)
	if err != nil {
		return nil, "", err
	}
	if err := s.imports.writeAudit(ctx, &audit.Log{
		ActorID:    &actorID,
		EntityType: "raid",
		EntityID:   strconv.FormatInt(rd.ID, 10),
		Action:     "create",
		BeforeJSON: "null",
		AfterJSON:  string(after),
		Reason:     "Google Sheet ICC25 Raidsetup: nächster geplanter Raid nach " + lastName,
		CreatedAt:  now,
	}); err != nil {
		return nil, "", err
	}
	return rd, "", nil
}
