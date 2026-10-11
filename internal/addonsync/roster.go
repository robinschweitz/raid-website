package addonsync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/raid"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/validation"
)

// lateJoinGrace is how long after the raid start a first sighting still counts as on time.
const lateJoinGrace = 15 * time.Minute

// Roster row statuses.
const (
	RosterInRoster = "in_roster"
	RosterKnown    = "known"
	RosterUnknown  = "unknown"
	RosterInvalid  = "invalid"
)

// RosterPreview compares who the addon saw in the raid with the website roster.
type RosterPreview struct {
	RaidID       int64              `json:"raid_id"`
	HeaderRaidID int64              `json:"header_raid_id"`
	Recorder     string             `json:"recorder"`
	Rows         []RosterPreviewRow `json:"rows"`
	Missing      []string           `json:"missing"`
	Warnings     []string           `json:"warnings"`
}

// RosterPreviewRow is one character from the addon export. Known characters can
// be added to the roster; unknown ones need the Rising-Gods quick-add first.
type RosterPreviewRow struct {
	LineNumber      int                       `json:"line_number"`
	Name            string                    `json:"name"`
	ClassToken      string                    `json:"class_token"`
	FirstSeen       *time.Time                `json:"first_seen"`
	Status          string                    `json:"status"`
	CharacterID     *int64                    `json:"character_id"`
	CharacterName   string                    `json:"character_name"`
	PlayerID        *int64                    `json:"player_id"`
	Class           string                    `json:"class"`
	SuggestedRole   validation.AttendanceRole `json:"suggested_role"`
	SuggestLateJoin bool                      `json:"suggest_late_join"`
	Errors          []string                  `json:"errors"`
}

// RosterSelection is the staff decision for one known character.
type RosterSelection struct {
	CharacterID int64                     `json:"character_id"`
	Role        validation.AttendanceRole `json:"role"`
	LateJoin    bool                      `json:"late_join"`
}

// Roster import result statuses; failures use ResultFailed.
const (
	ResultAdded           = "added"
	ResultAlreadyInRoster = "already_in_roster"
)

// RosterImportResult reports what happened to one selected character.
type RosterImportResult struct {
	CharacterID   int64  `json:"character_id"`
	CharacterName string `json:"character_name"`
	Status        string `json:"status"`
	AttendanceID  *int64 `json:"attendance_id"`
	Code          string `json:"code"`
	Message       string `json:"message"`
}

// PreviewRosterImport matches every character the addon saw against the raid
// roster and the known characters.
func (s *Service) PreviewRosterImport(ctx context.Context, raidID int64, text string) (*RosterPreview, error) {
	rd, err := s.openRaid(ctx, raidID)
	if err != nil {
		return nil, err
	}
	export, err := ParseRosterExport(text)
	if err != nil {
		return nil, err
	}
	roster, err := s.rosterEntries(ctx, rd.ID)
	if err != nil {
		return nil, err
	}
	known, err := s.knownCharacters(ctx)
	if err != nil {
		return nil, err
	}
	preview := &RosterPreview{RaidID: rd.ID, HeaderRaidID: export.RaidID, Recorder: export.Recorder, Rows: []RosterPreviewRow{}, Missing: []string{}, Warnings: []string{}}
	if export.RaidID != rd.ID {
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("Der Export wurde für Raid #%d aufgezeichnet, nicht für diesen Raid (#%d).", export.RaidID, rd.ID))
	}
	seen := map[int64]bool{}
	for _, member := range export.Members {
		row, err := s.resolveRosterMember(ctx, rd, roster, known, member)
		if err != nil {
			return nil, err
		}
		if row.CharacterID != nil {
			seen[*row.CharacterID] = true
		}
		preview.Rows = append(preview.Rows, row)
	}
	for _, entry := range roster {
		if !entry.attendance.IsBench && !seen[entry.attendance.CharacterID] {
			preview.Missing = append(preview.Missing, entry.currentName)
		}
	}
	return preview, nil
}

func (s *Service) resolveRosterMember(ctx context.Context, rd *raid.Raid, roster []rosterEntry, known knownCharacters, member RosterMember) (RosterPreviewRow, error) {
	row := RosterPreviewRow{LineNumber: member.LineNumber, Name: member.Name, ClassToken: member.ClassToken, Errors: []string{}}
	if member.Error != "" {
		row.Status = RosterInvalid
		row.Errors = append(row.Errors, fmt.Sprintf("Zeile %d: %s", member.LineNumber, member.Error))
		return row, nil
	}
	firstSeen := member.FirstSeen
	row.FirstSeen = &firstSeen
	if entry := findRosterEntry(roster, member.Name); entry != nil {
		characterID, playerID := entry.attendance.CharacterID, entry.attendance.PlayerID
		row.Status, row.CharacterID, row.PlayerID = RosterInRoster, &characterID, &playerID
		row.CharacterName, row.Class, row.SuggestedRole = entry.currentName, entry.attendance.Class, entry.attendance.Role
		return row, nil
	}
	c := known.find(member.Name)
	if c == nil {
		row.Status = RosterUnknown
		return row, nil
	}
	role, err := s.suggestedRole(ctx, c)
	if err != nil {
		return row, err
	}
	characterID, playerID := c.ID, c.PlayerID
	row.Status, row.CharacterID, row.PlayerID = RosterKnown, &characterID, &playerID
	row.CharacterName, row.Class, row.SuggestedRole = c.Name, c.Class, role
	if start := raidStart(rd); start != nil && member.FirstSeen.After(start.Add(lateJoinGrace)) {
		row.SuggestLateJoin = true
	}
	return row, nil
}

// ApplyRosterImport adds the selected known characters to the roster through
// the ordinary correction path. Only characters named in the export are
// accepted, and characters already in the roster are reported, not duplicated.
func (s *Service) ApplyRosterImport(ctx context.Context, raidID, actorID int64, text string, selections []RosterSelection) ([]RosterImportResult, error) {
	rd, err := s.openRaid(ctx, raidID)
	if err != nil {
		return nil, err
	}
	export, err := ParseRosterExport(text)
	if err != nil {
		return nil, err
	}
	named := map[string]bool{}
	for _, member := range export.Members {
		if member.Error == "" {
			named[strings.ToLower(member.Name)] = true
		}
	}
	results := make([]RosterImportResult, 0, len(selections))
	for _, selection := range selections {
		result := RosterImportResult{CharacterID: selection.CharacterID}
		c, err := s.characters.Get(ctx, selection.CharacterID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			result.Status, result.Code, result.Message = ResultFailed, "unknown_character", "Charakter nicht gefunden"
		case err != nil:
			return nil, fmt.Errorf("load character %d: %w", selection.CharacterID, err)
		case !named[strings.ToLower(c.Name)]:
			result.CharacterName = c.Name
			result.Status, result.Code, result.Message = ResultFailed, "not_in_export", "Charakter steht nicht im Addon-Export"
		default:
			result.CharacterName = c.Name
			s.addRosterCharacter(ctx, rd.ID, actorID, c, selection, &result)
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Service) addRosterCharacter(ctx context.Context, raidID, actorID int64, c *character.Character, selection RosterSelection, result *RosterImportResult) {
	exists, err := s.attendance.Exists(ctx, raidID, c.PlayerID, c.ID)
	if err == nil && exists {
		result.Status, result.Code, result.Message = ResultAlreadyInRoster, "", ""
		return
	}
	now := time.Now().UTC()
	row := &attendance.Attendance{RaidID: raidID, PlayerID: c.PlayerID, CharacterID: c.ID, Role: selection.Role, LateJoin: selection.LateJoin, CreatedAt: now, UpdatedAt: now}
	if err == nil {
		err = s.corrections.SaveAttendance(ctx, row, actorID, "")
	}
	switch {
	case err == nil:
		result.Status, result.AttendanceID = ResultAdded, &row.ID
	case errors.Is(err, raidcorrection.ErrConflict):
		result.Status = ResultAlreadyInRoster
	case errors.Is(err, raidcorrection.ErrInvalid):
		result.Status, result.Code, result.Message = ResultFailed, "invalid_role", "Rolle muss TANK, HEAL oder DD sein"
	case errors.Is(err, raidcorrection.ErrRaidState):
		result.Status, result.Code, result.Message = ResultFailed, "raid_state", "Raid erlaubt diese Änderung nicht"
	default:
		result.Status, result.Code, result.Message = ResultFailed, "save_failed", "Teilnahme konnte nicht gespeichert werden"
	}
}

// knownCharacters indexes the character directory by lower-cased name. The
// addon reports names as the game shows them, which need not match the case
// stored on the website; folding in Go also covers umlauts, which SQLite's
// lower() leaves alone.
type knownCharacters map[string][]character.Character

func (s *Service) knownCharacters(ctx context.Context) (knownCharacters, error) {
	rows, err := s.characters.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list characters: %w", err)
	}
	known := knownCharacters{}
	for _, c := range rows {
		key := toKey(c.Name)
		known[key] = append(known[key], c)
	}
	return known, nil
}

// find prefers an exact-case match and otherwise the oldest character, the
// same tie-break as character.Repo.FindByName.
func (k knownCharacters) find(name string) *character.Character {
	matches := k[toKey(name)]
	if len(matches) == 0 {
		return nil
	}
	best := &matches[0]
	for i := range matches {
		switch {
		case matches[i].Name == name && best.Name != name:
			best = &matches[i]
		case (matches[i].Name == name) == (best.Name == name) && matches[i].ID < best.ID:
			best = &matches[i]
		}
	}
	return best
}

// suggestedRole repeats the role the character played last time, or DD.
func (s *Service) suggestedRole(ctx context.Context, c *character.Character) (validation.AttendanceRole, error) {
	last, err := s.attendance.LatestByCharacter(ctx, c.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return validation.RoleDD, nil
	}
	if err != nil {
		return "", fmt.Errorf("latest attendance of character %d: %w", c.ID, err)
	}
	return last.Role, nil
}

func (s *Service) rosterEntries(ctx context.Context, raidID int64) ([]rosterEntry, error) {
	rows, err := s.attendance.ListByRaid(ctx, raidID)
	if err != nil {
		return nil, fmt.Errorf("list attendance for raid %d: %w", raidID, err)
	}
	roster := make([]rosterEntry, 0, len(rows))
	for _, row := range rows {
		name, err := s.currentCharacterName(ctx, row)
		if err != nil {
			return nil, err
		}
		roster = append(roster, rosterEntry{attendance: row, currentName: name})
	}
	return roster, nil
}

// findRosterEntry matches the current character name first, then the name
// the roster stored when the character was added.
func findRosterEntry(roster []rosterEntry, name string) *rosterEntry {
	for i := range roster {
		if strings.EqualFold(roster[i].currentName, name) {
			return &roster[i]
		}
	}
	for i := range roster {
		if strings.EqualFold(roster[i].attendance.CharacterName, name) {
			return &roster[i]
		}
	}
	return nil
}

func raidStart(rd *raid.Raid) *time.Time {
	if rd.StartedAt != nil {
		return rd.StartedAt
	}
	return rd.ScheduledStart
}

func toKey(name string) string {
	return strings.ToLower(name)
}
