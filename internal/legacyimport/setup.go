package legacyimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/validation"
)

// SetupConfig is explicit administrator input, not a schedule inferred from history.
type SetupConfig struct {
	Name          string `json:"name"`
	Leader        string `json:"leader"`
	FirstDate     string `json:"first_date"`
	StartTime     string `json:"start_time"`
	Timezone      string `json:"timezone"`
	Weekdays      []int  `json:"weekdays"`
	IntervalWeeks int64  `json:"interval_weeks"`
}

type SetupPlan struct {
	Series  raidseries.Series
	Start   time.Time
	Leader  string
	Entries []AttendancePlan
}

type SetupReport struct {
	Name            string    `json:"name"`
	Leader          string    `json:"leader"`
	ScheduledStart  time.Time `json:"scheduled_start"`
	Timezone        string    `json:"timezone"`
	Weekdays        []int     `json:"weekdays"`
	IntervalWeeks   int64     `json:"interval_weeks"`
	Attendance      int       `json:"attendance"`
	AlreadyImported bool      `json:"already_imported"`
}

func setupHash(rawHash string, config *SetupConfig) string {
	if config == nil {
		return rawHash
	}
	// SetupConfig contains only JSON-supported scalar fields and a slice of ints.
	data, _ := json.Marshal(config)
	return fmt.Sprintf("%x", sha256.Sum256(append([]byte(rawHash+"\x00setup-v1\x00"), data...)))
}

// parseSetup reads only the first numbered roster block. Later notes and secondary
// tables in A:F are reported, never interpreted as extra participants or substitutes.
func parseSetup(data []byte, p *Plan) ([]AttendancePlan, int, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 || len(rows[0]) < 6 {
		p.Errors = append(p.Errors, "ICC25 Raidsetup: A:F header missing")
		return nil, 0, nil
	}
	for i, expected := range []string{"Charakter", "Spieler", "Klasse", "Spec", "Rolle"} {
		if !strings.EqualFold(strings.TrimSpace(rows[0][i+1]), expected) {
			p.Errors = append(p.Errors, "ICC25 Raidsetup: expected columns A:F = position, Charakter, Spieler, Klasse, Spec, Rolle")
			return nil, len(rows) - 1, nil
		}
	}
	entries := []AttendancePlan{}
	seenPositions, seenCharacters := map[int]bool{}, map[string]bool{}
	ended, ignored := false, 0
	for i, row := range rows[1:] {
		for len(row) < 6 {
			row = append(row, "")
		}
		for j := range row {
			row[j] = strings.TrimSpace(row[j])
		}
		position, err := strconv.Atoi(row[0])
		if err != nil {
			ended = true
		}
		if ended {
			if strings.TrimSpace(strings.Join(row, "")) != "" {
				ignored++
			}
			continue
		}
		if position < 1 || position > 25 || seenPositions[position] {
			p.Errors = append(p.Errors, fmt.Sprintf("ICC25 Raidsetup row %d: invalid or duplicate position %q", i+2, row[0]))
			continue
		}
		seenPositions[position] = true
		if row[1] == "" {
			if row[2] != "" || row[3] != "" || row[4] != "" {
				p.Errors = append(p.Errors, fmt.Sprintf("ICC25 Raidsetup row %d: participant data without a character", i+2))
			} else {
				p.Warnings = append(p.Warnings, fmt.Sprintf("ICC25 Raidsetup position %d: empty slot, no attendance created", position))
			}
			continue
		}
		role, valid := mapRole(row[5])
		class := attendance.NormalizeClass(row[3])
		_, validClass := validation.ClassSpecs[class]
		if row[2] == "" || !valid || !validClass || seenCharacters[row[1]] {
			p.Errors = append(p.Errors, fmt.Sprintf("ICC25 Raidsetup row %d: missing player, invalid role/class or duplicate character %q", i+2, row[1]))
			continue
		}
		seenCharacters[row[1]] = true
		if row[4] == "" {
			p.Warnings = append(p.Warnings, fmt.Sprintf("ICC25 Raidsetup character %q: spec missing; source remains blank and must be completed before archival", row[1]))
		}
		entries = append(entries, AttendancePlan{Position: position, Character: row[1], Player: row[2], Class: class, Spec: row[4], Role: role})
	}
	if ignored > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("ICC25 Raidsetup: %d nonempty rows after the main roster were not imported (notes/secondary tables)", ignored))
	}
	if len(entries) == 0 {
		p.Errors = append(p.Errors, "ICC25 Raidsetup: no participants in the main roster")
	}
	return entries, len(rows) - 1, nil
}

func (s *Service) planWithSetup(ctx context.Context, source, sheetID string, data SheetData, config *SetupConfig) (Plan, int, *SetupReport, error) {
	p, rows, err := planFromSheet(data)
	if err != nil || config == nil {
		return p, rows, nil, err
	}
	mapping, err := s.imports.setupMapping(ctx, source, sheetID)
	if err == nil {
		series, err := s.series.Get(ctx, mapping.SeriesID)
		if err != nil {
			return p, rows, nil, err
		}
		rd, err := s.raids.Get(ctx, mapping.RaidID)
		if err != nil {
			return p, rows, nil, err
		}
		entries, err := s.att.ListByRaid(ctx, rd.ID)
		if err != nil {
			return p, rows, nil, err
		}
		leader, err := s.players.Get(ctx, series.RaidLeaderID)
		if err != nil {
			return p, rows, nil, err
		}
		report := &SetupReport{Name: series.Name, Leader: leader.Name, Timezone: series.Timezone, Weekdays: series.Weekdays, IntervalWeeks: series.IntervalWeeks, Attendance: len(entries), AlreadyImported: true}
		if rd.ScheduledStart != nil {
			report.ScheduledStart = *rd.ScheduledStart
		}
		p.Warnings = append(p.Warnings, "ICC25 Raidsetup already imported: existing series, schedule and roster preserved")
		return p, rows, report, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return p, rows, nil, err
	}
	instance, size := icecrownCitadelInstanceID, 25
	series := raidseries.Series{Name: strings.TrimSpace(config.Name), RaidLeaderID: 1, Weekdays: config.Weekdays, IntervalWeeks: config.IntervalWeeks, StartTime: config.StartTime, Timezone: config.Timezone, CatalogInstanceID: &instance, RaidSize: &size}
	if raidseries.Validate(&series) != nil || strings.TrimSpace(config.Leader) == "" {
		p.Errors = append(p.Errors, "ICC25 Raidsetup: explicit name, leader, weekdays, repeat interval, start time and IANA timezone required")
		return p, rows, nil, nil
	}
	location, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return p, rows, nil, err
	}
	date, err := time.ParseInLocation("2006-01-02", config.FirstDate, location)
	if err != nil {
		p.Errors = append(p.Errors, "ICC25 Raidsetup: first_date must be YYYY-MM-DD")
		return p, rows, nil, nil
	}
	// Reuse DST-safe recurrence validation but preserve the explicit first date,
	// including dates earlier than import time; never silently shift it forward.
	firstOccurrence := series
	firstOccurrence.IntervalWeeks = 1 // The explicit first date anchors later recurrence weeks.
	start, err := raidseries.NextOccurrence(&firstOccurrence, date.AddDate(0, 0, -1), date.AddDate(0, 0, -1))
	if err != nil || start.In(location).Format("2006-01-02") != config.FirstDate {
		p.Errors = append(p.Errors, fmt.Sprintf("ICC25 Raidsetup: first date must match the recurrence and have an unambiguous local start time (%v)", err))
		return p, rows, nil, nil
	}
	if err := s.raids.Catalog().ValidateAssociation(ctx, &instance, &size); err != nil {
		p.Errors = append(p.Errors, fmt.Sprintf("ICC25 Raidsetup catalog: %v", err))
	}
	existingSeries, err := s.series.List(ctx)
	if err != nil {
		return p, rows, nil, err
	}
	for _, existing := range existingSeries {
		if strings.EqualFold(existing.Name, series.Name) {
			p.Errors = append(p.Errors, "ICC25 Raidsetup: series name already exists without setup provenance; existing configuration will not be overwritten")
		}
	}
	if _, err := s.raids.FindByName(ctx, series.Name); err == nil {
		p.Errors = append(p.Errors, "ICC25 Raidsetup: raid name already exists without setup provenance")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return p, rows, nil, err
	}
	entries, setupRows, err := parseSetup(data.Setup, &p)
	if err != nil {
		return p, rows, nil, fmt.Errorf("parse ICC25 Raidsetup: %w", err)
	}
	rows += setupRows
	for _, entry := range entries {
		foundPlayer := false
		for _, pl := range p.Players {
			if pl.Name == entry.Player {
				foundPlayer = true
			}
		}
		if !foundPlayer {
			p.Players = append(p.Players, PlayerPlan{Name: entry.Player, Active: true})
		}
		foundCharacter := false
		for _, char := range p.Characters {
			if char.Name == entry.Character {
				foundCharacter = true
				if char.PlayerName != entry.Player {
					p.Errors = append(p.Errors, fmt.Sprintf("ICC25 Raidsetup: character %q has conflicting owner %q", entry.Character, entry.Player))
				}
			}
		}
		if !foundCharacter {
			p.Characters = append(p.Characters, CharacterPlan{Name: entry.Character, PlayerName: entry.Player, Class: entry.Class, MainSpec: entry.Spec})
		}
	}
	leader := ""
	for _, pl := range p.Players {
		if strings.EqualFold(pl.Name, strings.TrimSpace(config.Leader)) {
			if leader != "" {
				p.Errors = append(p.Errors, "ICC25 Raidsetup: ambiguous leader name")
			}
			leader = pl.Name
		}
	}
	if leader == "" {
		p.Errors = append(p.Errors, fmt.Sprintf("ICC25 Raidsetup: leader %q not found in source players", config.Leader))
	}
	sort.Slice(p.Players, func(i, j int) bool { return p.Players[i].Name < p.Players[j].Name })
	sort.Slice(p.Characters, func(i, j int) bool { return p.Characters[i].Name < p.Characters[j].Name })
	p.Setup = &SetupPlan{Series: series, Start: start, Leader: leader, Entries: entries}
	report := &SetupReport{Name: series.Name, Leader: leader, ScheduledStart: start, Timezone: series.Timezone, Weekdays: series.Weekdays, IntervalWeeks: series.IntervalWeeks, Attendance: len(entries)}
	return p, rows, report, nil
}

func (s *Service) mergeSetup(ctx context.Context, source, sheetID string, p Plan, charIDs, playerIDs map[string]int64, now time.Time, rep *Report) error {
	if p.Setup == nil {
		return nil
	}
	setup := p.Setup
	series := setup.Series
	series.RaidLeaderID = playerIDs[setup.Leader]
	if series.RaidLeaderID == 0 {
		return fmt.Errorf("setup leader: %w", raidseries.ErrLeaderNotFound)
	}
	series.CreatedAt, series.UpdatedAt = now, now
	if err := raidseries.Validate(&series); err != nil {
		return err
	}
	if err := s.series.Save(ctx, &series); err != nil {
		return err
	}
	rd := &raid.Raid{Name: series.Name, RaidLeaderID: series.RaidLeaderID, SeriesID: &series.ID, Status: validation.RaidStatusPlanned, ScheduledStart: &setup.Start, CatalogInstanceID: series.CatalogInstanceID, RaidSize: series.RaidSize, CreatedAt: now, UpdatedAt: now}
	if err := s.raids.Create(ctx, rd); err != nil {
		return err
	}
	for _, entry := range setup.Entries {
		row := &attendance.Attendance{RaidID: rd.ID, PlayerID: playerIDs[entry.Player], CharacterID: charIDs[entry.Character], CharacterName: entry.Character, Class: entry.Class, Spec: entry.Spec, Role: entry.Role, CreatedAt: now, UpdatedAt: now}
		if err := s.att.Create(ctx, row); err != nil {
			return err
		}
		rep.Created.Attendance++
	}
	if err := s.imports.insertSetupMapping(ctx, &setupMapping{Source: source, SheetID: sheetID, SeriesID: series.ID, RaidID: rd.ID}); err != nil {
		return err
	}
	rep.Created.Series++
	rep.Created.Raids++
	return nil
}
