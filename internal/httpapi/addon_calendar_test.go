package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/character"
	"raid-website/internal/raidsignup"
	"raid-website/internal/validation"
)

// calendarFixture adds two players beside the correction fixture's: a priest
// who signed up and a warrior who is unsure.
type calendarFixture struct {
	*correctionFixture
	healer, healerChar   int64
	unsure, unsureChar   int64
	stranger, strangerCh int64
}

func newCalendarFixture(t *testing.T) *calendarFixture {
	t.Helper()
	f := &calendarFixture{correctionFixture: newCorrectionFixture(t)}
	ctx := context.Background()
	now := time.Now().UTC()
	create := func(username, name, class, spec string, isMain bool) (int64, int64) {
		playerID := seedAccount(t, f.db, username, "password", validation.RoleMember)
		c := &character.Character{PlayerID: playerID, Name: name, Class: class, MainSpec: spec, SecondSpec: spec, IsMain: isMain, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := character.NewRepo(f.db).Create(ctx, c); err != nil {
			t.Fatal(err)
		}
		return playerID, c.ID
	}
	f.healer, f.healerChar = create("calendar-healer", "Heilerin", "Priester", "Heilig", true)
	f.unsure, f.unsureChar = create("calendar-unsure", "Zweifler", "Krieger", "Schutz (Tank)", true)
	f.stranger, f.strangerCh = create("calendar-stranger", "Neuling", "Jäger", "Treffsicherheit", true)
	signups := raidsignup.NewService(raidsignup.NewRepo(f.db))
	if err := signups.SubmitForPlayer(ctx, f.raidID, f.actor, f.healerChar, "Heilig"); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPut, fmt.Sprintf("/absences/%d", f.unsure), map[string]any{"status": "TENTATIVE"}, http.StatusOK)
	return f
}

func (f *calendarFixture) inviteExport() string {
	f.t.Helper()
	status, body := do(f.t, f.srv.Client(), http.MethodGet, fmt.Sprintf("%s/api/v1/raids/%d/addon-invite-export", f.srv.URL, f.raidID), f.memberToken, nil)
	if status != http.StatusOK {
		f.t.Fatalf("export: %d %s", status, body)
	}
	var out struct{ Data struct{ Text string } }
	if err := json.Unmarshal(body, &out); err != nil {
		f.t.Fatal(err)
	}
	return out.Data.Text
}

type calendarPreviewRow struct {
	Name          string   `json:"name"`
	Answer        string   `json:"answer"`
	Status        string   `json:"status"`
	CharacterID   *int64   `json:"character_id"`
	WebsiteStates []string `json:"website_states"`
	Action        string   `json:"action"`
	Spec          string   `json:"spec"`
	Errors        []string `json:"errors"`
}

func (f *calendarFixture) calendarPreview(payload string) []calendarPreviewRow {
	f.t.Helper()
	body := f.request(http.MethodPost, "/addon-calendar-import/preview", map[string]any{"payload": payload}, http.StatusOK)
	var out struct {
		Data struct{ Rows []calendarPreviewRow }
	}
	if err := json.Unmarshal(body, &out); err != nil {
		f.t.Fatal(err)
	}
	return out.Data.Rows
}

func (f *calendarFixture) overview() *raidsignup.Overview {
	f.t.Helper()
	overview, err := raidsignup.NewService(raidsignup.NewRepo(f.db)).Overview(context.Background(), f.raidID)
	if err != nil {
		f.t.Fatal(err)
	}
	return overview
}

// ADR 092: after the roster, the invite list names every player who answered
// on the website without a roster slot, with that answer, for the calendar.
func TestAddonInviteExportListsWebsiteAnswersForCalendar(t *testing.T) {
	f := newCalendarFixture(t)
	f.attend(f.first, f.firstChar)
	f.request(http.MethodPut, fmt.Sprintf("/absences/%d", f.second), map[string]any{"reason": "Urlaub"}, http.StatusOK)
	want := fmt.Sprintf("RS2;INV;%d;Correction raid;25\n"+
		"Correction0;DD;Magier;0\n"+
		"Heilerin;HEAL;Priester;0;SIGNUP\n"+
		"Zweifler;TANK;Krieger;0;TENTATIVE\n"+
		"Correction1;DD;Magier;0;DECLINED", f.raidID)
	if got := f.inviteExport(); got != want {
		t.Fatalf("export:\n%s\nwant:\n%s", got, want)
	}
	// A rostered player is listed once, with the roster line, whatever he answered.
	f.request(http.MethodPut, fmt.Sprintf("/absences/%d", f.first), map[string]any{"status": "TENTATIVE"}, http.StatusOK)
	if got := f.inviteExport(); got != want {
		t.Fatalf("rostered player listed twice:\n%s", got)
	}
}

func TestAddonCalendarImportPreviewProposesActions(t *testing.T) {
	f := newCalendarFixture(t)
	f.attend(f.first, f.firstChar)
	payload := fmt.Sprintf("RS2;CAL;%d;Raidleiter;1760000000\n"+
		"Correction0;accepted\n"+ // in the roster already
		"correction1;accepted\n"+ // new acceptance, case-insensitive
		"Heilerin;accepted\n"+ // already signed up
		"Zweifler;declined\n"+ // unsure on the website, now declines
		"Neuling;invited\n"+ // no answer
		"Fremder;accepted\n"+ // not on the website
		"Kaputt", f.raidID)
	rows := f.calendarPreview(payload)
	want := []struct{ name, status, action string }{
		{"Correction0", "known", "none"},
		{"correction1", "known", "signup"},
		{"Heilerin", "known", "none"},
		{"Zweifler", "known", "decline"},
		{"Neuling", "known", "none"},
		{"Fremder", "unknown", "none"},
		{"", "invalid", "none"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for i, w := range want {
		if rows[i].Name != w.name || rows[i].Status != w.status || rows[i].Action != w.action {
			t.Fatalf("row %d = %+v, want %+v", i, rows[i], w)
		}
	}
	if rows[1].Spec != "Fire" {
		t.Fatalf("signup spec = %q, want the main spec", rows[1].Spec)
	}
	if len(rows[3].WebsiteStates) != 1 || rows[3].WebsiteStates[0] != "tentative" {
		t.Fatalf("website states = %v", rows[3].WebsiteStates)
	}
}

func TestAddonCalendarImportAppliesSelectedAnswers(t *testing.T) {
	f := newCalendarFixture(t)
	// Neuling declined on the website, then accepted the calendar invite.
	f.request(http.MethodPut, fmt.Sprintf("/absences/%d", f.stranger), map[string]any{}, http.StatusOK)
	payload := fmt.Sprintf("RS2;CAL;%d;Raidleiter;1760000000\nNeuling;accepted\nZweifler;declined\nHeilerin;tentative", f.raidID)
	selections := []map[string]any{
		{"character_id": f.strangerCh, "action": "signup"},
		{"character_id": f.unsureChar, "action": "decline"},
		{"character_id": f.healerChar, "action": "signup"}, // the preview proposes "tentative"
	}
	if status, _ := do(t, f.srv.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/raids/%d/addon-calendar-import", f.srv.URL, f.raidID), f.memberToken, map[string]any{"payload": payload, "selections": selections}); status != http.StatusForbidden {
		t.Fatalf("member apply: %d", status)
	}
	body := f.request(http.MethodPost, "/addon-calendar-import", map[string]any{"payload": payload, "selections": selections}, http.StatusOK)
	var out struct {
		Data []struct {
			CharacterID int64  `json:"character_id"`
			Status      string `json:"status"`
			Code        string `json:"code"`
		}
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 3 || out.Data[0].Status != "signed_up" || out.Data[1].Status != "declined" || out.Data[2].Code != "not_proposed" {
		t.Fatalf("results = %s", body)
	}
	overview := f.overview()
	absences := map[int64]string{}
	for _, a := range overview.Absences {
		absences[a.PlayerID] = a.Status
	}
	if _, still := absences[f.stranger]; still {
		t.Fatal("acceptance did not remove the website refusal")
	}
	if absences[f.unsure] != raidsignup.AbsenceDeclined {
		t.Fatalf("unsure player = %q, want DECLINED", absences[f.unsure])
	}
	if _, changed := absences[f.healer]; changed {
		t.Fatal("unselected answer was applied")
	}
	signedUp := false
	for _, entry := range overview.Entries {
		if entry.CharacterID == f.strangerCh && !entry.IsWithdrawn && entry.Spec == "Treffsicherheit" {
			signedUp = true
		}
	}
	if !signedUp {
		t.Fatalf("no pending signup with main spec: %+v", overview.Entries)
	}
	// Applying the same export again changes nothing.
	again := f.calendarPreview(payload)
	if again[0].Action != "none" || again[1].Action != "none" {
		t.Fatalf("second preview still proposes changes: %+v", again)
	}
	var audits int
	if err := f.db.NewRaw("SELECT COUNT(*) FROM audit_log WHERE entity_type = 'raid_signup' AND action = 'submit_for_player'").Scan(context.Background(), &audits); err != nil || audits != 2 {
		t.Fatalf("submit_for_player audits = %d, %v", audits, err)
	}
}
