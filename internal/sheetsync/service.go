package sheetsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/uptrace/bun"
	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/legacyimport"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

const Source = "google_sheets"

// Status is persisted so restarts retain the last good export fingerprint.
// A failure is visible to admins and never silently treated as a successful sync.
type Status struct {
	Enabled       bool                          `json:"enabled"`
	SheetID       string                        `json:"sheet_id"`
	BackupSheetID string                        `json:"backup_sheet_id"`
	LastRun       *time.Time                    `json:"last_run,omitempty"`
	LastSuccess   *time.Time                    `json:"last_success,omitempty"`
	LastError     string                        `json:"last_error,omitempty"`
	LastImport    *legacyimport.Report          `json:"last_import,omitempty"`
	CurrentSetup  *legacyimport.SetupSyncReport `json:"current_setup,omitempty"`
	ExportHashes  map[string]string             `json:"-"`
}

type Service struct {
	mu       sync.Mutex
	db       *bun.DB
	importer *legacyimport.Service
	writer   MirrorWriter
	sheetID  string
	backupID string
	actorID  int64
	reason   string
	interval time.Duration
}

func New(db *bun.DB, importer *legacyimport.Service, writer MirrorWriter, sheetID, backupID string, actorID int64, reason string, interval time.Duration) (*Service, error) {
	if db == nil || importer == nil || writer == nil || strings.TrimSpace(sheetID) == "" || strings.TrimSpace(backupID) == "" || sheetID == backupID || actorID <= 0 || strings.TrimSpace(reason) == "" || interval < time.Minute {
		return nil, errors.New("sheet sync needs a database, importer, writer, sheet ID, distinct backup ID, actor, reason and interval >= 1m")
	}
	return &Service{db: db, importer: importer, writer: writer, sheetID: sheetID, backupID: backupID, actorID: actorID, reason: reason, interval: interval}, nil
}

func (s *Service) load(ctx context.Context) (Status, error) {
	var raw string
	err := s.db.NewSelect().TableExpr("sheet_sync_state").ColumnExpr("state_json").Where("id = 1").Scan(ctx, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Status{Enabled: true, SheetID: s.sheetID, BackupSheetID: s.backupID, ExportHashes: map[string]string{}}, nil
	}
	if err != nil {
		return Status{}, err
	}
	var status Status
	// Export hashes are intentionally a private persistence detail, not sent in API JSON.
	var stored struct {
		Status
		ExportHashes map[string]string `json:"export_hashes"`
	}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return Status{}, err
	}
	status = stored.Status
	status.Enabled = true
	status.SheetID = s.sheetID
	status.BackupSheetID = s.backupID
	status.ExportHashes = stored.ExportHashes
	if status.ExportHashes == nil {
		status.ExportHashes = map[string]string{}
	}
	return status, nil
}

func (s *Service) save(ctx context.Context, status Status) error {
	encoded, err := json.Marshal(struct {
		Status
		ExportHashes map[string]string `json:"export_hashes"`
	}{status, status.ExportHashes})
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO sheet_sync_state (id, state_json) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET state_json = excluded.state_json", string(encoded))
	return err
}

func (s *Service) GetStatus(ctx context.Context) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(ctx)
}

// RunOnce first reconciles append-only historical sheet rows, then imports only
// new, known participants from ICC25 Raidsetup, then exports website snapshots.
// Import conflicts prevent the export, preserving evidence for an administrator.
func (s *Service) RunOnce(ctx context.Context) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.load(ctx)
	if err != nil {
		return Status{}, err
	}
	now := time.Now().UTC()
	status.LastRun = &now
	status.LastError = ""
	// No import or export writes until the separately stored workbook copy is accessible.
	var backupVerifier, ok = s.writer.(interface {
		VerifyBackup(context.Context, string, string) error
	})
	var backupErr error
	if !ok {
		backupErr = errors.New("sheet writer must verify the backup before any sync")
	} else {
		backupErr = backupVerifier.VerifyBackup(ctx, s.sheetID, s.backupID)
	}
	var preview legacyimport.Report
	if backupErr != nil {
		err = fmt.Errorf("backup prerequisite: %w", backupErr)
	} else {
		preview, err = s.importer.DryRun(ctx, Source, s.sheetID)
	}
	if err == nil && len(preview.Errors) > 0 {
		err = fmt.Errorf("historical sheet conflicts: %s", strings.Join(preview.Errors, "; "))
	}
	if err == nil {
		var merged legacyimport.Report
		merged, err = s.importer.Merge(ctx, s.actorID, Source, s.sheetID, preview.RawHash, s.reason)
		if err == nil && len(merged.Errors) > 0 {
			err = fmt.Errorf("historical sheet merge conflicts: %s", strings.Join(merged.Errors, "; "))
		}
		if err == nil {
			status.LastImport = &merged
		}
	}
	if err == nil {
		var setup legacyimport.SetupSyncReport
		setup, err = s.importer.SyncCurrentSetup(ctx, s.actorID, Source, s.sheetID, preview.RawHash)
		if err == nil {
			status.CurrentSetup = &setup
			if len(setup.Conflicts) > 0 {
				err = fmt.Errorf("current raid setup conflicts: %s", strings.Join(setup.Conflicts, "; "))
			}
		}
	}
	if err == nil {
		var data map[string][][]string
		data, err = snapshots(ctx, s.db)
		if err == nil {
			var hashes map[string]string
			hashes, err = s.writer.Publish(ctx, s.sheetID, data, status.ExportHashes)
			if err == nil {
				status.ExportHashes = hashes
			}
		}
	}
	if err != nil {
		status.LastError = err.Error()
	} else {
		status.LastSuccess = &now
	}
	if saveErr := s.save(ctx, status); saveErr != nil {
		return status, fmt.Errorf("persist sync status: %w", saveErr)
	}
	return status, err
}

// Start runs at startup and then at the configured interval. Only one process
// should run against a SQLite database; RunOnce serializes HTTP and timer runs.
func (s *Service) Start(ctx context.Context) {
	go func() {
		run := func() {
			runCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
			defer cancel()
			if _, err := s.RunOnce(runCtx); err != nil {
				log.Printf("google sheet sync: %v", err)
			}
		}
		run()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func snapshots(ctx context.Context, db bun.IDB) (map[string][][]string, error) {
	var players []player.Player
	var characters []character.Character
	var raids []raid.Raid
	var attendanceRows []attendance.Attendance
	var lootRows []loot.Award
	var points []enhancepoints.Transaction
	queries := []func() error{
		func() error { return db.NewSelect().Model(&players).Order("id ASC").Scan(ctx) },
		func() error { return db.NewSelect().Model(&characters).Order("id ASC").Scan(ctx) },
		func() error { return db.NewSelect().Model(&raids).Order("id ASC").Scan(ctx) },
		func() error { return db.NewSelect().Model(&attendanceRows).Order("id ASC").Scan(ctx) },
		func() error { return db.NewSelect().Model(&lootRows).Order("id ASC").Scan(ctx) },
		func() error { return db.NewSelect().Model(&points).Order("id ASC").Scan(ctx) },
	}
	for _, q := range queries {
		if err := q(); err != nil {
			return nil, err
		}
	}
	series, err := raidseries.NewRepo(db).List(ctx)
	if err != nil {
		return nil, err
	}
	playerNames := map[int64]string{}
	for _, p := range players {
		playerNames[p.ID] = p.Name
	}
	raidNames := map[int64]string{}
	activeRaids := map[int64]bool{}
	for _, rd := range raids {
		raidNames[rd.ID] = rd.Name
		activeRaids[rd.ID] = rd.ArchivedAt == nil && (rd.Status == validation.RaidStatusPlanned || rd.Status == validation.RaidStatusOpen)
	}
	yesNo := func(value bool) string {
		if value {
			return "Ja"
		}
		return "Nein"
	}
	nullableTime := func(value *time.Time) string {
		if value == nil {
			return ""
		}
		return value.UTC().Format(time.RFC3339)
	}
	number := func(value int64) string { return strconv.FormatInt(value, 10) }
	sheets := map[string][][]string{
		"Website Raids":      {{"Raid ID", "Name", "Status", "Start (UTC)", "Archiviert (UTC)", "Raidleiter", "Serie ID"}},
		"Website Raidsetup":  {{"Raid ID", "Raid", "Start (UTC)", "Charakter ID", "Charakter", "Spieler", "Klasse", "Spec", "Rolle", "Ersatzbank"}},
		"Website Teilnahmen": {{"Teilnahme ID", "Raid ID", "Raid", "Spieler", "Charakter", "Klasse", "Spec", "Rolle", "Ersatzbank"}},
		"Website Loot":       {{"Loot ID", "Raid ID", "Raid", "Datum (UTC)", "Charakter", "Spieler", "Gegenstand", "Item ID", "Boss", "Priorität", "EP Kosten", "Quelle"}},
		"Website Charaktere": {{"Charakter ID", "Charakter", "Spieler", "Klasse", "Main Spec", "Second Spec", "Aktiv"}},
		"Website Punkte":     {{"Buchung ID", "Spieler", "Punkte Änderung", "Typ", "Raid ID", "Loot ID", "Datum (UTC)", "Grund"}},
		"Website Raidserien": {{"Serien ID", "Name", "Raidleiter", "Wochentage", "Intervall Wochen", "Startzeit", "Zeitzone"}},
	}
	for _, rd := range raids {
		seriesID := ""
		if rd.SeriesID != nil {
			seriesID = number(*rd.SeriesID)
		}
		sheets["Website Raids"] = append(sheets["Website Raids"], []string{number(rd.ID), rd.Name, string(rd.Status), nullableTime(rd.ScheduledStart), nullableTime(rd.ArchivedAt), playerNames[rd.RaidLeaderID], seriesID})
	}
	for _, c := range characters {
		sheets["Website Charaktere"] = append(sheets["Website Charaktere"], []string{number(c.ID), c.Name, playerNames[c.PlayerID], c.Class, c.MainSpec, c.SecondSpec, yesNo(c.Active)})
	}
	for _, a := range attendanceRows {
		row := []string{number(a.ID), number(a.RaidID), raidNames[a.RaidID], playerNames[a.PlayerID], a.CharacterName, a.Class, a.Spec, string(a.Role), yesNo(a.IsBench)}
		sheets["Website Teilnahmen"] = append(sheets["Website Teilnahmen"], row)
		if activeRaids[a.RaidID] {
			var start string
			for _, rd := range raids {
				if rd.ID == a.RaidID {
					start = nullableTime(rd.ScheduledStart)
					break
				}
			}
			sheets["Website Raidsetup"] = append(sheets["Website Raidsetup"], []string{number(a.RaidID), raidNames[a.RaidID], start, number(a.CharacterID), a.CharacterName, playerNames[a.PlayerID], a.Class, a.Spec, string(a.Role), yesNo(a.IsBench)})
		}
	}
	for _, l := range lootRows {
		sheets["Website Loot"] = append(sheets["Website Loot"], []string{number(l.ID), number(l.RaidID), raidNames[l.RaidID], l.AwardedAt.UTC().Format(time.RFC3339), l.AwardedToCharacterName, playerNames[l.AwardedToPlayerID], l.ItemName, l.ItemID, l.Boss, string(l.AwardMethod), strconv.Itoa(l.EnhancedCost), l.Source})
	}
	for _, tx := range points {
		raidID, lootID := "", ""
		if tx.RaidID != nil {
			raidID = number(*tx.RaidID)
		}
		if tx.LootAwardID != nil {
			lootID = number(*tx.LootAwardID)
		}
		sheets["Website Punkte"] = append(sheets["Website Punkte"], []string{number(tx.ID), playerNames[tx.PlayerID], strconv.Itoa(tx.Amount), tx.Type, raidID, lootID, tx.CreatedAt.UTC().Format(time.RFC3339), tx.Reason})
	}
	for _, se := range series {
		days := make([]string, len(se.Weekdays))
		for i, day := range se.Weekdays {
			days[i] = strconv.Itoa(day)
		}
		sheets["Website Raidserien"] = append(sheets["Website Raidserien"], []string{number(se.ID), se.Name, playerNames[se.RaidLeaderID], strings.Join(days, ","), strconv.FormatInt(se.IntervalWeeks, 10), se.StartTime, se.Timezone})
	}
	return sheets, nil
}
