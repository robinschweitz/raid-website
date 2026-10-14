package sheetsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/uptrace/bun"
	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
	"strconv"
	"strings"
	"time"
)

func (s *Service) load(ctx context.Context) (Status, error) {
	var raw string
	err := s.db.NewSelect().TableExpr("sheet_sync_state").ColumnExpr("state_json").Where("id = 1").Scan(ctx, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Status{Enabled: true, Automatic: s.automatic, SheetID: s.sheetID, ExportHashes: map[string]string{}}, nil
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
	status.Automatic = s.automatic
	status.SheetID = s.sheetID
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

func snapshots(ctx context.Context, db bun.IDB) (map[string][][]string, error) {
	var rows map[string][][]string
	err := db.RunInTx(ctx, &sql.TxOptions{ReadOnly: true}, func(ctx context.Context, tx bun.Tx) error {
		var err error
		rows, err = snapshotRows(ctx, tx)
		return err
	})
	return rows, err
}

func snapshotRows(ctx context.Context, db bun.IDB) (map[string][][]string, error) {
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
