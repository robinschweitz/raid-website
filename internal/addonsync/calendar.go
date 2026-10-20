package addonsync

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"raid-website/internal/attendance"
	"raid-website/internal/character"
	"raid-website/internal/raid"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/raidsignup"
)

// The game calendar sync (ADR 092) works in two directions:
//
//   - website -> game: the invite export lists, after the roster, every player
//     without a roster slot who answered on the website, with that answer.
//     RaidLedger puts them into the calendar event with a matching status.
//   - game -> website: RaidLedger exports the calendar event's invite list
//     (RS2;CAL). Staff preview it here and apply selected answers: an
//     acceptance becomes a pending signup with the character's main spec, a
//     refusal becomes "kann nicht", an unsure answer "unsicher".

// calendarDeclineReason is stored as the absence reason of imported refusals.
const calendarDeclineReason = "Im Spielkalender beantwortet"

// answerMembers lists the website answers of players without a roster slot.
// A player's absence wins over a pending signup; the character is the pending
// signup's (the main character first), otherwise the player's main character.
func (s *Service) answerMembers(ctx context.Context, raidID int64, roster []attendance.Attendance) ([]InviteMember, error) {
	overview, err := s.signups.Overview(ctx, raidID)
	if err != nil {
		return nil, fmt.Errorf("load signups of raid %d: %w", raidID, err)
	}
	inRoster := map[int64]bool{}
	for _, row := range roster {
		inRoster[row.PlayerID] = true
	}
	type answer struct {
		playerID int64
		status   string
	}
	var answers []answer
	answered := map[int64]bool{}
	for _, absence := range overview.Absences {
		if inRoster[absence.PlayerID] || answered[absence.PlayerID] {
			continue
		}
		status := InviteDeclined
		if absence.Status == raidsignup.AbsenceTentative {
			status = InviteTentative
		}
		answers = append(answers, answer{playerID: absence.PlayerID, status: status})
		answered[absence.PlayerID] = true
	}
	for _, entry := range overview.Entries {
		if entry.IsWithdrawn || inRoster[entry.PlayerID] || answered[entry.PlayerID] {
			continue
		}
		answers = append(answers, answer{playerID: entry.PlayerID, status: InviteSignup})
		answered[entry.PlayerID] = true
	}
	members := make([]InviteMember, 0, len(answers))
	for _, a := range answers {
		c, spec, err := s.answerCharacter(ctx, a.playerID, overview.Entries)
		if err != nil {
			return nil, err
		}
		if c == nil {
			continue // a player without any active character cannot be invited
		}
		members = append(members, InviteMember{Name: c.Name, Role: raidsignup.SpecRole(spec), Class: attendance.NormalizeClass(c.Class), Status: a.status})
	}
	order := map[string]int{InviteSignup: 1, InviteTentative: 2, InviteDeclined: 3}
	sort.SliceStable(members, func(i, j int) bool {
		if order[members[i].Status] != order[members[j].Status] {
			return order[members[i].Status] < order[members[j].Status]
		}
		return toKey(members[i].Name) < toKey(members[j].Name)
	})
	return members, nil
}

// answerCharacter picks the character to put into the calendar for a player
// and the spec that decides its role.
func (s *Service) answerCharacter(ctx context.Context, playerID int64, entries []raidsignup.Entry) (*character.Character, string, error) {
	var signedUp []*character.Character
	specs := map[int64]string{}
	for _, entry := range entries {
		if entry.PlayerID != playerID || entry.IsWithdrawn {
			continue
		}
		c, err := s.characters.Get(ctx, entry.CharacterID)
		if err != nil {
			return nil, "", fmt.Errorf("load signed-up character %d: %w", entry.CharacterID, err)
		}
		signedUp = append(signedUp, c)
		specs[c.ID] = entry.Spec
	}
	if c := preferMain(signedUp); c != nil {
		return c, specs[c.ID], nil
	}
	owned, err := s.characters.ListByPlayer(ctx, playerID)
	if err != nil {
		return nil, "", fmt.Errorf("list characters of player %d: %w", playerID, err)
	}
	var active []*character.Character
	for i := range owned {
		if owned[i].Active {
			active = append(active, &owned[i])
		}
	}
	if c := preferMain(active); c != nil {
		return c, c.MainSpec, nil
	}
	return nil, "", nil
}

// preferMain returns the main character, otherwise the oldest one.
func preferMain(characters []*character.Character) *character.Character {
	var best *character.Character
	for _, c := range characters {
		switch {
		case best == nil:
			best = c
		case c.IsMain != best.IsMain:
			if c.IsMain {
				best = c
			}
		case c.ID < best.ID:
			best = c
		}
	}
	return best
}

// Calendar preview row statuses.
const (
	CalendarRowKnown   = "known"
	CalendarRowUnknown = "unknown"
	CalendarRowInvalid = "invalid"
)

// Calendar import actions. CalendarActionNone rows are shown, never applied.
const (
	CalendarActionNone      = "none"
	CalendarActionSignup    = "signup"
	CalendarActionDecline   = "decline"
	CalendarActionTentative = "tentative"
)

// Website states of a player in the preview.
const (
	WebsiteRoster    = "roster"
	WebsiteBench     = "bench"
	WebsiteSignup    = "signup"
	WebsiteTentative = "tentative"
	WebsiteDeclined  = "declined"
)

// CalendarPreview compares the calendar event's answers with the website.
type CalendarPreview struct {
	RaidID       int64                `json:"raid_id"`
	HeaderRaidID int64                `json:"header_raid_id"`
	Recorder     string               `json:"recorder"`
	Rows         []CalendarPreviewRow `json:"rows"`
	Warnings     []string             `json:"warnings"`
}

// CalendarPreviewRow is one calendar invite with what applying it would do.
type CalendarPreviewRow struct {
	LineNumber    int    `json:"line_number"`
	Name          string `json:"name"`
	Answer        string `json:"answer"`
	Status        string `json:"status"`
	CharacterID   *int64 `json:"character_id"`
	CharacterName string `json:"character_name"`
	PlayerID      *int64 `json:"player_id"`
	Class         string `json:"class"`
	// WebsiteStates is what the website knows about the player now.
	WebsiteStates []string `json:"website_states"`
	// RosterCharacter names the character holding the player's roster slot.
	RosterCharacter string   `json:"roster_character"`
	Action          string   `json:"action"`
	Spec            string   `json:"spec"`
	Notes           []string `json:"notes"`
	Errors          []string `json:"errors"`
	// absenceReason is the reason of the player's website absence; a calendar
	// answer keeps it instead of writing calendarDeclineReason (ADR 093).
	absenceReason string
}

// CalendarSelection is one previewed answer staff chose to apply.
type CalendarSelection struct {
	CharacterID int64  `json:"character_id"`
	Action      string `json:"action"`
}

// Calendar import result statuses (failures use ResultFailed).
const (
	ResultSignedUp  = "signed_up"
	ResultDeclined  = "declined"
	ResultTentative = "tentative"
	ResultUnchanged = "unchanged"
)

// CalendarImportResult reports what happened to one selection.
type CalendarImportResult struct {
	CharacterID   int64  `json:"character_id"`
	CharacterName string `json:"character_name"`
	Status        string `json:"status"`
	Code          string `json:"code"`
	Message       string `json:"message"`
}

// calendarState is the website's view of one player in this raid.
type calendarState struct {
	rosterCharacter string
	isBench         bool
	absence         string
	absenceReason   string
	pending         map[int64]bool // character IDs with a pending signup
}

// PreviewCalendarImport resolves every calendar invite and proposes an action.
func (s *Service) PreviewCalendarImport(ctx context.Context, raidID int64, text string) (*CalendarPreview, error) {
	rd, err := s.openRaid(ctx, raidID)
	if err != nil {
		return nil, err
	}
	export, err := ParseCalendarExport(text)
	if err != nil {
		return nil, err
	}
	return s.calendarPreview(ctx, rd, export)
}

func (s *Service) calendarPreview(ctx context.Context, rd *raid.Raid, export *CalendarExport) (*CalendarPreview, error) {
	states, err := s.calendarStates(ctx, rd.ID)
	if err != nil {
		return nil, err
	}
	known, err := s.knownCharacters(ctx)
	if err != nil {
		return nil, err
	}
	preview := &CalendarPreview{RaidID: rd.ID, HeaderRaidID: export.RaidID, Recorder: export.Recorder, Rows: []CalendarPreviewRow{}, Warnings: []string{}}
	if export.RaidID != rd.ID {
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("Der Export stammt vom Kalender-Termin für Raid #%d, nicht für diesen Raid (#%d).", export.RaidID, rd.ID))
	}
	proposed := map[int64]bool{} // one action per player, the first answer wins
	for _, invite := range export.Invites {
		row := CalendarPreviewRow{LineNumber: invite.LineNumber, Name: invite.Name, Answer: invite.Answer, Action: CalendarActionNone, WebsiteStates: []string{}, Notes: []string{}, Errors: []string{}}
		if invite.Error != "" {
			row.Status = CalendarRowInvalid
			row.Errors = append(row.Errors, fmt.Sprintf("Zeile %d: %s", invite.LineNumber, invite.Error))
			preview.Rows = append(preview.Rows, row)
			continue
		}
		c := known.find(invite.Name)
		if c == nil {
			row.Status = CalendarRowUnknown
			row.Notes = append(row.Notes, "Charakter ist nicht auf der Webseite.")
			preview.Rows = append(preview.Rows, row)
			continue
		}
		characterID, playerID := c.ID, c.PlayerID
		row.Status, row.CharacterID, row.PlayerID = CalendarRowKnown, &characterID, &playerID
		row.CharacterName, row.Class = c.Name, attendance.NormalizeClass(c.Class)
		state := states[c.PlayerID]
		row.WebsiteStates, row.RosterCharacter = state.describe(c.ID), state.rosterCharacter
		row.Action, row.Spec, row.Notes = proposeCalendarAction(invite.Answer, c, state)
		row.absenceReason = state.absenceReason
		if row.Action != CalendarActionNone {
			if proposed[c.PlayerID] {
				row.Action, row.Spec = CalendarActionNone, ""
				row.Notes = append(row.Notes, "Für diesen Spieler wird schon eine andere Antwort übernommen.")
			}
			proposed[c.PlayerID] = true
		}
		preview.Rows = append(preview.Rows, row)
	}
	return preview, nil
}

func (state calendarState) describe(characterID int64) []string {
	states := []string{}
	switch {
	case state.rosterCharacter != "" && state.isBench:
		states = append(states, WebsiteBench)
	case state.rosterCharacter != "":
		states = append(states, WebsiteRoster)
	}
	if state.pending[characterID] {
		states = append(states, WebsiteSignup)
	}
	switch state.absence {
	case raidsignup.AbsenceDeclined:
		states = append(states, WebsiteDeclined)
	case raidsignup.AbsenceTentative:
		states = append(states, WebsiteTentative)
	}
	return states
}

// proposeCalendarAction maps a calendar answer onto the website. Statuses the
// raid lead sets in game (confirmed, standby) and missing answers change nothing.
func proposeCalendarAction(answer string, c *character.Character, state calendarState) (string, string, []string) {
	switch answer {
	case CalendarAccepted, CalendarSignedUp:
		switch {
		case state.rosterCharacter != "":
			return CalendarActionNone, "", []string{fmt.Sprintf("Steht schon mit %s in der Aufstellung.", state.rosterCharacter)}
		case state.pending[c.ID]:
			return CalendarActionNone, "", []string{"Ist schon angemeldet."}
		}
		notes := []string{fmt.Sprintf("Wird mit %s angemeldet; die Raidleitung nimmt wie gewohnt an.", c.MainSpec)}
		if state.absence != "" {
			notes = append(notes, "Die Absage bzw. „unsicher“ auf der Webseite wird dabei entfernt.")
		}
		return CalendarActionSignup, c.MainSpec, notes
	case CalendarDeclined, CalendarOut:
		if state.absence == raidsignup.AbsenceDeclined {
			return CalendarActionNone, "", []string{"Absage ist schon eingetragen."}
		}
		notes := []string{"Wird als „kann nicht“ eingetragen."}
		if state.rosterCharacter != "" {
			notes = append(notes, fmt.Sprintf("Wird mit %s aus der Aufstellung entfernt.", state.rosterCharacter))
		}
		if len(state.pending) > 0 {
			notes = append(notes, "Offene Anmeldung wird zurückgezogen.")
		}
		return CalendarActionDecline, "", append(notes, state.keptReasonNotes()...)
	case CalendarTentative:
		if state.absence == raidsignup.AbsenceTentative {
			return CalendarActionNone, "", []string{"Ist schon als unsicher eingetragen."}
		}
		return CalendarActionTentative, "", append([]string{"Wird als „unsicher“ eingetragen."}, state.keptReasonNotes()...)
	case CalendarConfirmed, CalendarStandby:
		return CalendarActionNone, "", []string{"Status wurde von der Raidleitung gesetzt."}
	}
	return CalendarActionNone, "", []string{"Noch keine Antwort im Kalender."}
}

// keptReasonNotes tells staff that the website's reason stays (ADR 093).
func (state calendarState) keptReasonNotes() []string {
	if state.absenceReason == "" {
		return nil
	}
	return []string{fmt.Sprintf("Grund „%s“ bleibt erhalten.", state.absenceReason)}
}

// calendarReason is the absence reason a calendar refusal or unsure answer
// stores: the website's own reason if there is one, else calendarDeclineReason.
func calendarReason(row CalendarPreviewRow) string {
	if row.absenceReason != "" {
		return row.absenceReason
	}
	return calendarDeclineReason
}

// calendarStates collects roster slot, absence and pending signups per player.
func (s *Service) calendarStates(ctx context.Context, raidID int64) (map[int64]calendarState, error) {
	roster, err := s.rosterEntries(ctx, raidID)
	if err != nil {
		return nil, err
	}
	overview, err := s.signups.Overview(ctx, raidID)
	if err != nil {
		return nil, fmt.Errorf("load signups of raid %d: %w", raidID, err)
	}
	states := map[int64]calendarState{}
	get := func(playerID int64) calendarState {
		state, ok := states[playerID]
		if !ok {
			state = calendarState{pending: map[int64]bool{}}
		}
		return state
	}
	for _, entry := range roster {
		state := get(entry.attendance.PlayerID)
		if state.rosterCharacter == "" {
			state.rosterCharacter, state.isBench = entry.currentName, entry.attendance.IsBench
		}
		states[entry.attendance.PlayerID] = state
	}
	for _, entry := range overview.Entries {
		if entry.IsWithdrawn || entry.IsAccepted {
			continue
		}
		state := get(entry.PlayerID)
		state.pending[entry.CharacterID] = true
		states[entry.PlayerID] = state
	}
	for _, absence := range overview.Absences {
		state := get(absence.PlayerID)
		state.absence, state.absenceReason = absence.Status, absence.Reason
		states[absence.PlayerID] = state
	}
	return states, nil
}

// ApplyCalendarImport applies the selected answers. Each selection must match
// the action the preview proposes for the export right now, so a stale or
// edited request changes nothing it was not shown. Every change goes through
// the signup service, which audits it.
func (s *Service) ApplyCalendarImport(ctx context.Context, raidID, actorID int64, text string, selections []CalendarSelection) ([]CalendarImportResult, error) {
	rd, err := s.openRaid(ctx, raidID)
	if err != nil {
		return nil, err
	}
	export, err := ParseCalendarExport(text)
	if err != nil {
		return nil, err
	}
	preview, err := s.calendarPreview(ctx, rd, export)
	if err != nil {
		return nil, err
	}
	proposals := map[int64]CalendarPreviewRow{}
	for _, row := range preview.Rows {
		if row.CharacterID != nil && row.Action != CalendarActionNone {
			proposals[*row.CharacterID] = row
		}
	}
	results := make([]CalendarImportResult, 0, len(selections))
	for _, selection := range selections {
		result := CalendarImportResult{CharacterID: selection.CharacterID}
		row, ok := proposals[selection.CharacterID]
		if !ok || row.Action != selection.Action {
			result.Status, result.Code, result.Message = ResultFailed, "not_proposed", "Diese Antwort schlägt die Vorschau nicht (mehr) vor; bitte neu abgleichen"
			results = append(results, result)
			continue
		}
		result.CharacterName = row.CharacterName
		if err := s.applyCalendarRow(ctx, rd.ID, actorID, row, &result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Service) applyCalendarRow(ctx context.Context, raidID, actorID int64, row CalendarPreviewRow, result *CalendarImportResult) error {
	playerID := *row.PlayerID
	var err error
	switch row.Action {
	case CalendarActionSignup:
		// Also removes the website's refusal in the same transaction (preview note).
		err = s.signups.SubmitForPlayer(ctx, raidID, actorID, *row.CharacterID, row.Spec)
		result.Status = ResultSignedUp
	case CalendarActionDecline:
		// Also removes the roster slot and withdraws open signups (ADR 093).
		err = s.signups.Decline(ctx, raidID, actorID, playerID, calendarReason(row), raidsignup.AbsenceDeclined)
		result.Status = ResultDeclined
	case CalendarActionTentative:
		err = s.signups.Decline(ctx, raidID, actorID, playerID, calendarReason(row), raidsignup.AbsenceTentative)
		result.Status = ResultTentative
	}
	switch {
	case err == nil:
	case errors.Is(err, raidsignup.ErrAlreadySignedUp):
		result.Status = ResultUnchanged
	case errors.Is(err, raidcorrection.ErrHasLoot):
		result.Status, result.Code, result.Message = ResultFailed, "attendance_has_loot", "Spieler hat in diesem Raid schon Loot; zuerst den Loot zurückziehen"
	case errors.Is(err, raidsignup.ErrInvalid):
		result.Status, result.Code, result.Message = ResultFailed, "invalid_spec", "Die Main-Spec des Charakters passt nicht zu seiner Klasse; bitte am Charakter korrigieren"
	case errors.Is(err, raidsignup.ErrRaidState), errors.Is(err, raidsignup.ErrTooLate):
		result.Status, result.Code, result.Message = ResultFailed, "raid_state", "Raid erlaubt diese Änderung nicht"
	case errors.Is(err, raidsignup.ErrForbidden):
		result.Status, result.Code, result.Message = ResultFailed, "forbidden", "Raidpersonal erforderlich"
	case errors.Is(err, raidsignup.ErrConflict), errors.Is(err, raidsignup.ErrNotFound):
		result.Status, result.Code, result.Message = ResultFailed, "conflict", "Anmeldung hat sich geändert; bitte neu abgleichen"
	default:
		return fmt.Errorf("apply calendar answer of character %d: %w", *row.CharacterID, err)
	}
	return nil
}
